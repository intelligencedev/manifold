//go:build !wasm

package onnx

import (
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/born-ml/born/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

type dpRef struct {
	TextIds       []int64   `json:"text_ids"`
	TextIdsShape  []int     `json:"text_ids_shape"`
	StyleDp       []float32 `json:"style_dp"`
	StyleDpShape  []int     `json:"style_dp_shape"`
	TextMask      []float32 `json:"text_mask"`
	TextMaskShape []int     `json:"text_mask_shape"`
	Duration      []float32 `json:"duration"`
}

func mustF32(t *testing.T, shape []int, data []float32) *tensor.RawTensor {
	t.Helper()
	raw, err := tensor.NewRaw(tensor.Shape(shape), tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatalf("NewRaw f32: %v", err)
	}
	copy(raw.AsFloat32(), data)
	return raw
}

func mustI64(t *testing.T, shape []int, data []int64) *tensor.RawTensor {
	t.Helper()
	raw, err := tensor.NewRaw(tensor.Shape(shape), tensor.Int64, tensor.CPU)
	if err != nil {
		t.Fatalf("NewRaw i64: %v", err)
	}
	copy(raw.AsInt64(), data)
	return raw
}

// TestDurationPredictorParity loads the real duration_predictor graph in strict
// mode (every op must be implemented) and checks Born's output matches the
// onnxruntime reference in SUPERTONIC_DP_REF for identical inputs.
func TestDurationPredictorParity(t *testing.T) {
	modelDir := os.Getenv("SUPERTONIC_MODEL_DIR")
	refPath := os.Getenv("SUPERTONIC_DP_REF")
	if modelDir == "" || refPath == "" {
		t.Skip("set SUPERTONIC_MODEL_DIR and SUPERTONIC_DP_REF")
	}
	raw, err := os.ReadFile(refPath)
	if err != nil {
		t.Fatalf("read ref: %v", err)
	}
	var ref dpRef
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatalf("parse ref: %v", err)
	}

	opts := DefaultLoadOptions()
	opts.StrictMode = true
	model, err := Load(modelDir+"/onnx/duration_predictor.onnx", cpu.New(), opts)
	if err != nil {
		t.Fatalf("strict Load failed: %v", err)
	}

	outputs, err := model.ForwardNamed(map[string]*tensor.RawTensor{
		"text_ids":  mustI64(t, ref.TextIdsShape, ref.TextIds),
		"style_dp":  mustF32(t, ref.StyleDpShape, ref.StyleDp),
		"text_mask": mustF32(t, ref.TextMaskShape, ref.TextMask),
	})
	if err != nil {
		t.Fatalf("ForwardNamed: %v", err)
	}
	out, ok := outputs["duration"]
	if !ok {
		t.Fatalf("no 'duration' output; got keys %v", keysOf(outputs))
	}
	got := out.AsFloat32()
	if len(got) != len(ref.Duration) {
		t.Fatalf("duration len: got %d want %d", len(got), len(ref.Duration))
	}
	for i := range ref.Duration {
		if d := math.Abs(float64(got[i] - ref.Duration[i])); d > 1e-3 {
			t.Fatalf("duration[%d]: got %v want %v (|Δ|=%g)", i, got[i], ref.Duration[i], d)
		}
	}
	t.Logf("duration parity OK: got=%v ref=%v", got, ref.Duration)
}

func keysOf(m map[string]*tensor.RawTensor) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
