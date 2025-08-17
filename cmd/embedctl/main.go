package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
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

func main() {
	log.SetFlags(0)
	var (
		model = flag.String("model", "", "override model")
		text  = flag.String("text", "", "text to embed (use -stdin to read from STDIN)")
		stdin = flag.Bool("stdin", false, "read entire STDIN as input text")
	)
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// Override model if specified
	if *model != "" {
		cfg.Embedding.Model = *model
	}

	// Check required config
	if cfg.Embedding.APIKey == "" {
		log.Fatal("EMBED_API_KEY not set (set in .env, environment, or config.yaml)")
	}

	var input string
	if *stdin {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatalf("read stdin: %v", err)
		}
		input = string(b)
	} else {
		input = *text
	}
	if input == "" {
		log.Fatal("no input provided; use -text or -stdin")
	}

	reqBody, _ := json.Marshal(embedReq{Model: cfg.Embedding.Model, Input: []string{input}})

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Embedding.Timeout)*time.Second)
	defer cancel()
	url := cfg.Embedding.BaseURL + cfg.Embedding.Path

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		log.Fatalf("new request: %v", err)
	}

	// Default header scheme: Authorization: Bearer <key>
	if cfg.Embedding.APIHeader == "Authorization" {
		req.Header.Set("Authorization", "Bearer "+cfg.Embedding.APIKey)
	} else {
		req.Header.Set(cfg.Embedding.APIHeader, cfg.Embedding.APIKey)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("http: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		log.Fatalf("embeddings error: %s: %s", resp.Status, string(b))
	}
	var er embedResp
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		log.Fatalf("decode: %v", err)
	}
	if len(er.Data) == 0 {
		log.Fatal("no data returned")
	}

	// Print JSON array of numbers
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(er.Data[0].Embedding); err != nil {
		log.Fatalf("encode: %v", err)
	}
}
