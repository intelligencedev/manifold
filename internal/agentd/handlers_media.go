package agentd

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"

	"manifold/internal/auth"
	llmpkg "manifold/internal/llm"
	openaillm "manifold/internal/llm/openai"
	persist "manifold/internal/persistence"
)

func (a *app) agentVisionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var userID *int64
		if a.cfg.Auth.Enabled {
			u, ok := auth.CurrentUser(r.Context())
			if !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			id, _, err := resolveChatAccess(r.Context(), a.authStore, u)
			if err != nil {
				log.Error().Err(err).Msg("resolve_chat_access")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			userID = id
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		prompt := strings.TrimSpace(r.FormValue("prompt"))
		if prompt == "" {
			http.Error(w, "prompt required", http.StatusBadRequest)
			return
		}
		sessionID := strings.TrimSpace(r.FormValue("session_id"))
		if sessionID == "" {
			sessionID = "default"
		}
		if _, err := ensureChatSession(r.Context(), a.chatStore, userID, sessionID); err != nil {
			if errors.Is(err, persist.ErrForbidden) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			log.Error().Err(err).Str("session", sessionID).Msg("ensure_chat_session")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		history, err := a.chatMemory.BuildContext(r.Context(), userID, sessionID)
		if err != nil {
			if errors.Is(err, persist.ErrForbidden) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			log.Error().Err(err).Str("session", sessionID).Msg("load_chat_history")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		form := r.MultipartForm
		var files []*multipart.FileHeader
		if form != nil {
			if fh := form.File["images"]; len(fh) > 0 {
				files = append(files, fh...)
			}
			if fh := form.File["image"]; len(fh) > 0 {
				files = append(files, fh...)
			}
		}
		if len(files) == 0 {
			http.Error(w, "no images provided", http.StatusBadRequest)
			return
		}

		if a.cfg.OpenAI.APIKey == "" {
			vrun := a.runs.create("[vision] " + prompt)
			if r.Header.Get("Accept") == "text/event-stream" {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				fl, _ := w.(http.Flusher)
				if b, err := json.Marshal("(dev) mock vision response: " + prompt); err == nil {
					fmt.Fprintf(w, "event: final\ndata: %s\n\n", b)
				} else {
					fmt.Fprintf(w, "event: final\ndata: %q\n\n", "(dev) mock vision response")
				}
				fl.Flush()
				a.runs.updateStatus(vrun.ID, "completed", 0)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"result": "(dev) mock vision response: " + prompt})
			a.runs.updateStatus(vrun.ID, "completed", 0)
			return
		}

		type imgAtt struct {
			mime string
			b64  string
		}
		var atts []imgAtt
		for _, fh := range files {
			f, err := fh.Open()
			if err != nil {
				http.Error(w, "file open", http.StatusBadRequest)
				return
			}
			data, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				http.Error(w, "file read", http.StatusBadRequest)
				return
			}
			mt := http.DetectContentType(data)
			if mt != "image/png" && mt != "image/jpeg" && mt != "image/jpg" {
				http.Error(w, "unsupported image type", http.StatusBadRequest)
				return
			}
			if mt == "image/jpg" {
				mt = "image/jpeg"
			}
			atts = append(atts, imgAtt{mime: mt, b64: base64.StdEncoding.EncodeToString(data)})
		}

		msgs := make([]llmpkg.Message, 0, len(history)+1)
		msgs = append(msgs, history...)
		msgs = append(msgs, llmpkg.Message{Role: "user", Content: prompt})

		vSeconds := a.cfg.AgentRunTimeoutSeconds
		ctx, cancel, vDur := withMaybeTimeout(r.Context(), vSeconds)
		defer cancel()
		if vDur > 0 {
			log.Debug().Dur("timeout", vDur).Str("endpoint", "/agent/vision").Msg("using configured agent timeout")
		} else {
			log.Debug().Str("endpoint", "/agent/vision").Msg("no timeout configured; running until completion")
		}

		images := make([]openaillm.ImageAttachment, 0, len(atts))
		for _, att := range atts {
			images = append(images, openaillm.ImageAttachment{MimeType: att.mime, Base64Data: att.b64})
		}

		vrun := a.runs.create("[vision] " + prompt)

		openaiClient, ok := a.llm.(*openaillm.Client)
		if !ok {
			http.Error(w, "vision is only supported with the OpenAI provider", http.StatusBadRequest)
			a.runs.updateStatus(vrun.ID, "failed", 0)
			return
		}
		out, err := openaiClient.ChatWithImageAttachments(ctx, msgs, images, nil, a.cfg.OpenAI.Model)
		if err != nil {
			log.Error().Err(err).Msg("vision chat error")
			http.Error(w, "internal server error", http.StatusInternalServerError)
			a.runs.updateStatus(vrun.ID, "failed", 0)
			return
		}

		if r.Header.Get("Accept") == "text/event-stream" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			fl, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "streaming not supported", http.StatusInternalServerError)
				return
			}
			payload := map[string]string{"type": "final", "data": out.Content}
			b, _ := json.Marshal(payload)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
			a.runs.updateStatus(vrun.ID, "completed", 0)
			if err := storeChatTurn(r.Context(), a.chatStore, userID, sessionID, prompt, out.Content, a.cfg.OpenAI.Model); err != nil {
				log.Error().Err(err).Str("session", sessionID).Msg("store_chat_turn_vision_stream")
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": out.Content})
		a.runs.updateStatus(vrun.ID, "completed", 0)
		if err := storeChatTurn(r.Context(), a.chatStore, userID, sessionID, prompt, out.Content, a.cfg.OpenAI.Model); err != nil {
			log.Error().Err(err).Str("session", sessionID).Msg("store_chat_turn_vision")
		}
	}
}

