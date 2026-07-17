package nn

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

func TestNewRotaryEncoding(t *testing.T) {
	backend := cpu.New()

	tests := []struct {
		name      string
		cfg       RotaryEncodingConfig
		wantPanic bool
	}{
		{
			name: "valid config",
			cfg: RotaryEncodingConfig{
				DModel:    64,
				MaxSeqLen: 128,
				Theta:     10000.0,
			},
			wantPanic: false,
		},
		{
			name: "odd dimension",
			cfg: RotaryEncodingConfig{
				DModel:    63,
				MaxSeqLen: 128,
				Theta:     10000.0,
			},
			wantPanic: true,
		},
		{
			name: "invalid max seq len",
			cfg: RotaryEncodingConfig{
				DModel:    64,
				MaxSeqLen: -1,
				Theta:     10000.0,
			},
			wantPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if (r != nil) != tt.wantPanic {
					t.Errorf("NewRotaryEncoding() panic = %v, wantPanic %v", r != nil, tt.wantPanic)
				}
			}()

			rope := NewRotaryEncoding(tt.cfg, backend)
			if !tt.wantPanic {
				if rope.DModel != tt.cfg.DModel {
					t.Errorf("DModel = %d, want %d", rope.DModel, tt.cfg.DModel)
				}
				if rope.MaxSeqLen != tt.cfg.MaxSeqLen {
					t.Errorf("MaxSeqLen = %d, want %d", rope.MaxSeqLen, tt.cfg.MaxSeqLen)
				}
			}
		})
	}
}

func TestRotaryEncodingForward3D(t *testing.T) {
	backend := cpu.New()
	cfg := RotaryEncodingConfig{
		DModel:    4, // Small for testing
		MaxSeqLen: 10,
		Theta:     10000.0,
	}
	rope := NewRotaryEncoding(cfg, backend)

	// Test 3D input: [batch, seq, d_model]
	batch := 2
	seq := 3
	x := tensor.Randn[float32](tensor.Shape{batch, seq, 4}, backend)

	out := rope.Forward(x)

	// Check shape
	if !equalShape(out.Shape(), tensor.Shape{batch, seq, 4}) {
		t.Errorf("Forward() shape = %v, want [%d, %d, 4]", out.Shape(), batch, seq)
	}

	// Check that output is different from input (rotation applied)
	xData := x.Data()
	outData := out.Data()
	allSame := true
	for i := range xData {
		if math.Abs(float64(xData[i]-outData[i])) > 1e-6 {
			allSame = false
			break
		}
	}
	if allSame {
		t.Error("Forward() output is identical to input, expected rotation to be applied")
	}
}

func TestRotaryEncodingForward4D(t *testing.T) {
	backend := cpu.New()
	cfg := RotaryEncodingConfig{
		DModel:    64,
		MaxSeqLen: 128,
		Theta:     10000.0,
	}
	rope := NewRotaryEncoding(cfg, backend)

	// Test 4D input: [batch, heads, seq, d_k]
	batch := 2
	heads := 8
	seq := 16
	dk := 64

	x := tensor.Randn[float32](tensor.Shape{batch, heads, seq, dk}, backend)
	out := rope.Forward(x)

	// Check shape
	expectedShape := tensor.Shape{batch, heads, seq, dk}
	if !equalShape(out.Shape(), expectedShape) {
		t.Errorf("Forward() shape = %v, want %v", out.Shape(), expectedShape)
	}
}

func TestRotaryEncodingForwardWithOffset(t *testing.T) {
	backend := cpu.New()
	cfg := RotaryEncodingConfig{
		DModel:    4,
		MaxSeqLen: 100,
		Theta:     10000.0,
	}
	rope := NewRotaryEncoding(cfg, backend)

	// Test with offset
	batch := 1
	seq := 5
	offset := 10

	x := tensor.Randn[float32](tensor.Shape{batch, seq, 4}, backend)
	out := rope.ForwardWithOffset(x, offset)

	// Check shape
	if !equalShape(out.Shape(), tensor.Shape{batch, seq, 4}) {
		t.Errorf("ForwardWithOffset() shape = %v, want [%d, %d, 4]", out.Shape(), batch, seq)
	}

	// Test that offset beyond max_seq_len panics
	defer func() {
		if r := recover(); r == nil {
			t.Error("ForwardWithOffset() with offset+seq > MaxSeqLen should panic")
		}
	}()
	rope.ForwardWithOffset(x, 96) // 96 + 5 > 100
}

