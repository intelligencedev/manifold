package webui

import (
	"bytes"
	"embed"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

//go:embed templates/*
var assets embed.FS

// Register registers handlers on the provided ServeMux.
func Register(mux *http.ServeMux) {
	// Serve static files under /static/
	fs := http.FS(assets)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(fs)))

	// Serve index on /
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		f, err := assets.Open("templates/index.html")
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
			// default backend endpoint
			backend = "http://localhost:32180/agent/run"
		}

		// create JSON payload and forward
		reqBody, err := json.Marshal(map[string]string{"prompt": in.Prompt})
		if err != nil {
			log.Printf("marshal: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		client := &http.Client{Timeout: 30 * time.Second}
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, backend, io.NopCloser(bytes.NewReader(reqBody)))
		if err != nil {
			log.Printf("new req: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("backend request: %v", err)
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// copy content-type and status
		if ct := resp.Header.Get("Content-Type"); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		w.WriteHeader(resp.StatusCode)
		if _, err := io.Copy(w, resp.Body); err != nil {
			log.Printf("copy backend resp: %v", err)
		}
	})
}