func (a *app) audioServeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		filename := strings.TrimPrefix(r.URL.Path, "/audio/")
		if filename == "" {
			http.Error(w, "file not specified", http.StatusBadRequest)
			return
		}
		http.ServeFile(w, r, filename)
	}
}

func (a *app) sttHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if a.whisperModel == nil {
			http.Error(w, "whisper model unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("audio")
		if err != nil {
			http.Error(w, "missing audio", http.StatusBadRequest)
			return
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "read error", http.StatusInternalServerError)
			return
		}
		if len(data) < 44 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
			http.Error(w, "unsupported audio (expect WAV)", http.StatusBadRequest)
			return
		}
		channels := binary.LittleEndian.Uint16(data[22:24])
		sampleRate := binary.LittleEndian.Uint32(data[24:28])
		bitsPerSample := binary.LittleEndian.Uint16(data[34:36])
		offset := 12
		var audioStart, audioLen int
		for offset+8 <= len(data) {
			chunkID := string(data[offset : offset+4])
			chunkSize := int(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
			if chunkID == "data" {
				audioStart = offset + 8
				audioLen = chunkSize
				break
			}
			offset += 8 + chunkSize
		}
		if audioLen == 0 || audioStart+audioLen > len(data) {
			http.Error(w, "invalid wav data", http.StatusBadRequest)
			return
		}
		raw := data[audioStart : audioStart+audioLen]
		var samples []float32
		switch bitsPerSample {
		case 16:
			for i := 0; i+1 < len(raw); i += 2 {
				sample := int16(binary.LittleEndian.Uint16(raw[i : i+2]))
				samples = append(samples, float32(sample)/32768.0)
			}
		case 32:
			for i := 0; i+3 < len(raw); i += 4 {
				bits := binary.LittleEndian.Uint32(raw[i : i+4])
				samples = append(samples, wavFloat32(bits))
			}
		default:
			http.Error(w, "unsupported bit depth", http.StatusBadRequest)
			return
		}
		if channels == 2 {
			mono := make([]float32, len(samples)/2)
			for i := 0; i < len(mono); i++ {
				mono[i] = (samples[i*2] + samples[i*2+1]) / 2
			}
			samples = mono
		}
		if sampleRate != 16000 {
			log.Warn().Uint32("rate", sampleRate).Msg("non-16k audio provided; transcription may be degraded")
		}
		ctx, err := a.whisperModel.NewContext()
		if err != nil {
			http.Error(w, "ctx error", http.StatusInternalServerError)
			return
		}
		ctx.SetLanguage("en")
		if err := ctx.Process(samples, nil, nil, nil); err != nil {
			http.Error(w, "process error", http.StatusInternalServerError)
			return
		}
		var sb strings.Builder
		for {
			seg, err := ctx.NextSegment()
			if err != nil {
				break
			}
			sb.WriteString(seg.Text)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"text": strings.TrimSpace(sb.String())})
	}
}
