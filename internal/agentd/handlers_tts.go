package agentd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// ttsRequest is the robot-facing /tts payload.
type ttsRequest struct {
	Text      string `json:"text"`
	Voice     string `json:"voice"`
	Lang      string `json:"lang"`
	SessionID string `json:"sessionId"`
}

// ttsHandler renders speech for the given text via the configured TTS backend
// (a host-side Supertonic sidecar when tts.engine=supertonic) and relays the
// audio to the caller as an SSE stream of tts_chunk frames carrying base64
// PCM16, followed by a terminal done frame. This is the endpoint the Reachy
// Mini bot calls; it decodes the frames and plays them on the robot's speaker.
func (a *app) ttsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req ttsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// In-process pure-Go engine: no sidecar, no proxy.
		if strings.EqualFold(strings.TrimSpace(a.cfg.TTS.Engine), "born") {
			a.ttsBornResponse(w, r, req)
			return
		}

		baseURL := strings.TrimSpace(a.cfg.TTS.BaseURL)
		if baseURL == "" {
			http.Error(w, "tts not configured", http.StatusServiceUnavailable)
			return
		}

		voice := strings.TrimSpace(req.Voice)
		if voice == "" {
			voice = strings.TrimSpace(a.cfg.TTS.Voice)
		}

		payload := map[string]any{
			"input":           req.Text,
			"voice":           voice,
			"response_format": "pcm",
		}
		if model := strings.TrimSpace(a.cfg.TTS.Model); model != "" {
			payload["model"] = model
		}
		if lang := strings.TrimSpace(req.Lang); lang != "" {
			payload["lang"] = lang
		}
		bodyBytes, _ := json.Marshal(payload)

		reqURL := strings.TrimRight(baseURL, "/") + "/audio/speech"
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()

		outReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
		if err != nil {
			http.Error(w, "request error", http.StatusInternalServerError)
			return
		}
		outReq.Header.Set("Content-Type", "application/json")

		resp, err := a.httpClient.Do(outReq)
		if err != nil {
			log.Warn().Err(err).Str("endpoint", reqURL).Msg("tts_request_failed")
			http.Error(w, "tts request failed", http.StatusBadGateway)
			return
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			log.Warn().Int("status", resp.StatusCode).Str("endpoint", reqURL).Msg("tts_backend_error")
			http.Error(w, "tts backend error: "+string(snippet), http.StatusBadGateway)
			return
		}

		rate := 0
		if v := resp.Header.Get("X-Sample-Rate"); v != "" {
			if n, convErr := strconv.Atoi(v); convErr == nil {
				rate = n
			}
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		writeFrame := func(m map[string]any) {
			data, _ := json.Marshal(m)
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(data)
			_, _ = w.Write([]byte("\n\n"))
			flusher.Flush()
		}

		buf := make([]byte, 16*1024)
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				writeFrame(map[string]any{
					"type":  "tts_chunk",
					"bytes": n,
					"b64":   base64.StdEncoding.EncodeToString(buf[:n]),
					"rate":  rate,
				})
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				log.Warn().Err(readErr).Str("endpoint", reqURL).Msg("tts_stream_read")
				break
			}
		}
		writeFrame(map[string]any{"type": "done", "rate": rate})
	}
}