func TestRotaryEncodingRotationProperties(t *testing.T) {
	backend := cpu.New()
	cfg := RotaryEncodingConfig{
		DModel:    4,
		MaxSeqLen: 10,
		Theta:     10000.0,
	}
	rope := NewRotaryEncoding(cfg, backend)

	// Create simple input with known values
	// [1, 1, 4] - single batch, single position, 4 dimensions
	xData := []float32{1.0, 0.0, 1.0, 0.0}
	x, err := tensor.FromSlice[float32](xData, tensor.Shape{1, 1, 4}, backend)
	if err != nil {
		t.Fatalf("FromSlice failed: %v", err)
	}

	out := rope.Forward(x)
	outData := out.Data()

	// At position 0, cos = [1, 1], sin = [0, 0]
	// Rotation should be identity at position 0
	// x_rotated[0] = x[0] * cos[0] - x[1] * sin[0] = 1.0 * 1.0 - 0.0 * 0.0 = 1.0
	// x_rotated[1] = x[0] * sin[0] + x[1] * cos[0] = 1.0 * 0.0 + 0.0 * 1.0 = 0.0

	// Check first pair (should be close to original at position 0)
	if math.Abs(float64(outData[0]-1.0)) > 0.01 {
		t.Errorf("First dimension at pos 0: got %f, want ~1.0", outData[0])
	}
	if math.Abs(float64(outData[1])) > 0.01 {
		t.Errorf("Second dimension at pos 0: got %f, want ~0.0", outData[1])
	}
}

func TestRotaryEncodingInvalidInput(t *testing.T) {
	backend := cpu.New()
	cfg := RotaryEncodingConfig{
		DModel:    64,
		MaxSeqLen: 128,
		Theta:     10000.0,
	}
	rope := NewRotaryEncoding(cfg, backend)

	tests := []struct {
		name      string
		shape     tensor.Shape
		wantPanic bool
	}{
		{
			name:      "2D input",
			shape:     tensor.Shape{2, 64},
			wantPanic: true,
		},
		{
			name:      "5D input",
			shape:     tensor.Shape{2, 8, 10, 64, 1},
			wantPanic: true,
		},
		{
			name:      "wrong dimension",
			shape:     tensor.Shape{2, 10, 32}, // d_model = 32, expected 64
			wantPanic: true,
		},
		{
			name:      "seq too long",
			shape:     tensor.Shape{2, 200, 64}, // seq = 200 > max_seq_len = 128
			wantPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if (r != nil) != tt.wantPanic {
					t.Errorf("Forward() panic = %v, wantPanic %v", r != nil, tt.wantPanic)
				}
			}()

			x := tensor.Randn[float32](tt.shape, backend)
			_ = rope.Forward(x)
		})
	}
}

func TestRotaryEncodingThetaFrequencies(t *testing.T) {
	backend := cpu.New()
	cfg := RotaryEncodingConfig{
		DModel:    4,
		MaxSeqLen: 1,
		Theta:     10000.0,
	}
	rope := NewRotaryEncoding(cfg, backend)

	// Check that frequencies are computed correctly
	// θ_0 = 10000^(-0/4) = 1.0
	// θ_1 = 10000^(-2/4) = 10000^(-0.5) = 0.01

	cosData := rope.FreqCos.Data() // [1, 2] - 1 position, 2 pairs
	sinData := rope.FreqSin.Data()

	// At position 0:
	// cos(0 * θ_0) = cos(0) = 1.0
	// sin(0 * θ_0) = sin(0) = 0.0
	// cos(0 * θ_1) = cos(0) = 1.0
	// sin(0 * θ_1) = sin(0) = 0.0

	epsilon := 1e-5
	if math.Abs(float64(cosData[0]-1.0)) > epsilon {
		t.Errorf("cos[0] = %f, want 1.0", cosData[0])
	}
	if math.Abs(float64(sinData[0])) > epsilon {
		t.Errorf("sin[0] = %f, want 0.0", sinData[0])
	}
}

