package agentd

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"manifold/internal/config"
)

// collectSSEFrames parses "data: {json}\n\n" frames from an SSE body.
func collectSSEFrames(t *testing.T, body string) []map[string]any {
	t.Helper()
	var frames []map[string]any
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 0, 1<<20), 16<<20) // tts_chunk lines can exceed the 64KB default
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		var m map[string]any
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			t.Fatalf("bad SSE json %q: %v", payload, err)
		}
		frames = append(frames, m)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("SSE scan: %v", err)
	}
	return frames
}

func TestTTSHandlerStreamsSupertonicChunksAsSSE(t *testing.T) {
	pcm := []byte("0123456789abcdef") // stand-in PCM bytes

	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Errorf("unexpected sidecar path %q", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["input"] != "hello robot" {
			t.Errorf("input = %v, want 'hello robot'", body["input"])
		}
		if body["response_format"] != "pcm" {
			t.Errorf("response_format = %v, want pcm", body["response_format"])
		}
		w.Header().Set("Content-Type", "audio/pcm")
		w.Header().Set("X-Sample-Rate", "44100")
		w.WriteHeader(http.StatusOK)
		w.Write(pcm[:8])
		w.(http.Flusher).Flush()
		w.Write(pcm[8:])
	}))
	defer sidecar.Close()

	a := &app{
		cfg:        &config.Config{TTS: config.TTSConfig{BaseURL: sidecar.URL + "/v1", Engine: "supertonic"}},
		httpClient: sidecar.Client(),
	}

	req := httptest.NewRequest(http.MethodPost, "/tts", strings.NewReader(`{"text":"hello robot","voice":"M1"}`))
	rec := httptest.NewRecorder()
	a.ttsHandler()(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}

	frames := collectSSEFrames(t, rec.Body.String())
	var audio []byte
	sawDone := false
	for _, f := range frames {
		switch f["type"] {
		case "tts_chunk":
			b64, _ := f["b64"].(string)
			dec, err := base64.StdEncoding.DecodeString(b64)
			if err != nil {
				t.Fatalf("bad b64 in chunk: %v", err)
			}
			audio = append(audio, dec...)
			if rate, ok := f["rate"].(float64); !ok || int(rate) != 44100 {
				t.Errorf("chunk rate = %v, want 44100", f["rate"])
			}
		case "done":
			sawDone = true
		}
	}
	if string(audio) != string(pcm) {
		t.Errorf("reassembled audio = %q, want %q", audio, pcm)
	}
	if !sawDone {
		t.Errorf("missing terminal done frame")
	}
}

