package supertonic

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

func TestPreprocessAndTokenize(t *testing.T) {
	md := os.Getenv("SUPERTONIC_MODEL_DIR")
	if md == "" {
		t.Skip("set SUPERTONIC_MODEL_DIR")
	}
	pre, ok := preprocessText("Hello there.", "en")
	if !ok {
		t.Fatal("preprocess failed")
	}
	if pre != "<en>Hello there.</en>" {
		t.Fatalf("preprocess = %q", pre)
	}
	idx, err := loadIndexer(md)
	if err != nil {
		t.Fatal(err)
	}
	ids, mask := tokenize(idx, pre)
	want := []int64{29, 64, 73, 31, 40, 64, 71, 71, 74, 2, 79, 67, 64, 77, 64, 15, 29, 16, 64, 73, 31}
	if len(ids) != len(want) {
		t.Fatalf("len ids = %d want %d", len(ids), len(want))
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ids[%d] = %d want %d (got %v)", i, ids[i], want[i], ids)
		}
	}
	for i, m := range mask {
		if m != 1.0 {
			t.Fatalf("mask[%d] = %v, want 1", i, m)
		}
	}
}

type refDoc struct {
	Inputs  map[string]struct{ Data []float64 } `json:"inputs"`
	Outputs map[string]struct{ Data []float64 } `json:"outputs"`
}

func loadRefFloats(t *testing.T, path, section, name string) []float32 {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var d refDoc
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	var data []float64
	if section == "inputs" {
		data = d.Inputs[name].Data
	} else {
		data = d.Outputs[name].Data
	}
	out := make([]float32, len(data))
	for i, v := range data {
		out[i] = float32(v)
	}
	return out
}

// TestSynthesizeDeterministic injects the captured initial noise and checks the
// full pipeline reproduces the captured audio (RNG otherwise makes output
// non-repeatable). This validates tokenization + dp + text_encoder + the
// flow-matching loop + vocoder + slicing end-to-end.
func TestSynthesizeDeterministic(t *testing.T) {
	md := os.Getenv("SUPERTONIC_MODEL_DIR")
	veRef := os.Getenv("SUPERTONIC_VE_REF")
	vocRef := os.Getenv("SUPERTONIC_VOC_REF")
	if md == "" || veRef == "" || vocRef == "" {
		t.Skip("set SUPERTONIC_MODEL_DIR, SUPERTONIC_VE_REF, SUPERTONIC_VOC_REF")
	}
	noise := loadRefFloats(t, veRef, "inputs", "noisy_latent")
	wantWav := loadRefFloats(t, vocRef, "outputs", "wav_tts")

	tts, err := New(md)
	if err != nil {
		t.Fatal(err)
	}
	samples, err := tts.Synthesize("Hello there.", "M1", Options{injectNoise: noise})
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) == 0 {
		t.Fatal("no samples")
	}
	// samples are the sliced output; wantWav is the raw vocoder output. Compare
	// over the sliced length.
	n := len(samples)
	if n > len(wantWav) {
		n = len(wantWav)
	}
	var maxAbs float64
	for i := 0; i < n; i++ {
		if d := math.Abs(float64(samples[i] - wantWav[i])); d > maxAbs {
			maxAbs = d
		}
	}
	if maxAbs > 2e-3 {
		t.Fatalf("waveform diverges: maxAbs=%g over %d samples", maxAbs, n)
	}
	t.Logf("deterministic pipeline OK: %d samples, maxAbs=%g", n, maxAbs)
}
