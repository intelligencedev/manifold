package tts

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
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

	"manifold/internal/config"
	"manifold/internal/observability"
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
				"text":   map[string]any{"type": "string", "description": "Text to synthesize"},
				"model":  map[string]any{"type": "string", "description": "TTS model to use (optional)"},
				"voice":  map[string]any{"type": "string", "description": "Voice name (optional)"},
				"stream": map[string]any{"type": "boolean", "description": "If true, stream audio chunks (SSE) and return final file when complete"},
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

// streaming context key + helpers -------------------------------------------------
type streamChunkKey struct{}

// WithStreamChunkCallback stores a per-chunk callback into ctx. The callback receives raw audio bytes
// for each streamed audio chunk (already decoded from hex/base64). Tools that support streaming will
// invoke it as data arrives.
func WithStreamChunkCallback(ctx context.Context, cb func([]byte)) context.Context {
	return context.WithValue(ctx, streamChunkKey{}, cb)
}

// getStreamChunkCallback retrieves the streaming chunk callback if present.
func getStreamChunkCallback(ctx context.Context) func([]byte) {
	v := ctx.Value(streamChunkKey{})
	if v == nil {
		return nil
	}
	if cb, ok := v.(func([]byte)); ok {
		return cb
	}
	return nil
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
	streamFlag, _ := args["stream"].(bool)

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

	// Build request URL (ensure no double slashes). For streaming requests assume provider exposes /v1/audio/speech/stream
	base := strings.TrimRight(baseURL, "/")
	reqURL := base + "/v1/audio/speech"
	if streamFlag {
		reqURL = base + "/v1/audio/speech/stream"
	}

	body := callBody{Model: model, Voice: voice, Input: text}
	b, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(string(b)))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if streamFlag {
		// Request SSE / chunked stream where supported
		req.Header.Set("Accept", "text/event-stream")
	}
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
	// If not streaming, read entire body same as before
	if !streamFlag {
		audio, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read audio: %w", err)
		}
		return t.saveFinalAudio(ctx, audio)
	}

	// Streaming path: parse SSE style events line-by-line.
	cb := getStreamChunkCallback(ctx)
	reader := bufio.NewReader(resp.Body)

	// Aggregation state
	type agg struct {
		frames        bytes.Buffer // raw PCM frames
		channels      uint16
		sampleRate    uint32
		bitsPerSample uint16
		format        string
		initialized   bool
	}
	var A agg

	parseWAVChunk := func(buf []byte) ([]byte, uint16, uint32, uint16, error) {
		// Minimal WAV parsing; expect standard PCM header
		if len(buf) < 44 || string(buf[0:4]) != "RIFF" || string(buf[8:12]) != "WAVE" {
			return buf, 1, 16000, 16, nil // Assume raw frames if not a WAV; fallback
		}
		offset := 12
		var channels uint16 = 1
		var sampleRate uint32 = 16000
		var bits uint16 = 16
		var dataStart, dataLen int
		for offset+8 <= len(buf) {
			ckID := string(buf[offset : offset+4])
			ckSize := int(binary.LittleEndian.Uint32(buf[offset+4 : offset+8]))
			if offset+8+ckSize > len(buf) { // malformed
				break
			}
			if ckID == "fmt " {
				if ckSize >= 16 {
					channels = binary.LittleEndian.Uint16(buf[offset+10 : offset+12])
					sampleRate = binary.LittleEndian.Uint32(buf[offset+12 : offset+16])
					bits = binary.LittleEndian.Uint16(buf[offset+22 : offset+24])
				}
			} else if ckID == "data" {
				dataStart = offset + 8
				dataLen = ckSize
				break
			}
			offset += 8 + ckSize
		}
		if dataLen == 0 || dataStart+dataLen > len(buf) { // fallback treat entire buf as frames
			return buf, channels, sampleRate, bits, nil
		}
		return buf[dataStart : dataStart+dataLen], channels, sampleRate, bits, nil
	}

	finalize := func() (any, error) {
		// Build a single WAV from aggregated frames
		frames := A.frames.Bytes()
		if len(frames) == 0 { // Nothing received
			return t.saveFinalAudio(ctx, frames) // fallback (will still write small file)
		}
		// Construct WAV header
		dataSize := uint32(len(frames))
		var hdr bytes.Buffer
		// RIFF chunk
		hdr.WriteString("RIFF")
		binary.Write(&hdr, binary.LittleEndian, uint32(36+dataSize))
		hdr.WriteString("WAVE")
		// fmt chunk
		hdr.WriteString("fmt ")
		binary.Write(&hdr, binary.LittleEndian, uint32(16))   // PCM fmt chunk size
		binary.Write(&hdr, binary.LittleEndian, uint16(1))    // PCM
		binary.Write(&hdr, binary.LittleEndian, A.channels)   // channels
		binary.Write(&hdr, binary.LittleEndian, A.sampleRate) // sampleRate
		byteRate := A.sampleRate * uint32(A.channels) * uint32(A.bitsPerSample) / 8
		blockAlign := A.channels * A.bitsPerSample / 8
		binary.Write(&hdr, binary.LittleEndian, byteRate)
		binary.Write(&hdr, binary.LittleEndian, blockAlign)
		binary.Write(&hdr, binary.LittleEndian, A.bitsPerSample)
		// data chunk
		hdr.WriteString("data")
		binary.Write(&hdr, binary.LittleEndian, dataSize)
		hdr.Write(frames)
		return t.saveFinalAudio(ctx, hdr.Bytes())
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("stream read: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" || strings.HasPrefix(line, ":") { // skip empty & comment/ping lines
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue // ignore 'event:' lines; rely solely on data JSON
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var evt map[string]any
		if err := json.Unmarshal([]byte(payload), &evt); err != nil {
			continue
		}
		// Chunk detection: presence of 'audio'
		if aRaw, ok := evt["audio"].(string); ok && aRaw != "" {
			enc := "base64"
			if e2, ok2 := evt["encoding"].(string); ok2 && e2 != "" {
				enc = e2
			}
			// decode
			var data []byte
			if enc == "base64" {
				bts, err := base64.StdEncoding.DecodeString(aRaw)
				if err == nil {
					data = bts
				}
			} else if enc == "hex" {
				// Keep simple: hex decoding not implemented; skip if not base64
			}
			if len(data) > 0 {
				frames, ch, sr, bits, _ := parseWAVChunk(data)
				if !A.initialized {
					A.channels = ch
					A.sampleRate = sr
					A.bitsPerSample = bits
					A.format = "wav"
					A.initialized = true
				}
				A.frames.Write(frames)
				if cb != nil {
					cb(frames)
				}
			}
		}
		// Completion indicator
		if st, ok := evt["status"].(string); ok && strings.HasPrefix(st, "complete") {
			return finalize()
		}
		if _, ok := evt["final"].(bool); ok { // alternative final flag
			return finalize()
		}
	}
	return finalize()
}

// saveFinalAudio infers format, writes file, returns standard response map
func (t *Tool) saveFinalAudio(ctx context.Context, audio []byte) (any, error) {
	logger := observability.LoggerWithTrace(ctx)
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
	logger.Info().Str("file", filepath).Int("bytes", len(audio)).Msg("tts_audio_saved")
	return map[string]any{"ok": true, "file_path": filepath, "filename": filename}, nil
}
