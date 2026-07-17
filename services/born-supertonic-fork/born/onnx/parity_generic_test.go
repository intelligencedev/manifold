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

type genTensor struct {
	DType string    `json:"dtype"`
	Shape []int     `json:"shape"`
	Data  []float64 `json:"data"`
}

type genRef struct {
	Model   string               `json:"model"`
	Inputs  map[string]genTensor `json:"inputs"`
	Outputs map[string]genTensor `json:"outputs"`
}

func toRaw(t *testing.T, g genTensor) *tensor.RawTensor {
	t.Helper()
	shape := tensor.Shape(g.Shape)
	switch g.DType {
	case "int64":
		r, err := tensor.NewRaw(shape, tensor.Int64, tensor.CPU)
		if err != nil {
			t.Fatalf("NewRaw: %v", err)
		}
		d := r.AsInt64()
		for i, v := range g.Data {
			d[i] = int64(v)
		}
		return r
	default:
		r, err := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
		if err != nil {
			t.Fatalf("NewRaw: %v", err)
		}
		d := r.AsFloat32()
		for i, v := range g.Data {
			d[i] = float32(v)
		}
		return r
	}
}

// TestGraphParity runs any Supertonic graph in strict mode and compares all its
// outputs to an onnxruntime reference. Set SUPERTONIC_GEN_REF to the ref JSON
// and SUPERTONIC_MODEL_DIR to the model dir.
func TestGraphParity(t *testing.T) {
	refPath := os.Getenv("SUPERTONIC_GEN_REF")
	modelDir := os.Getenv("SUPERTONIC_MODEL_DIR")
	if refPath == "" || modelDir == "" {
		t.Skip("set SUPERTONIC_GEN_REF and SUPERTONIC_MODEL_DIR")
	}
	raw, err := os.ReadFile(refPath)
	if err != nil {
		t.Fatalf("read ref: %v", err)
	}
	var ref genRef
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatalf("parse ref: %v", err)
	}

	opts := DefaultLoadOptions()
	opts.StrictMode = true
	model, err := Load(modelDir+"/onnx/"+ref.Model, cpu.New(), opts)
	if err != nil {
		t.Fatalf("strict Load %s failed: %v", ref.Model, err)
	}

	feeds := make(map[string]*tensor.RawTensor, len(ref.Inputs))
	for name, g := range ref.Inputs {
		feeds[name] = toRaw(t, g)
	}
	outputs, err := model.ForwardNamed(feeds)
	if err != nil {
		t.Fatalf("ForwardNamed: %v", err)
	}

	var maxAbs float64
	for name, want := range ref.Outputs {
		got, ok := outputs[name]
		if !ok {
			t.Fatalf("missing output %q", name)
		}
		gd := got.AsFloat32()
		if len(gd) != len(want.Data) {
			t.Fatalf("output %q len: got %d want %d", name, len(gd), len(want.Data))
		}
		for i := range want.Data {
			d := math.Abs(float64(gd[i]) - want.Data[i])
			if d > maxAbs {
				maxAbs = d
			}
		}
		if maxAbs > 2e-3 {
			t.Fatalf("output %q diverges: maxAbs=%g", name, maxAbs)
		}
	}
	t.Logf("%s parity OK: maxAbs=%g", ref.Model, maxAbs)
}
