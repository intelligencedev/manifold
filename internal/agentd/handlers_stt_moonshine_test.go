package agentd

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"manifold/internal/config"
)

// Env-gated live test: MOONSHINE_MODEL_DIR + MOONSHINE_REF (audio + text).
func TestSTTMoonshineLive(t *testing.T) {
	md, ref := os.Getenv("MOONSHINE_MODEL_DIR"), os.Getenv("MOONSHINE_REF")
	if md == "" || ref == "" {
		t.Skip("set MOONSHINE_MODEL_DIR and MOONSHINE_REF")
	}
	raw, err := os.ReadFile(ref)
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		Audio []float32 `json:"audio"`
		Text  string    `json:"text"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	// Build a 16k mono PCM16 WAV from the reference audio.
	pcm := make([]byte, len(r.Audio)*2)
	for i, v := range r.Audio {
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(int16(v*32767)))
	}
	var wavBuf bytes.Buffer
	wavBuf.WriteString("RIFF")
	binary.Write(&wavBuf, binary.LittleEndian, uint32(36+len(pcm)))
	wavBuf.WriteString("WAVEfmt ")
	binary.Write(&wavBuf, binary.LittleEndian, uint32(16))
	binary.Write(&wavBuf, binary.LittleEndian, uint16(1))
	binary.Write(&wavBuf, binary.LittleEndian, uint16(1))
	binary.Write(&wavBuf, binary.LittleEndian, uint32(16000))
	binary.Write(&wavBuf, binary.LittleEndian, uint32(32000))
	binary.Write(&wavBuf, binary.LittleEndian, uint16(2))
	binary.Write(&wavBuf, binary.LittleEndian, uint16(16))
	wavBuf.WriteString("data")
	binary.Write(&wavBuf, binary.LittleEndian, uint32(len(pcm)))
	wavBuf.Write(pcm)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("audio", "test.wav")
	fw.Write(wavBuf.Bytes())
	mw.Close()

	a := &app{
		cfg:        &config.Config{STT: config.STTConfig{Engine: "moonshine", ModelDir: md}},
		httpClient: http.DefaultClient,
	}
	req := httptest.NewRequest(http.MethodPost, "/stt", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	a.sttHandler()(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Text != r.Text {
		t.Fatalf("text %q want %q", out.Text, r.Text)
	}
	t.Logf("STT live: %q", out.Text)
}
