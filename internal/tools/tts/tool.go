package tts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	// Intentionally use plain HTTP for the TTS POST. The project uses
	// github.com/openai/openai-go/v2 elsewhere for chat; this tool keeps
	// a minimal dependency surface and honors the configured base URL and
	// API key via headers.

	"intelligence.dev/internal/config"
	"intelligence.dev/internal/observability"
)

// Tool implements a simple TTS tool that calls the OpenAI /v1/audio/speech endpoint.
// It prefers a provider present in context but falls back to the configured
// TTSBaseURL in the top-level config. The tool saves the audio file and returns
// success status with file information.
type Tool struct {
	cfg        config.Config
	httpClient *http.Client
}

func New(cfg config.Config, httpClient *http.Client) *Tool {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Tool{cfg: cfg, httpClient: httpClient}
}

func (t *Tool) Name() string { return "text_to_speech" }

func (t *Tool) JSONSchema() map[string]any {
	return map[string]any{
		"description": "Create speech audio from text using OpenAI-compatible TTS endpoint",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text":  map[string]any{"type": "string", "description": "Text to synthesize"},
				"model": map[string]any{"type": "string", "description": "TTS model to use (optional)"},
				"voice": map[string]any{"type": "string", "description": "Voice name (optional)"},
			},
			"required": []string{"text"},
		},
	}
}

// callBody represents request fields accepted by many OpenAI-compatible TTS
// endpoints. We send a simple JSON payload; some gateways may require
// multipart/form-data instead. Adjust if needed for a specific gateway.
type callBody struct {
	Model string `json:"model,omitempty"`
	Voice string `json:"voice,omitempty"`
	Input string `json:"input"`
}

func (t *Tool) Call(ctx context.Context, raw json.RawMessage) (any, error) {
	logger := observability.LoggerWithTrace(ctx)
	// Parse args
	var args map[string]any
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	text, _ := args["text"].(string)
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("text is required")
	}
	modelArg, _ := args["model"].(string)
	voice, _ := args["voice"].(string)

	// Apply defaults from config when not provided in-call
	model := modelArg
	if model == "" {
		model = t.cfg.TTS.Model
	}
	if model == "" {
		model = "gpt-4o-mini-tts"
	}
	if voice == "" {
		voice = t.cfg.TTS.Voice
	}

	// Determine base URL and API key
	// For TTS, we prioritize the explicit TTS configuration over provider context
	// since TTS might use a different endpoint than the main LLM
	baseURL := t.cfg.TTS.BaseURL
	apiKey := t.cfg.OpenAI.APIKey

	logger.Debug().Str("config_tts_baseURL", t.cfg.TTS.BaseURL).Str("config_openai_baseURL", t.cfg.OpenAI.BaseURL).Msg("tts_config_urls")

	// Only fall back to OpenAI config if no TTS-specific config is provided
	if baseURL == "" {
		baseURL = t.cfg.OpenAI.BaseURL
		logger.Debug().Str("using_openai_baseURL", baseURL).Msg("tts_fallback_to_openai")
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com"
		logger.Debug().Msg("tts_using_default_openai")
	}

	logger.Debug().Str("final_baseURL", baseURL).Msg("tts_request")

	// Build request URL (ensure no double slashes)
	reqURL := strings.TrimRight(baseURL, "/") + "/v1/audio/speech"

	body := callBody{Model: model, Voice: voice, Input: text}
	b, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(string(b)))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// SDK may set auth header when using option.WithAPIKey only on its own requests;
	// here we set Authorization explicitly for the raw http request.
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tts request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return nil, fmt.Errorf("tts server error: %d %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	// Read binary audio
	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read audio: %w", err)
	}

	// Save audio to ./tmp folder
	if err := os.MkdirAll("./tmp", 0755); err != nil {
		return nil, fmt.Errorf("create tmp directory: %w", err)
	}

	// Detect actual audio format from content
	actualFormat := "wav"
	if len(audio) >= 4 {
		// Check for WAV signature (RIFF)
		if string(audio[0:4]) == "RIFF" && len(audio) >= 12 && string(audio[8:12]) == "WAVE" {
			actualFormat = "wav"
			logger.Debug().Str("detected_format", "wav").Msg("tts_format_detection")
		} else if len(audio) >= 3 && (audio[0] == 0xFF && audio[1] == 0xFB) ||
			(audio[0] == 0xFF && audio[1] == 0xFA) ||
			(string(audio[0:3]) == "ID3") {
			actualFormat = "mp3"
			logger.Debug().Str("detected_format", "mp3").Msg("tts_format_detection")
		}
	}

	// Generate filename with timestamp and actual format
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("tts_%s.%s", timestamp, actualFormat)
	filepath := filepath.Join("./tmp", filename)

	if err := os.WriteFile(filepath, audio, 0644); err != nil {
		return nil, fmt.Errorf("save audio file: %w", err)
	}

	logger.Info().Str("file", filepath).Msg("tts_audio_saved")

	return map[string]any{
		"ok":        true,
		"file_path": filepath,
		"filename":  filename,
	}, nil
}
