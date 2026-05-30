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
	args, err := parseCallArgs(raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Text) == "" {
		return nil, fmt.Errorf("text is required")
	}
	request := t.callRequest(args)

	logger.Debug().Str("config_tts_baseURL", t.cfg.TTS.BaseURL).Str("config_openai_baseURL", t.cfg.OpenAI.BaseURL).Msg("tts_config_urls")
	logger.Debug().Str("final_baseURL", request.baseURL).Msg("tts_request")
	req, err := request.httpRequest(ctx)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
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
	if !args.Stream {
		audio, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read audio: %w", err)
		}
		return t.saveFinalAudio(ctx, audio)
	}
	return t.saveStreamedAudio(ctx, resp.Body)
}

type callArgs struct {
	Text   string `json:"text"`
	Model  string `json:"model"`
	Voice  string `json:"voice"`
	Stream bool   `json:"stream"`
}

func parseCallArgs(raw json.RawMessage) (callArgs, error) {
	var args callArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return callArgs{}, fmt.Errorf("invalid args: %w", err)
	}
	return args, nil
}

type callRequest struct {
	url     string
	body    callBody
	apiKey  string
	stream  bool
	baseURL string
}

func (t *Tool) callRequest(args callArgs) callRequest {
	model := args.Model
	if model == "" {
		model = t.cfg.TTS.Model
	}
	if model == "" {
		model = "gpt-4o-mini-tts"
	}
	voice := args.Voice
	if voice == "" {
		voice = t.cfg.TTS.Voice
	}
	baseURL := t.cfg.TTS.BaseURL
	if baseURL == "" {
		baseURL = t.cfg.OpenAI.BaseURL
	}
	if baseURL == "" {
		baseURL = config.OpenAIAPIBaseURL
	}
	base := strings.TrimRight(baseURL, "/")
	url := base + "/v1/audio/speech"
	if args.Stream {
		url = base + "/v1/audio/speech/stream"
	}
	return callRequest{
		url:     url,
		body:    callBody{Model: model, Voice: voice, Input: args.Text},
		apiKey:  t.cfg.OpenAI.APIKey,
		stream:  args.Stream,
		baseURL: baseURL,
	}
}

func (r callRequest) httpRequest(ctx context.Context) (*http.Request, error) {
	b, _ := json.Marshal(r.body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	if r.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+r.apiKey)
	}
	return req, nil
}

func (t *Tool) saveStreamedAudio(ctx context.Context, body io.Reader) (any, error) {
	cb := getStreamChunkCallback(ctx)
	reader := bufio.NewReader(body)
	agg := newStreamAudioAggregator(cb)

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
		evt, ok := decodeStreamEvent(payload)
		if !ok {
			continue
		}
		agg.addEvent(evt)
		if st, ok := evt["status"].(string); ok && strings.HasPrefix(st, "complete") {
			return t.saveFinalAudio(ctx, agg.finalAudio())
		}
		if _, ok := evt["final"].(bool); ok { // alternative final flag
			return t.saveFinalAudio(ctx, agg.finalAudio())
		}
	}
	return t.saveFinalAudio(ctx, agg.finalAudio())
}

func decodeStreamEvent(payload string) (map[string]any, bool) {
	var evt map[string]any
	if err := json.Unmarshal([]byte(payload), &evt); err != nil {
		return nil, false
	}
	return evt, true
}

type streamAudioAggregator struct {
	frames        bytes.Buffer
	channels      uint16
	sampleRate    uint32
	bitsPerSample uint16
	initialized   bool
	callback      func([]byte)
}

func newStreamAudioAggregator(callback func([]byte)) *streamAudioAggregator {
	return &streamAudioAggregator{channels: 1, sampleRate: 16000, bitsPerSample: 16, callback: callback}
}

func (a *streamAudioAggregator) addEvent(evt map[string]any) {
	raw, ok := evt["audio"].(string)
	if !ok || raw == "" {
		return
	}
	data := decodeAudioChunk(raw, evt)
	if len(data) == 0 {
		return
	}
	frames, channels, sampleRate, bits, err := parseWAVChunk(data)
	if err != nil {
		return
	}
	if !a.initialized {
		a.channels = channels
		a.sampleRate = sampleRate
		a.bitsPerSample = bits
		a.initialized = true
	}
	a.frames.Write(frames)
	if a.callback != nil {
		a.callback(frames)
	}
}

func decodeAudioChunk(raw string, evt map[string]any) []byte {
	encoding := "base64"
	if value, ok := evt["encoding"].(string); ok && value != "" {
		encoding = value
	}
	if encoding != "base64" {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil
	}
	return data
}

func (a *streamAudioAggregator) finalAudio() []byte {
	frames := a.frames.Bytes()
	if len(frames) == 0 {
		return frames
	}
	dataSize := uint32(len(frames))
	var hdr bytes.Buffer
	hdr.WriteString("RIFF")
	binary.Write(&hdr, binary.LittleEndian, uint32(36+dataSize))
	hdr.WriteString("WAVE")
	hdr.WriteString("fmt ")
	binary.Write(&hdr, binary.LittleEndian, uint32(16))
	binary.Write(&hdr, binary.LittleEndian, uint16(1))
	binary.Write(&hdr, binary.LittleEndian, a.channels)
	binary.Write(&hdr, binary.LittleEndian, a.sampleRate)
	byteRate := a.sampleRate * uint32(a.channels) * uint32(a.bitsPerSample) / 8
	blockAlign := a.channels * a.bitsPerSample / 8
	binary.Write(&hdr, binary.LittleEndian, byteRate)
	binary.Write(&hdr, binary.LittleEndian, blockAlign)
	binary.Write(&hdr, binary.LittleEndian, a.bitsPerSample)
	hdr.WriteString("data")
	binary.Write(&hdr, binary.LittleEndian, dataSize)
	hdr.Write(frames)
	return hdr.Bytes()
}

func parseWAVChunk(buf []byte) ([]byte, uint16, uint32, uint16, error) {
	if len(buf) < 44 || string(buf[0:4]) != "RIFF" || string(buf[8:12]) != "WAVE" {
		return buf, 1, 16000, 16, nil
	}
	offset := 12
	var channels uint16 = 1
	var sampleRate uint32 = 16000
	var bits uint16 = 16
	var dataStart, dataLen int
	for offset+8 <= len(buf) {
		ckID := string(buf[offset : offset+4])
		ckSize := int(binary.LittleEndian.Uint32(buf[offset+4 : offset+8]))
		if offset+8+ckSize > len(buf) {
			break
		}
		if ckID == "fmt " && ckSize >= 16 {
			channels = binary.LittleEndian.Uint16(buf[offset+10 : offset+12])
			sampleRate = binary.LittleEndian.Uint32(buf[offset+12 : offset+16])
			bits = binary.LittleEndian.Uint16(buf[offset+22 : offset+24])
		} else if ckID == "data" {
			dataStart = offset + 8
			dataLen = ckSize
			break
		}
		offset += 8 + ckSize
	}
	if dataLen == 0 || dataStart+dataLen > len(buf) {
		return buf, channels, sampleRate, bits, nil
	}
	return buf[dataStart : dataStart+dataLen], channels, sampleRate, bits, nil
}

// saveFinalAudio infers format, writes file, returns standard response map
func (t *Tool) saveFinalAudio(ctx context.Context, audio []byte) (any, error) {
	logger := observability.LoggerWithTrace(ctx)
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