// TestRotaryEncodingRotateHalfConvention verifies that RoPE uses the rotate-half
// convention (LLaMA/GPT-NeoX standard) rather than interleaved (GPT-J style).
//
// Rotate-half pairs (x[i], x[i+d/2]); interleaved pairs (x[2i], x[2i+1]).
// LLaMA models are trained with rotate-half — using interleaved produces garbage.
func TestRotaryEncodingRotateHalfConvention(t *testing.T) {
	backend := cpu.New()

	dModel := 4 // Minimal: 4 dims → halfDim=2, pairs: (x[0],x[2]) and (x[1],x[3]).
	cfg := RotaryEncodingConfig{
		DModel:    dModel,
		MaxSeqLen: 10,
		Theta:     10000.0,
	}
	rope := NewRotaryEncoding(cfg, backend)

	// Known input: x = [1, 2, 3, 4]. Shape [1, 1, 4] (batch=1, seq=1, dim=4).
	x, err := tensor.FromSlice[float32](
		[]float32{1, 2, 3, 4},
		tensor.Shape{1, 1, dModel},
		backend,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Position 0: cos=[1,1], sin=[0,0] → output = input (identity rotation).
	out0 := rope.ForwardWithOffset(x, 0)
	d0 := out0.Data()
	for i, want := range []float32{1, 2, 3, 4} {
		if math.Abs(float64(d0[i]-want)) > 1e-5 {
			t.Errorf("pos=0: out[%d]=%.6f, want %.6f", i, d0[i], want)
		}
	}

	// Position 1: non-trivial rotation. Verify rotate-half formula explicitly.
	//
	// cos/sin at position 1:
	//   freq[0] = 1/theta^(0/4) = 1.0
	//   freq[1] = 1/theta^(2/4) = 1/100 = 0.01
	//   cos = [cos(1*1.0), cos(1*0.01)] = [cos(1), cos(0.01)]
	//   sin = [sin(1*1.0), sin(1*0.01)] = [sin(1), sin(0.01)]
	//
	// Rotate-half (correct):
	//   out[0] = x[0]*cos[0] - x[2]*sin[0] = 1*cos(1) - 3*sin(1)
	//   out[1] = x[1]*cos[1] - x[3]*sin[1] = 2*cos(0.01) - 4*sin(0.01)
	//   out[2] = x[2]*cos[0] + x[0]*sin[0] = 3*cos(1) + 1*sin(1)
	//   out[3] = x[3]*cos[1] + x[1]*sin[1] = 4*cos(0.01) + 2*sin(0.01)
	//
	// Interleaved (WRONG for LLaMA):
	//   out[0] = x[0]*cos[0] - x[1]*sin[0]  ← pairs (x[0],x[1]) instead of (x[0],x[2])
	cos1 := float32(math.Cos(1.0))
	sin1 := float32(math.Sin(1.0))
	cos001 := float32(math.Cos(0.01))
	sin001 := float32(math.Sin(0.01))

	expected := []float32{
		1*cos1 - 3*sin1,     // out[0]: x[0]*cos[0] - x[halfDim+0]*sin[0]
		2*cos001 - 4*sin001, // out[1]: x[1]*cos[1] - x[halfDim+1]*sin[1]
		3*cos1 + 1*sin1,     // out[2]: x[halfDim+0]*cos[0] + x[0]*sin[0]
		4*cos001 + 2*sin001, // out[3]: x[halfDim+1]*cos[1] + x[1]*sin[1]
	}

	out1 := rope.ForwardWithOffset(x, 1)
	d1 := out1.Data()
	for i, want := range expected {
		if math.Abs(float64(d1[i]-want)) > 1e-4 {
			t.Errorf("pos=1 rotate-half: out[%d]=%.6f, want %.6f (delta=%.6f)",
				i, d1[i], want, d1[i]-want)
		}
	}

	// Verify it's NOT interleaved: if interleaved, out[0] = x[0]*cos[0] - x[1]*sin[0].
	interleavedOut0 := 1*cos1 - 2*sin1
	if math.Abs(float64(d1[0]-interleavedOut0)) < 1e-4 {
		t.Error("RoPE appears to use INTERLEAVED convention — LLaMA requires ROTATE-HALF")
	}
}

// TestRotaryEncodingRotateHalf4D verifies rotate-half on 4D attention tensors.
func TestRotaryEncodingRotateHalf4D(t *testing.T) {
	backend := cpu.New()

	dModel := 4
	rope := NewRotaryEncoding(RotaryEncodingConfig{
		DModel: dModel, MaxSeqLen: 10, Theta: 10000.0,
	}, backend)

	// [batch=1, heads=2, seq=1, dim=4] — two heads with different values.
	x, err := tensor.FromSlice[float32](
		[]float32{
			1, 2, 3, 4, // head 0
			5, 6, 7, 8, // head 1
		},
		tensor.Shape{1, 2, 1, dModel},
		backend,
	)
	if err != nil {
		t.Fatal(err)
	}

	out := rope.ForwardWithOffset(x, 1)
	d := out.Data()

	cos1 := float32(math.Cos(1.0))
	sin1 := float32(math.Sin(1.0))
	cos001 := float32(math.Cos(0.01))
	sin001 := float32(math.Sin(0.01))

	// Head 0: rotate-half on [1,2,3,4].
	expected0 := []float32{1*cos1 - 3*sin1, 2*cos001 - 4*sin001, 3*cos1 + 1*sin1, 4*cos001 + 2*sin001}
	// Head 1: rotate-half on [5,6,7,8].
	expected1 := []float32{5*cos1 - 7*sin1, 6*cos001 - 8*sin001, 7*cos1 + 5*sin1, 8*cos001 + 6*sin001}

	for i, want := range append(expected0, expected1...) {
		if math.Abs(float64(d[i]-want)) > 1e-4 {
			t.Errorf("4D out[%d]=%.6f, want %.6f", i, d[i], want)
		}
	}
}

func BenchmarkRotaryEncodingForward3D(b *testing.B) {
	backend := cpu.New()
	cfg := RotaryEncodingConfig{
		DModel:    64,
		MaxSeqLen: 2048,
		Theta:     10000.0,
	}
	rope := NewRotaryEncoding(cfg, backend)

	x := tensor.Randn[float32](tensor.Shape{32, 128, 64}, backend) // batch=32, seq=128, dim=64

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rope.Forward(x)
	}
}

func BenchmarkRotaryEncodingForward4D(b *testing.B) {
	backend := cpu.New()
	cfg := RotaryEncodingConfig{
		DModel:    64,
		MaxSeqLen: 2048,
		Theta:     10000.0,
	}
	rope := NewRotaryEncoding(cfg, backend)

	x := tensor.Randn[float32](tensor.Shape{16, 12, 128, 64}, backend) // batch=16, heads=12, seq=128, dim=64

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rope.Forward(x)
	}
}

func BenchmarkRotaryEncodingForwardWithOffset(b *testing.B) {
	backend := cpu.New()
	cfg := RotaryEncodingConfig{
		DModel:    64,
		MaxSeqLen: 2048,
		Theta:     10000.0,
	}
	rope := NewRotaryEncoding(cfg, backend)

	x := tensor.Randn[float32](tensor.Shape{16, 12, 1, 64}, backend) // KV-cache: single new token

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rope.ForwardWithOffset(x, 100)
	}
}
