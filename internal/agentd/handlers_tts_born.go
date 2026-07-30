package agentd

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/intelligencedev/born/supertonic"
	"github.com/rs/zerolog/log"
)

// bornTTSHolder lazily loads the in-process pure-Go Supertonic engine once per
// process. The zero value is usable; failures are cached so every request does
// not retry a broken model directory.
type bornTTSHolder struct {
	once sync.Once
	tts  *supertonic.TTS
	err  error
}

func (h *bornTTSHolder) get(modelDir string) (*supertonic.TTS, error) {
	h.once.Do(func() {
		h.tts, h.err = supertonic.New(modelDir)
		if h.err == nil {
			log.Info().
				Str("backend", h.tts.BackendName()).
				Str("model_dir", modelDir).
				Msg("born_tts_loaded")
		}
	})
	return h.tts, h.err
}

func (h *bornTTSHolder) close() {
	if h != nil && h.tts != nil {
		h.tts.Close()
	}
}

// ttsBornResponse serves /tts entirely in-process via the Born Supertonic
// engine (services/born-supertonic-fork), streaming the same SSE tts_chunk
// frames (base64 PCM16 + rate) the sidecar proxy path emits, so clients like
// the Reachy Mini bot cannot tell the engines apart.
func (a *app) ttsBornResponse(w http.ResponseWriter, r *http.Request, req ttsRequest) {
	modelDir := strings.TrimSpace(a.cfg.TTS.ModelDir)
	if modelDir == "" {
		http.Error(w, "tts.modelDir not configured for born engine", http.StatusServiceUnavailable)
		return
	}
	tts, err := a.bornTTS.get(modelDir)
	if err != nil {
		log.Warn().Err(err).Str("model_dir", modelDir).Msg("born_tts_load_failed")
		http.Error(w, "born tts engine failed to load", http.StatusServiceUnavailable)
		return
	}

	voice := strings.TrimSpace(req.Voice)
	if voice == "" {
		voice = strings.TrimSpace(a.cfg.TTS.Voice)
	}
	if voice == "" {
		voice = "M1"
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	writeFrame := func(m map[string]any) {
		data, _ := json.Marshal(m)
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(data)
		_, _ = w.Write([]byte("\n\n"))
		flusher.Flush()
	}

	rate := tts.SampleRate()
	seq := 0
	err = tts.SynthesizeStream(req.Text, voice, bornTTSOptions(req),
		func(wav []float32) error {
			if ctxErr := r.Context().Err(); ctxErr != nil {
				return ctxErr // client went away (barge-in); stop synthesizing
			}
			pcm := supertonic.FloatToPCM16(wav)
			// Sub-chunk so no SSE line grows beyond ~43KB of base64: a whole
			// sentence group can be megabytes, and line-based consumers often
			// cap line length (bufio.Scanner defaults to 64KB).
			const frameBytes = 32 * 1024
			for off := 0; off < len(pcm); off += frameBytes {
				end := off + frameBytes
				if end > len(pcm) {
					end = len(pcm)
				}
				writeFrame(map[string]any{
					"type":  "tts_chunk",
					"bytes": end - off,
					"b64":   base64.StdEncoding.EncodeToString(pcm[off:end]),
					"rate":  rate,
					"seq":   seq,
				})
				seq++
			}
			return nil
		})
	if err != nil {
		// Headers are already sent; signal failure in-band and omit the done
		// frame so clients do not mistake a truncated stream for success.
		log.Warn().Err(err).Str("voice", voice).Msg("born_tts_synthesize_failed")
		writeFrame(map[string]any{"type": "error", "error": err.Error()})
		return
	}
	writeFrame(map[string]any{"type": "done", "rate": rate})
}

func bornTTSOptions(req ttsRequest) supertonic.Options {
	opts := supertonic.Options{Lang: strings.TrimSpace(req.Lang)}
	if req.TotalSteps >= 1 && req.TotalSteps <= 20 {
		opts.TotalSteps = req.TotalSteps
	}
	if req.Speed >= 0.7 && req.Speed <= 1.6 {
		opts.Speed = req.Speed
	}
	return opts
}