// TestTTSHandlerAgainstLiveSidecar exercises the real handler against a running
// Supertonic sidecar. Guarded by SUPERTONIC_LIVE_URL (e.g. http://127.0.0.1:8891/v1).
func TestTTSHandlerAgainstLiveSidecar(t *testing.T) {
	base := os.Getenv("SUPERTONIC_LIVE_URL")
	if base == "" {
		t.Skip("SUPERTONIC_LIVE_URL not set")
	}
	a := &app{
		cfg:        &config.Config{TTS: config.TTSConfig{BaseURL: base, Engine: "supertonic"}},
		httpClient: http.DefaultClient,
	}
	req := httptest.NewRequest(http.MethodPost, "/tts",
		strings.NewReader(`{"text":"Live integration check for the Reachy Mini robot.","voice":"M1"}`))
	rec := httptest.NewRecorder()
	a.ttsHandler()(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	frames := collectSSEFrames(t, rec.Body.String())
	var totalPCM int
	chunks, sawDone := 0, false
	for _, f := range frames {
		switch f["type"] {
		case "tts_chunk":
			chunks++
			b64, _ := f["b64"].(string)
			dec, err := base64.StdEncoding.DecodeString(b64)
			if err != nil {
				t.Fatalf("bad b64: %v", err)
			}
			totalPCM += len(dec)
			if rate, _ := f["rate"].(float64); int(rate) != 44100 {
				t.Errorf("rate = %v, want 44100", f["rate"])
			}
		case "done":
			sawDone = true
		}
	}
	if chunks == 0 || !sawDone {
		t.Fatalf("chunks=%d sawDone=%v, want >0 and true", chunks, sawDone)
	}
	// int16 mono @44100: expect at least ~0.5s of audio for this sentence.
	if minSamples := 44100 / 2; totalPCM/2 < minSamples {
		t.Errorf("got %d samples, want >= %d", totalPCM/2, minSamples)
	}
	t.Logf("live sidecar: %d chunks, %d PCM bytes (%.2fs audio)", chunks, totalPCM, float64(totalPCM/2)/44100.0)
}

func TestTTSBornEngineRequiresModelDir(t *testing.T) {
	a := &app{
		cfg:        &config.Config{TTS: config.TTSConfig{Engine: "born"}},
		httpClient: http.DefaultClient,
	}
	req := httptest.NewRequest(http.MethodPost, "/tts", strings.NewReader(`{"text":"hi"}`))
	rec := httptest.NewRecorder()
	a.ttsHandler()(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (body: %s)", rec.Code, rec.Body.String())
	}
}

// TestTTSBornEngineLive runs the real in-process pure-Go engine end to end.
// Guarded by SUPERTONIC_MODEL_DIR (dir with onnx/ + voice_styles/).
func TestTTSBornEngineLive(t *testing.T) {
	modelDir := os.Getenv("SUPERTONIC_MODEL_DIR")
	if modelDir == "" {
		t.Skip("SUPERTONIC_MODEL_DIR not set")
	}
	a := &app{
		cfg: &config.Config{TTS: config.TTSConfig{
			Engine: "born", ModelDir: modelDir, Voice: "M1",
		}},
		httpClient: http.DefaultClient,
	}
	req := httptest.NewRequest(http.MethodPost, "/tts",
		strings.NewReader(`{"text":"In process synthesis check for the Reachy Mini robot."}`))
	rec := httptest.NewRecorder()
	a.ttsHandler()(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q", ct)
	}
	frames := collectSSEFrames(t, rec.Body.String())
	totalPCM, chunks, sawDone := 0, 0, false
	for _, f := range frames {
		switch f["type"] {
		case "tts_chunk":
			chunks++
			b64, _ := f["b64"].(string)
			dec, err := base64.StdEncoding.DecodeString(b64)
			if err != nil {
				t.Fatalf("bad b64: %v", err)
			}
			totalPCM += len(dec)
			if rate, _ := f["rate"].(float64); int(rate) != 44100 {
				t.Errorf("rate = %v, want 44100", f["rate"])
			}
		case "done":
			sawDone = true
		}
	}
	if chunks == 0 || !sawDone {
		t.Fatalf("chunks=%d sawDone=%v", chunks, sawDone)
	}
	if minSamples := 44100 / 2; totalPCM/2 < minSamples {
		t.Errorf("only %d samples", totalPCM/2)
	}
	t.Logf("born in-process: %d chunks, %.2fs audio", chunks, float64(totalPCM/2)/44100.0)
}

func TestTTSHandlerRejectsNonPost(t *testing.T) {
	a := &app{cfg: &config.Config{}, httpClient: http.DefaultClient}
	req := httptest.NewRequest(http.MethodGet, "/tts", nil)
	rec := httptest.NewRecorder()
	a.ttsHandler()(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestTTSHandlerErrorsWhenNotConfigured(t *testing.T) {
	a := &app{cfg: &config.Config{TTS: config.TTSConfig{}}, httpClient: http.DefaultClient}
	req := httptest.NewRequest(http.MethodPost, "/tts", strings.NewReader(`{"text":"hi"}`))
	rec := httptest.NewRecorder()
	a.ttsHandler()(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}
