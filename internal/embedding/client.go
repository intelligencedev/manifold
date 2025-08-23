package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"singularityio/internal/config"
)

type embedReq struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResp struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// EmbedText calls the configured embedding endpoint and returns one embedding
// per input string. Caller should provide cfg loaded from config.Load().
func EmbedText(ctx context.Context, cfg config.EmbeddingConfig, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("no inputs")
	}
	reqBody, _ := json.Marshal(embedReq{Model: cfg.Model, Input: inputs})
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	url := cfg.BaseURL + cfg.Path
	req, err := http.NewRequestWithContext(cctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	if cfg.APIHeader == "Authorization" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	} else if cfg.APIHeader != "" {
		req.Header.Set(cfg.APIHeader, cfg.APIKey)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embeddings error: %s: %s", resp.Status, string(b))
	}
	var er embedResp
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		return nil, err
	}
	if len(er.Data) != len(inputs) {
		// still return what we have, but consider it an error
		return nil, fmt.Errorf("unexpected embedding count: got %d, want %d", len(er.Data), len(inputs))
	}
	out := make([][]float32, len(er.Data))
	for i := range er.Data {
		out[i] = er.Data[i].Embedding
	}
	return out, nil
}
