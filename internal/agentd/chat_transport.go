package agentd

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"
)

type chatTransportOptions struct {
	EnablePromptCORS bool
	MaxBodyBytes     int64
	DecodeErrorLabel string
}

func prepareChatTransport(w http.ResponseWriter, r *http.Request, opts chatTransportOptions) (chatRunRequest, bool) {
	if opts.EnablePromptCORS {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Vary", "Origin")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
			w.WriteHeader(http.StatusNoContent)
			return chatRunRequest{}, false
		}
	}

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return chatRunRequest{}, false
	}

	if opts.MaxBodyBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, opts.MaxBodyBytes)
	}
	if r.Body != nil {
		defer r.Body.Close()
	}

	var req chatRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if opts.DecodeErrorLabel != "" {
			log.Printf("%s: %v", opts.DecodeErrorLabel, err)
		}
		http.Error(w, "bad request", http.StatusBadRequest)
		return chatRunRequest{}, false
	}
	req.normalize()
	return req, true
}
