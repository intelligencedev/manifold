package supertonic

import (
	"fmt"
	"math"
	"math/rand"
	"path/filepath"

	"github.com/born-ml/born/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
	"github.com/born-ml/born/onnx"
)

// TTS is a loaded Supertonic pipeline.
type TTS struct {
	cfg       Config
	indexer   []int64
	dp        onnx.Model
	textEnc   onnx.Model
	vectorEst onnx.Model
	vocoder   onnx.Model
	modelDir  string
	styles    map[string]VoiceStyle
}

// Options configures a synthesis call. Zero values fall back to the defaults
// used by the browser reference.
type Options struct {
	Lang            string
	TotalSteps      int
	Speed           float64
	SilenceDuration float64

	// injectNoise, when set, replaces sampled Gaussian noise (for deterministic
	// tests). Length must equal latentDimVal*latentLen of the (single) chunk.
	injectNoise []float32
}

// New loads the config, tokenizer, and the 4 ONNX graphs from modelDir.
func New(modelDir string) (*TTS, error) {
	cfg, err := loadConfig(modelDir)
	if err != nil {
		return nil, err
	}
	indexer, err := loadIndexer(modelDir)
	if err != nil {
		return nil, err
	}
	backend := cpu.New()
	opts := onnx.DefaultLoadOptions()
	opts.StrictMode = true
	load := func(name string) (onnx.Model, error) {
		return onnx.Load(filepath.Join(modelDir, "onnx", name), backend, opts)
	}
	dp, err := load("duration_predictor.onnx")
	if err != nil {
		return nil, fmt.Errorf("load dp: %w", err)
	}
	te, err := load("text_encoder.onnx")
	if err != nil {
		return nil, fmt.Errorf("load text_encoder: %w", err)
	}
	ve, err := load("vector_estimator.onnx")
	if err != nil {
		return nil, fmt.Errorf("load vector_estimator: %w", err)
	}
	voc, err := load("vocoder.onnx")
	if err != nil {
		return nil, fmt.Errorf("load vocoder: %w", err)
	}
	return &TTS{
		cfg: cfg, indexer: indexer, dp: dp, textEnc: te, vectorEst: ve,
		vocoder: voc, modelDir: modelDir, styles: map[string]VoiceStyle{},
	}, nil
}

// SampleRate returns the output sample rate (Hz).
func (t *TTS) SampleRate() int { return t.cfg.SampleRate }

func (t *TTS) style(voiceID string) (VoiceStyle, error) {
	if s, ok := t.styles[voiceID]; ok {
		return s, nil
	}
	s, err := loadVoiceStyle(t.modelDir, voiceID)
	if err != nil {
		return VoiceStyle{}, err
	}
	t.styles[voiceID] = s
	return s, nil
}

func rawF32(shape []int, data []float32) (*tensor.RawTensor, error) {
	r, err := tensor.NewRaw(tensor.Shape(shape), tensor.Float32, tensor.CPU)
	if err != nil {
		return nil, err
	}
	copy(r.AsFloat32(), data)
	return r, nil
}

func rawI64(shape []int, data []int64) (*tensor.RawTensor, error) {
	r, err := tensor.NewRaw(tensor.Shape(shape), tensor.Int64, tensor.CPU)
	if err != nil {
		return nil, err
	}
	copy(r.AsInt64(), data)
	return r, nil
}

// SynthesizeStream renders text sentence-group by sentence-group, invoking emit
// with each group's samples as soon as they are ready (minimizing
// time-to-first-audio for streaming consumers). Groups after the first are
// prefixed with opts.SilenceDuration of silence, so concatenating every emitted
// slice reproduces exactly what Synthesize returns. emit returning an error
// aborts the stream.
func (t *TTS) SynthesizeStream(text, voiceID string, opts Options, emit func(wav []float32) error) error {
	if opts.Lang == "" {
		opts.Lang = "en"
	}
	if opts.TotalSteps == 0 {
		opts.TotalSteps = 8
	}
	if opts.Speed == 0 {
		opts.Speed = 1.05
	}
	if opts.SilenceDuration == 0 {
		opts.SilenceDuration = 0.3
	}
	style, err := t.style(voiceID)
	if err != nil {
		return err
	}

	maxLen := 300
	if opts.Lang == "ko" || opts.Lang == "ja" {
		maxLen = 120
	}
	silence := make([]float32, int(opts.SilenceDuration*float64(t.cfg.SampleRate)))
	for i, chunk := range chunkText(text, maxLen) {
		wav, err := t.infer(chunk, style, opts)
		if err != nil {
			return err
		}
		if i > 0 {
			wav = append(append(make([]float32, 0, len(silence)+len(wav)), silence...), wav...)
		}
		if err := emit(wav); err != nil {
			return err
		}
	}
	return nil
}

