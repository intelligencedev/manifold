package agentd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"manifold/internal/auth"
	llmpkg "manifold/internal/llm"
	anthropicllm "manifold/internal/llm/anthropic"
	googlellm "manifold/internal/llm/google"
	openaillm "manifold/internal/llm/openai"
	persist "manifold/internal/persistence"
	"manifold/internal/specialists"
)

type visionClientSelection struct {
	Provider  string
	Model     string
	OpenAI    *openaillm.Client
	Anthropic *anthropicllm.Client
	Google    *googlellm.Client
}

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
		// Check if the main LLM provider supports compaction (OpenAI Responses API).
		// Non-OpenAI providers cannot use encrypted compaction summaries.
		targetSupportsCompaction := providerSupportsCompaction(a.llm)
		history, _, err := a.chatMemory.BuildContextForProvider(r.Context(), userID, sessionID, targetSupportsCompaction)
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

		specialistName := strings.TrimSpace(r.URL.Query().Get("specialist"))
		teamName := strings.TrimSpace(r.URL.Query().Get("team"))
		if teamName == "" {
			teamName = strings.TrimSpace(r.URL.Query().Get("group"))
		}

		specOwner := systemUserID
		if userID != nil {
			specOwner = *userID
		}

		visionSel, statusCode, resolveErr := a.resolveVisionClientAndModel(
			r.Context(),
			specOwner,
			specialistName,
			teamName,
		)
		if resolveErr != nil {
			http.Error(w, resolveErr.Error(), statusCode)
			return
		}

		if strings.TrimSpace(visionSel.Model) == "" {
			visionSel.Model = strings.TrimSpace(a.cfg.OpenAI.Model)
		}

		if specialistName == "" && teamName == "" && strings.EqualFold(visionSel.Provider, "openai") && a.cfg.OpenAI.APIKey == "" {
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
			if mt != "image/png" && mt != "image/jpeg" && mt != "image/jpg" && mt != "image/webp" {
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
		var out llmpkg.Message
		var callErr error
		switch {
		case visionSel.OpenAI != nil:
			out, callErr = visionSel.OpenAI.ChatWithImageAttachments(ctx, msgs, images, nil, visionSel.Model)
		case visionSel.Anthropic != nil:
			anthropicImages := make([]anthropicllm.ImageAttachment, 0, len(atts))
			for _, att := range atts {
				anthropicImages = append(anthropicImages, anthropicllm.ImageAttachment{MimeType: att.mime, Base64Data: att.b64})
			}
			out, callErr = visionSel.Anthropic.ChatWithImageAttachments(ctx, msgs, anthropicImages, nil, visionSel.Model)
		case visionSel.Google != nil:
			googleImages := make([]googlellm.ImageAttachment, 0, len(atts))
			for _, att := range atts {
				googleImages = append(googleImages, googlellm.ImageAttachment{MimeType: att.mime, Base64Data: att.b64})
			}
			out, callErr = visionSel.Google.ChatWithImageAttachments(ctx, msgs, googleImages, nil, visionSel.Model)
		default:
			callErr = errors.New("vision provider unavailable")
		}
		if callErr != nil {
			log.Error().Err(callErr).Msg("vision chat error")
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
			if err := storeChatTurn(r.Context(), a.chatStore, userID, sessionID, prompt, out.Content, visionSel.Model); err != nil {
				log.Error().Err(err).Str("session", sessionID).Msg("store_chat_turn_vision_stream")
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": out.Content})
		a.runs.updateStatus(vrun.ID, "completed", 0)
		if err := storeChatTurn(r.Context(), a.chatStore, userID, sessionID, prompt, out.Content, visionSel.Model); err != nil {
			log.Error().Err(err).Str("session", sessionID).Msg("store_chat_turn_vision")
		}
	}
}

func (a *app) resolveVisionClientAndModel(ctx context.Context, owner int64, specialistName, teamName string) (visionClientSelection, int, error) {
	unsupportedErr := errors.New("vision requires an OpenAI-compatible, Anthropic, or Google provider")
	empty := visionClientSelection{}

	if teamName != "" {
		if a.teamStore == nil {
			return empty, http.StatusInternalServerError, errors.New("teams unavailable")
		}
		team, ok, err := a.teamStore.GetByName(ctx, owner, teamName)
		if err != nil {
			return empty, http.StatusInternalServerError, errors.New("failed to load team")
		}
		if !ok {
			return empty, http.StatusNotFound, errors.New("team not found")
		}
		llmCfg, provider := specialists.ApplyLLMClientOverride(a.cfg.LLMClient, team.Orchestrator)
		provider = strings.ToLower(strings.TrimSpace(provider))
		if provider == "" {
			provider = strings.ToLower(strings.TrimSpace(llmCfg.Provider))
		}
		switch provider {
		case "", "openai", "local":
			model := strings.TrimSpace(team.Orchestrator.Model)
			if model == "" {
				model = strings.TrimSpace(llmCfg.OpenAI.Model)
			}
			return visionClientSelection{
				Provider: "openai",
				Model:    model,
				OpenAI:   openaillm.New(llmCfg.OpenAI, a.httpClient),
			}, 0, nil
		case "anthropic":
			model := strings.TrimSpace(team.Orchestrator.Model)
			if model == "" {
				model = strings.TrimSpace(llmCfg.Anthropic.Model)
			}
			return visionClientSelection{
				Provider:  "anthropic",
				Model:     model,
				Anthropic: anthropicllm.New(llmCfg.Anthropic, a.httpClient),
			}, 0, nil
		case "google":
			model := strings.TrimSpace(team.Orchestrator.Model)
			if model == "" {
				model = strings.TrimSpace(llmCfg.Google.Model)
			}
			client, buildErr := googlellm.New(llmCfg.Google, a.httpClient)
			if buildErr != nil {
				return empty, http.StatusInternalServerError, errors.New("team orchestrator not configured")
			}
			return visionClientSelection{
				Provider: "google",
				Model:    model,
				Google:   client,
			}, 0, nil
		default:
			return empty, http.StatusBadRequest, unsupportedErr
		}
	}

	if specialistName != "" && !strings.EqualFold(specialistName, specialists.OrchestratorName) {
		reg, err := a.specialistsRegistryForUser(ctx, owner)
		if err != nil {
			return empty, http.StatusInternalServerError, errors.New("specialist registry unavailable")
		}
		sp, ok := reg.Get(specialistName)
		if !ok || sp == nil {
			return empty, http.StatusNotFound, errors.New("specialist not found")
		}
		switch client := sp.Provider().(type) {
		case *openaillm.Client:
			return visionClientSelection{
				Provider: "openai",
				Model:    strings.TrimSpace(sp.Model),
				OpenAI:   client,
			}, 0, nil
		case *anthropicllm.Client:
			return visionClientSelection{
				Provider:  "anthropic",
				Model:     strings.TrimSpace(sp.Model),
				Anthropic: client,
			}, 0, nil
		case *googlellm.Client:
			return visionClientSelection{
				Provider: "google",
				Model:    strings.TrimSpace(sp.Model),
				Google:   client,
			}, 0, nil
		default:
			return empty, http.StatusBadRequest, unsupportedErr
		}
	}

	llmCfg := a.cfg.LLMClient
	provider := strings.ToLower(strings.TrimSpace(llmCfg.Provider))
	if provider == "" {
		provider = "openai"
	}
	if a.specStore != nil {
		if orch, ok, _ := a.specStore.GetByName(ctx, owner, specialists.OrchestratorName); ok {
			var resolved string
			llmCfg, resolved = specialists.ApplyLLMClientOverride(llmCfg, orch)
			if strings.TrimSpace(resolved) != "" {
				provider = strings.ToLower(strings.TrimSpace(resolved))
			}
		}
	}

	switch provider {
	case "", "openai", "local":
		return visionClientSelection{
			Provider: "openai",
			Model:    strings.TrimSpace(llmCfg.OpenAI.Model),
			OpenAI:   openaillm.New(llmCfg.OpenAI, a.httpClient),
		}, 0, nil
	case "anthropic":
		return visionClientSelection{
			Provider:  "anthropic",
			Model:     strings.TrimSpace(llmCfg.Anthropic.Model),
			Anthropic: anthropicllm.New(llmCfg.Anthropic, a.httpClient),
		}, 0, nil
	case "google":
		client, err := googlellm.New(llmCfg.Google, a.httpClient)
		if err != nil {
			return empty, http.StatusInternalServerError, errors.New("orchestrator not configured")
		}
		return visionClientSelection{
			Provider: "google",
			Model:    strings.TrimSpace(llmCfg.Google.Model),
			Google:   client,
		}, 0, nil
	default:
		return empty, http.StatusBadRequest, unsupportedErr
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
		// Resolve user for per-user API key lookup
		var userID int64
		if a.cfg.Auth.Enabled {
			u, ok := auth.CurrentUser(r.Context())
			if !ok {
				w.Header().Set("WWW-Authenticate", "Bearer realm=\"sio\"")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			id, _, err := resolveChatAccess(r.Context(), a.authStore, u)
			if err != nil {
				log.Error().Err(err).Msg("stt_resolve_chat_access")
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if id != nil {
				userID = *id
			}
		}

		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

		// Get per-user orchestrator config (includes API key)
		orch := a.orchestratorSpecialist(r.Context(), userID)

		model := strings.TrimSpace(a.cfg.STT.Model)
		if model == "" {
			model = "gpt-4o-mini-transcribe"
		}
		baseURL := strings.TrimSpace(a.cfg.STT.BaseURL)
		if baseURL == "" {
			baseURL = strings.TrimSpace(orch.BaseURL)
		}
		if baseURL == "" {
			baseURL = "https://api.openai.com"
		}
		baseURL = strings.TrimRight(baseURL, "/")
		baseURL = strings.TrimSuffix(baseURL, "/v1")
		reqURL := baseURL + "/v1/audio/transcriptions"
		log.Debug().Str("endpoint", reqURL).Str("model", model).Int64("user_id", userID).Msg("stt_request")

		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, err := mw.CreateFormFile("file", "prompt.wav")
		if err != nil {
			http.Error(w, "form error", http.StatusInternalServerError)
			return
		}
		if _, err := fw.Write(data); err != nil {
			http.Error(w, "form error", http.StatusInternalServerError)
			return
		}
		if err := mw.WriteField("model", model); err != nil {
			http.Error(w, "form error", http.StatusInternalServerError)
			return
		}
		if err := mw.WriteField("response_format", "json"); err != nil {
			http.Error(w, "form error", http.StatusInternalServerError)
			return
		}
		if err := mw.Close(); err != nil {
			http.Error(w, "form error", http.StatusInternalServerError)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		started := time.Now()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, &buf)
		if err != nil {
			http.Error(w, "request error", http.StatusInternalServerError)
			return
		}
		req.Header.Set("Content-Type", mw.FormDataContentType())
		if orch.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+orch.APIKey)
		}

		resp, err := a.httpClient.Do(req)
		if err != nil {
			log.Warn().Err(err).Str("endpoint", reqURL).Dur("elapsed", time.Since(started)).Msg("stt_request_failed")
			http.Error(w, "stt request failed", http.StatusBadGateway)
			return
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
			log.Warn().Int("status", resp.StatusCode).Str("body", strings.TrimSpace(string(b))).Str("endpoint", reqURL).Dur("elapsed", time.Since(started)).Msg("stt_request_error")
			http.Error(w, strings.TrimSpace(string(b)), resp.StatusCode)
			return
		}

		var out struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			log.Warn().Err(err).Str("endpoint", reqURL).Dur("elapsed", time.Since(started)).Msg("stt_response_decode_failed")
			http.Error(w, "invalid stt response", http.StatusBadGateway)
			return
		}
		log.Debug().Str("endpoint", reqURL).Int("text_len", len(out.Text)).Dur("elapsed", time.Since(started)).Msg("stt_response")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"text": strings.TrimSpace(out.Text)})
	}
}
