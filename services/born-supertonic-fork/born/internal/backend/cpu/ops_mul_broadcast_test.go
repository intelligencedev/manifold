package cpu

import (
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// TestMulBroadcast_Scale checks the trailing-run scale fast path in
// mulBroadcastFloat32 against the naive mock backend across the broadcast shapes
// the model actually uses (squeeze-and-excite scales), in both operand orders.
func TestMulBroadcast_Scale(t *testing.T) {
	cpuBackend := New()
	mockBackend := tensor.NewMockBackend()

	cases := []struct {
		name       string
		full, bcst tensor.Shape
	}{
		{"per-channel NC11", tensor.Shape{2, 4, 3, 5}, tensor.Shape{2, 4, 1, 1}},
		{"per-channel 1C11", tensor.Shape{2, 4, 3, 5}, tensor.Shape{1, 4, 1, 1}},
		{"scalar 1111", tensor.Shape{2, 4, 3, 5}, tensor.Shape{1, 1, 1, 1}},
		{"trailing W only", tensor.Shape{2, 4, 3, 5}, tensor.Shape{2, 4, 3, 1}},
		{"middle-dim broadcast", tensor.Shape{2, 4, 3, 5}, tensor.Shape{2, 1, 3, 5}}, // odometer fallback
		{"middle-dim broadcast 3d", tensor.Shape{2, 3, 5}, tensor.Shape{2, 1, 5}},    // odometer fallback
		{"3d per-channel", tensor.Shape{4, 6, 6}, tensor.Shape{4, 1, 1}},
		// Leading-tile (per-feature) broadcasts: the dense vector tiles over leading
		// dims (the model's STFT front-end pattern [M,L]*[L]).
		{"leading-tile 2d", tensor.Shape{5, 8}, tensor.Shape{8}},
		{"leading-tile 3d", tensor.Shape{2, 5, 8}, tensor.Shape{8}},
		{"leading-tile last-dim2", tensor.Shape{2, 5, 4, 2}, tensor.Shape{1, 1, 1, 2}},
		{"leading-tile 3d explicit", tensor.Shape{2, 5, 8}, tensor.Shape{1, 1, 8}},
	}

	mkData := func(shape tensor.Shape, scale float32) *tensor.RawTensor {
		rt, _ := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
		d := rt.AsFloat32()
		for i := range d {
			d[i] = float32((i%11)-5) * scale
		}
		return rt
	}

	for _, c := range cases {
		for _, swap := range []bool{false, true} {
			a, b := mkData(c.full, 0.5), mkData(c.bcst, 0.3)
			if swap {
				a, b = b, a // exercise the commuted (bcast * full) order
			}
			got := cpuBackend.Mul(a, b).AsFloat32()
			want := mockBackend.Mul(a, b).AsFloat32()
			if len(got) != len(want) {
				t.Fatalf("%s swap=%v: len %d != %d", c.name, swap, len(got), len(want))
			}
			for i := range got {
				// A single float32 multiply per element matches the float64 oracle to
				// within float32 rounding; 1e-6 is tight for values in [-2.5, 2.5].
				if d := got[i] - want[i]; d < -1e-6 || d > 1e-6 {
					t.Errorf("%s swap=%v idx %d: got %.7f want %.7f", c.name, swap, i, got[i], want[i])
					break
				}
			}
		}
	}
}

func benchMulBroadcast(b *testing.B, full, bcst tensor.Shape) {
	backend := New()
	a, _ := tensor.NewRaw(full, tensor.Float32, tensor.CPU)
	for i, d := 0, a.AsFloat32(); i < len(d); i++ {
		d[i] = float32((i%11)-5) * 0.5
	}
	s, _ := tensor.NewRaw(bcst, tensor.Float32, tensor.CPU)
	for i, d := 0, s.AsFloat32(); i < len(d); i++ {
		d[i] = float32((i%7)-3) * 0.3
	}

	b.ResetTimer()
	for b.Loop() {
		backend.Mul(a, s)
	}
}

// Per-channel scale [N,C,H,W]*[N,C,1,1] (squeeze-and-excite), the trailing-run
// fast path, at a feature-map size typical of a conv block.
func BenchmarkMulBroadcast_PerChannelScale(b *testing.B) {
	benchMulBroadcast(b, tensor.Shape{1, 256, 16, 16}, tensor.Shape{1, 256, 1, 1})
}

// Per-feature scale [M,L]*[L] (STFT front end), the leading-tile fast path.
func BenchmarkMulBroadcast_LeadingTile(b *testing.B) {
	benchMulBroadcast(b, tensor.Shape{1024, 256}, tensor.Shape{256})
}