// Synthesize renders text to a float32 waveform at SampleRate(). Sentence groups
// are synthesized and concatenated with silence between them.
func (t *TTS) Synthesize(text, voiceID string, opts Options) ([]float32, error) {
	var wavCat []float32
	err := t.SynthesizeStream(text, voiceID, opts, func(wav []float32) error {
		wavCat = append(wavCat, wav...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return wavCat, nil
}

func (t *TTS) infer(chunk string, style VoiceStyle, opts Options) ([]float32, error) {
	pre, ok := preprocessText(chunk, opts.Lang)
	if !ok {
		return nil, fmt.Errorf("invalid language %q", opts.Lang)
	}
	ids, mask := tokenize(t.indexer, pre)
	seq := len(ids)

	textIDs, err := rawI64([]int{1, seq}, ids)
	if err != nil {
		return nil, err
	}
	textMask, err := rawF32([]int{1, 1, seq}, mask)
	if err != nil {
		return nil, err
	}
	styleTTL, err := rawF32(style.TTLDims, style.TTLData)
	if err != nil {
		return nil, err
	}
	styleDP, err := rawF32(style.DPDims, style.DPData)
	if err != nil {
		return nil, err
	}

	// Duration predictor.
	dpOut, err := t.dp.ForwardNamed(map[string]*tensor.RawTensor{
		"text_ids": textIDs, "style_dp": styleDP, "text_mask": textMask,
	})
	if err != nil {
		return nil, fmt.Errorf("dp: %w", err)
	}
	durTensor, ok := dpOut["duration"]
	if !ok {
		return nil, fmt.Errorf("dp: no duration output")
	}
	durationSec := float64(durTensor.AsFloat32()[0]) / opts.Speed

	// Text encoder.
	teOut, err := t.textEnc.ForwardNamed(map[string]*tensor.RawTensor{
		"text_ids": textIDs, "style_ttl": styleTTL, "text_mask": textMask,
	})
	if err != nil {
		return nil, fmt.Errorf("text_encoder: %w", err)
	}
	textEmb, ok := teOut["text_emb"]
	if !ok {
		return nil, fmt.Errorf("text_encoder: no text_emb output")
	}

	// Sample (or inject) the noisy latent.
	latentDimVal := t.cfg.LatentDim * t.cfg.ChunkCompressFactor
	chunkSize := t.cfg.BaseChunkSize * t.cfg.ChunkCompressFactor
	wavLen := int(math.Floor(durationSec * float64(t.cfg.SampleRate)))
	latentLen := (wavLen + chunkSize - 1) / chunkSize
	if latentLen < 1 {
		latentLen = 1
	}

	xt := make([]float32, latentDimVal*latentLen)
	latentMask := make([]float32, latentLen)
	for i := range latentMask {
		latentMask[i] = 1.0 // single sequence, full length
	}
	if opts.injectNoise != nil {
		copy(xt, opts.injectNoise)
	} else {
		for i := range xt {
			u1 := math.Max(0.0001, rand.Float64())
			u2 := rand.Float64()
			xt[i] = float32(math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2))
		}
		// Apply latent mask (all-ones here, but kept for fidelity).
		for d := 0; d < latentDimVal; d++ {
			for tt := 0; tt < latentLen; tt++ {
				xt[d*latentLen+tt] *= latentMask[tt]
			}
		}
	}

	latentMaskT, err := rawF32([]int{1, 1, latentLen}, latentMask)
	if err != nil {
		return nil, err
	}
	totalStepT, err := rawF32([]int{1}, []float32{float32(opts.TotalSteps)})
	if err != nil {
		return nil, err
	}

	// Flow-matching denoising loop.
	for step := 0; step < opts.TotalSteps; step++ {
		xtT, err := rawF32([]int{1, latentDimVal, latentLen}, xt)
		if err != nil {
			return nil, err
		}
		curStepT, err := rawF32([]int{1}, []float32{float32(step)})
		if err != nil {
			return nil, err
		}
		veOut, err := t.vectorEst.ForwardNamed(map[string]*tensor.RawTensor{
			"noisy_latent": xtT, "text_emb": textEmb, "style_ttl": styleTTL,
			"latent_mask": latentMaskT, "text_mask": textMask,
			"current_step": curStepT, "total_step": totalStepT,
		})
		if err != nil {
			return nil, fmt.Errorf("vector_estimator step %d: %w", step, err)
		}
		den, ok := veOut["denoised_latent"]
		if !ok {
			return nil, fmt.Errorf("vector_estimator: no denoised_latent output")
		}
		xt = append(xt[:0:0], den.AsFloat32()...)
	}

	// Vocoder.
	finalXt, err := rawF32([]int{1, latentDimVal, latentLen}, xt)
	if err != nil {
		return nil, err
	}
	vocOut, err := t.vocoder.ForwardNamed(map[string]*tensor.RawTensor{"latent": finalXt})
	if err != nil {
		return nil, fmt.Errorf("vocoder: %w", err)
	}
	wavT, ok := vocOut["wav_tts"]
	if !ok {
		return nil, fmt.Errorf("vocoder: no wav_tts output")
	}
	wav := wavT.AsFloat32()
	if wavLen > 0 && wavLen < len(wav) {
		wav = wav[:wavLen]
	}
	return append([]float32(nil), wav...), nil
}
