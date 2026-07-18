package agentd

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/intelligencedev/born/moonshine"
	"github.com/rs/zerolog/log"
)

// moonshineHolder lazily loads the in-process pure-Go Moonshine STT engine once
// per process; load failures are cached.
type moonshineHolder struct {
	once sync.Once
	stt  *moonshine.STT
	err  error
}

func (h *moonshineHolder) get(modelDir string) (*moonshine.STT, error) {
	h.once.Do(func() { h.stt, h.err = moonshine.New(modelDir) })
	return h.stt, h.err
}

// sttMoonshineResponse serves /stt entirely in-process: multipart WAV in,
// OpenAI-transcription-shaped JSON {"text": ...} out. Mirrors the proxy path's
// external contract so clients (Reachy bot, UI) cannot tell the engines apart.
func (a *app) sttMoonshineResponse(w http.ResponseWriter, r *http.Request, data []byte) {
	modelDir := strings.TrimSpace(a.cfg.STT.ModelDir)
	if modelDir == "" {
		http.Error(w, "stt.modelDir not configured for moonshine engine", http.StatusServiceUnavailable)
		return
	}
	stt, err := a.moonshineSTT.get(modelDir)
	if err != nil {
		log.Warn().Err(err).Str("model_dir", modelDir).Msg("moonshine_stt_load_failed")
		http.Error(w, "moonshine stt engine failed to load", http.StatusServiceUnavailable)
		return
	}
	audio, err := wavToMono16k(data)
	if err != nil {
		http.Error(w, "bad audio: "+err.Error(), http.StatusBadRequest)
		return
	}
	text, err := stt.Transcribe(audio)
	if err != nil {
		log.Warn().Err(err).Msg("moonshine_stt_transcribe_failed")
		http.Error(w, "transcription failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"text": text})
}

// wavToMono16k decodes a PCM16 RIFF/WAVE payload (any rate, mono or stereo) to
// mono float32 resampled to moonshine.SampleRate via linear interpolation.
func wavToMono16k(b []byte) ([]float32, error) {
	if len(b) < 44 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, fmt.Errorf("not a RIFF/WAVE file")
	}
	var rate, channels, bits int
	var pcm []byte
	for off := 12; off+8 <= len(b); {
		id := string(b[off : off+4])
		sz := int(binary.LittleEndian.Uint32(b[off+4 : off+8]))
		body := off + 8
		if body+sz > len(b) {
			sz = len(b) - body
		}
		switch id {
		case "fmt ":
			if sz >= 16 {
				channels = int(binary.LittleEndian.Uint16(b[body+2 : body+4]))
				rate = int(binary.LittleEndian.Uint32(b[body+4 : body+8]))
				bits = int(binary.LittleEndian.Uint16(b[body+14 : body+16]))
			}
		case "data":
			pcm = b[body : body+sz]
		}
		off = body + sz + (sz & 1)
	}
	if rate <= 0 || channels <= 0 || pcm == nil {
		return nil, fmt.Errorf("missing fmt/data chunks")
	}
	if bits != 16 {
		return nil, fmt.Errorf("only PCM16 supported, got %d-bit", bits)
	}
	n := len(pcm) / 2 / channels
	mono := make([]float32, n)
	for i := 0; i < n; i++ {
		var sum int
		for c := 0; c < channels; c++ {
			sum += int(int16(binary.LittleEndian.Uint16(pcm[(i*channels+c)*2:])))
		}
		mono[i] = float32(sum) / float32(channels) / 32768.0
	}
	if rate == moonshine.SampleRate || n == 0 {
		return mono, nil
	}
	m := int(int64(n) * moonshine.SampleRate / int64(rate))
	out := make([]float32, m)
	for i := 0; i < m; i++ {
		pos := float64(i) * float64(n-1) / float64(maxInt(m-1, 1))
		lo := int(pos)
		frac := float32(pos - float64(lo))
		hi := lo + 1
		if hi >= n {
			hi = n - 1
		}
		out[i] = mono[lo]*(1-frac) + mono[hi]*frac
	}
	return out, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
