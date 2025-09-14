package webui

import (
	"bytes"
	"embed"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
)

//go:embed templates/*
var Assets embed.FS

// Register registers handlers on the provided ServeMux.
func Register(mux *http.ServeMux) {
	// Serve static files under /static/
	fs := http.FS(Assets)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(fs)))

	// Serve assets files under /assets/
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))

	// Serve index on /
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		f, err := Assets.Open("templates/index.html")
		if err != nil {
			log.Printf("open index: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := io.Copy(w, f); err != nil {
			log.Printf("copy index: %v", err)
		}
	})

	// POST /api/prompt accepts {"prompt":"..."} and forwards to backend
	mux.HandleFunc("/api/prompt", func(w http.ResponseWriter, r *http.Request) {
		// Basic CORS / preflight support (useful when UI accessed from different origin during dev)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Vary", "Origin")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// limit body to 64KB
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		defer r.Body.Close()

		var in struct {
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			log.Printf("decode prompt: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		backend := os.Getenv("WEB_UI_BACKEND_URL")
		if backend == "" {
			// Default backend is agentd which by default listens on 32180.
			// Web UI itself now defaults to 32181 to avoid port collision.
			backend = "http://localhost:32180/agent/run"
		}

		// create JSON payload and forward
		reqBody, err := json.Marshal(map[string]string{"prompt": in.Prompt})
		if err != nil {
			log.Printf("marshal: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// NOTE: Removed hard timeout so long-running generations can continue
		// until the browser/client explicitly cancels (AbortController) which will
		// propagate via the request context. This enables the new Stop button in
		// the web UI to immediately terminate in-flight inference.
		client := &http.Client{}
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, backend, io.NopCloser(bytes.NewReader(reqBody)))
		if err != nil {
			log.Printf("new req: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		// Propagate Accept header (client may request text/event-stream for SSE)
		if a := r.Header.Get("Accept"); a != "" {
			req.Header.Set("Accept", a)
		}

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("backend request: %v", err)
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// If backend is streaming (SSE), proxy the stream and flush frequently
		ct := resp.Header.Get("Content-Type")
		w.Header().Set("Cache-Control", "no-cache")
		if ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		w.WriteHeader(resp.StatusCode)

		// If this is an event-stream or the client requested it, stream with flush
		if ct == "text/event-stream" || r.Header.Get("Accept") == "text/event-stream" {
			fl, ok := w.(http.Flusher)
			if !ok {
				// fallback to copy
				if _, err := io.Copy(w, resp.Body); err != nil {
					log.Printf("copy backend resp: %v", err)
				}
				return
			}
			buf := make([]byte, 1024)
			for {
				n, err := resp.Body.Read(buf)
				if n > 0 {
					if _, werr := w.Write(buf[:n]); werr != nil {
						log.Printf("write to client: %v", werr)
						return
					}
					fl.Flush()
				}
				if err != nil {
					if err != io.EOF {
						log.Printf("stream read error: %v", err)
					}
					return
				}
			}
		}
		// Non-streaming fallback: copy once
		if _, err := io.Copy(w, resp.Body); err != nil {
			log.Printf("copy backend resp: %v", err)
		}
	})
}
