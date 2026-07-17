package nn

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/autodiff"
	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type Backend = *autodiff.AutodiffBackend[*cpu.CPUBackend]

// sigmoid computes sigmoid for testing.
func sigmoid(x float32) float32 {
	return 1.0 / (1.0 + float32(math.Exp(float64(-x))))
}

// silu computes SiLU for testing.
func silu(x float32) float32 {
	return x * sigmoid(x)
}

// gelu computes GELU (tanh approximation) for testing.
func gelu(x float32) float32 {
	sqrt2pi := float32(math.Sqrt(2.0 / math.Pi))
	c := float32(0.044715)
	x3 := x * x * x
	inner := sqrt2pi * (x + c*x3)
	tanhVal := float32(math.Tanh(float64(inner)))
	return 0.5 * x * (1.0 + tanhVal)
}

// TestSwiGLU_Output tests SwiGLU(x, gate) = x * SiLU(gate).
func TestSwiGLU_Output(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Test data
	xData := []float32{1.0, 2.0, 3.0, 4.0}
	gateData := []float32{-1.0, 0.0, 1.0, 2.0}

	x, err := tensor.FromSlice[float32](xData, tensor.Shape{4}, backend)
	require.NoError(t, err)

	gate, err := tensor.FromSlice[float32](gateData, tensor.Shape{4}, backend)
	require.NoError(t, err)

	// Forward pass
	output := SwiGLU(x, gate)

	// Expected: x * SiLU(gate)
	expected := make([]float32, 4)
	for i := range xData {
		expected[i] = xData[i] * silu(gateData[i])
	}

	outputData := output.Data()
	for i, exp := range expected {
		assert.InDelta(t, exp, outputData[i], 0.001, "SwiGLU mismatch at index %d", i)
	}
}

// TestGeGLU_Output tests GeGLU(x, gate) = x * GELU(gate).
func TestGeGLU_Output(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Test data
	xData := []float32{1.0, 2.0, 3.0, 4.0}
	gateData := []float32{-1.0, 0.0, 1.0, 2.0}

	x, err := tensor.FromSlice[float32](xData, tensor.Shape{4}, backend)
	require.NoError(t, err)

	gate, err := tensor.FromSlice[float32](gateData, tensor.Shape{4}, backend)
	require.NoError(t, err)

	// Forward pass
	output := GeGLU(x, gate)

	// Expected: x * GELU(gate)
	expected := make([]float32, 4)
	for i := range xData {
		expected[i] = xData[i] * gelu(gateData[i])
	}

	outputData := output.Data()
	for i, exp := range expected {
		assert.InDelta(t, exp, outputData[i], 0.001, "GeGLU mismatch at index %d", i)
	}
}

// TestReGLU_Output tests ReGLU(x, gate) = x * ReLU(gate).
func TestReGLU_Output(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Test data
	xData := []float32{1.0, 2.0, 3.0, 4.0}
	gateData := []float32{-1.0, 0.0, 1.0, 2.0}

	x, err := tensor.FromSlice[float32](xData, tensor.Shape{4}, backend)
	require.NoError(t, err)

	gate, err := tensor.FromSlice[float32](gateData, tensor.Shape{4}, backend)
	require.NoError(t, err)

	// Forward pass
	output := ReGLU(x, gate)

	// Expected: x * ReLU(gate)
	// For gate=[-1, 0, 1, 2], ReLU=[0, 0, 1, 2]
	// Result: x * ReLU(gate) = [0, 0, 3, 8]
	expected := []float32{0.0, 0.0, 3.0, 8.0}

	outputData := output.Data()
	for i, exp := range expected {
		assert.InDelta(t, exp, outputData[i], 0.001, "ReGLU mismatch at index %d", i)
	}
}

// TestGLU_Output tests GLU(x, gate) = x * sigmoid(gate).
func TestGLU_Output(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Test data
	xData := []float32{1.0, 2.0, 3.0, 4.0}
	gateData := []float32{-1.0, 0.0, 1.0, 2.0}

	x, err := tensor.FromSlice[float32](xData, tensor.Shape{4}, backend)
	require.NoError(t, err)

	gate, err := tensor.FromSlice[float32](gateData, tensor.Shape{4}, backend)
	require.NoError(t, err)

	// Forward pass
	output := GLU(x, gate)

	// Expected: x * sigmoid(gate)
	expected := make([]float32, 4)
	for i := range xData {
		expected[i] = xData[i] * sigmoid(gateData[i])
	}

	outputData := output.Data()
	for i, exp := range expected {
		assert.InDelta(t, exp, outputData[i], 0.001, "GLU mismatch at index %d", i)
	}
}

// TestGELUFunc tests GELU approximation.
func TestGELUFunc(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Test data: [-2, -1, 0, 1, 2]
	inputData := []float32{-2.0, -1.0, 0.0, 1.0, 2.0}
	input, err := tensor.FromSlice[float32](inputData, tensor.Shape{5}, backend)
	require.NoError(t, err)

	// Forward pass
	output := GELUFunc(input)

	// Expected values using tanh approximation
	expected := make([]float32, 5)
	for i, x := range inputData {
		expected[i] = gelu(x)
	}

	outputData := output.Data()
	for i, exp := range expected {
		assert.InDelta(t, exp, outputData[i], 0.001, "GELU mismatch at index %d", i)
	}
}

// TestSwiGLUFFN_Shapes tests that SwiGLUFFN produces correct output shapes.
func TestSwiGLUFFN_Shapes(t *testing.T) {
	backend := autodiff.New(cpu.New())

	tests := []struct {
		name      string
		embedDim  int
		ffnDim    int
		inputSize []int
		wantSize  []int
	}{
		{
			name:      "2D input",
			embedDim:  64,
			ffnDim:    256,
			inputSize: []int{8, 64}, // [batch, embed]
			wantSize:  []int{8, 64},
		},
		{
			name:      "3D input",
			embedDim:  128,
			ffnDim:    512,
			inputSize: []int{4, 10, 128}, // [batch, seq, embed]
			wantSize:  []int{4, 10, 128},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SwiGLUFFNConfig{
				EmbedDim: tt.embedDim,
				FFNDim:   tt.ffnDim,
			}
			ffn := NewSwiGLUFFN(cfg, backend)

			input := tensor.Randn[float32](tensor.Shape(tt.inputSize), backend)
			output := ffn.Forward(input)

			assert.Equal(t, tt.wantSize, []int(output.Shape()), "Output shape mismatch")
		})
	}
}

// TestSwiGLUFFN_WithDifferentVariants tests all GLU variants.
func TestSwiGLUFFN_WithDifferentVariants(t *testing.T) {
	backend := autodiff.New(cpu.New())

	variants := []string{"swiglu", "geglu", "reglu", "glu"}

	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			cfg := SwiGLUFFNConfig{
				EmbedDim:   32,
				FFNDim:     128,
				GLUVariant: variant,
			}
			ffn := NewSwiGLUFFN(cfg, backend)

			input := tensor.Randn[float32](tensor.Shape{4, 32}, backend)
			output := ffn.Forward(input)

			assert.Equal(t, []int{4, 32}, []int(output.Shape()), "Output shape mismatch")
			assert.NotNil(t, output, "Output should not be nil")
		})
	}
}

// TestSwiGLUFFN_Parameters tests parameter count.
func TestSwiGLUFFN_Parameters(t *testing.T) {
	backend := autodiff.New(cpu.New())

	tests := []struct {
		name      string
		embedDim  int
		ffnDim    int
		useBias   bool
		wantCount int
	}{
		{
			name:      "no bias",
			embedDim:  64,
			ffnDim:    256,
			useBias:   false,
			wantCount: 3, // gate.weight, up.weight, down.weight
		},
		{
			name:      "with bias",
			embedDim:  64,
			ffnDim:    256,
			useBias:   true,
			wantCount: 6, // gate.weight+bias, up.weight+bias, down.weight+bias
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SwiGLUFFNConfig{
				EmbedDim: tt.embedDim,
				FFNDim:   tt.ffnDim,
				UseBias:  tt.useBias,
			}
			ffn := NewSwiGLUFFN(cfg, backend)

			params := ffn.Parameters()
			assert.Equal(t, tt.wantCount, len(params), "Parameter count mismatch")
		})
	}
}

// TestSwiGLUFFN_DefaultFFNDim tests automatic FFN dimension calculation.
func TestSwiGLUFFN_DefaultFFNDim(t *testing.T) {
	backend := autodiff.New(cpu.New())

	cfg := SwiGLUFFNConfig{
		EmbedDim: 4096,
		FFNDim:   0, // Auto-calculate
	}
	ffn := NewSwiGLUFFN(cfg, backend)

	// LLaMA formula: 8/3 * d_model, rounded to multiple of 256
	// 8/3 * 4096 = 10922.67 → round to 11008
	expectedFFNDim := 11008

	// Check via layer shapes
	gateProj := ffn.GateProj()
	assert.Equal(t, 4096, gateProj.InFeatures(), "InFeatures mismatch")
	assert.Equal(t, expectedFFNDim, gateProj.OutFeatures(), "FFNDim not calculated correctly")
}

// TestNewLinearNoBias tests Linear layer without bias.
func TestNewLinearNoBias(t *testing.T) {
	backend := autodiff.New(cpu.New())

	linear := NewLinear[Backend](128, 256, backend, WithBias(false))

	// Check parameters
	params := linear.Parameters()
	require.Len(t, params, 1, "Should have only weight parameter")
	assert.Equal(t, "weight", params[0].Name())

	// Check bias is nil
	assert.Nil(t, linear.Bias(), "Bias should be nil")

	// Test forward pass
	input := tensor.Randn[float32](tensor.Shape{4, 128}, backend)
	output := linear.Forward(input)

	assert.Equal(t, []int{4, 256}, []int(output.Shape()), "Output shape mismatch")
}

// TestSwiGLUFFN_InvalidConfig tests error handling for invalid configs.
func TestSwiGLUFFN_InvalidConfig(t *testing.T) {
	backend := autodiff.New(cpu.New())

	tests := []struct {
		name   string
		config SwiGLUFFNConfig
		panics bool
	}{
		{
			name: "zero EmbedDim",
			config: SwiGLUFFNConfig{
				EmbedDim: 0,
				FFNDim:   256,
			},
			panics: true,
		},
		{
			name: "negative EmbedDim",
			config: SwiGLUFFNConfig{
				EmbedDim: -10,
				FFNDim:   256,
			},
			panics: true,
		},
		{
			name: "invalid GLUVariant",
			config: SwiGLUFFNConfig{
				EmbedDim:   64,
				FFNDim:     256,
				GLUVariant: "invalid",
			},
			panics: true,
		},
		{
			name: "valid config",
			config: SwiGLUFFNConfig{
				EmbedDim: 64,
				FFNDim:   256,
			},
			panics: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.panics {
				assert.Panics(t, func() {
					NewSwiGLUFFN(tt.config, backend)
				}, "Expected panic for invalid config")
			} else {
				assert.NotPanics(t, func() {
					NewSwiGLUFFN(tt.config, backend)
				}, "Should not panic for valid config")
			}
		})
	}
}

// BenchmarkSwiGLU benchmarks SwiGLU function.
func BenchmarkSwiGLU(b *testing.B) {
	backend := autodiff.New(cpu.New())
	x := tensor.Randn[float32](tensor.Shape{1024, 2048}, backend)
	gate := tensor.Randn[float32](tensor.Shape{1024, 2048}, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = SwiGLU(x, gate)
	}
}

// BenchmarkGeGLU benchmarks GeGLU function.
func BenchmarkGeGLU(b *testing.B) {
	backend := autodiff.New(cpu.New())
	x := tensor.Randn[float32](tensor.Shape{1024, 2048}, backend)
	gate := tensor.Randn[float32](tensor.Shape{1024, 2048}, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GeGLU(x, gate)
	}
}

// BenchmarkSwiGLUFFN_Forward benchmarks SwiGLUFFN forward pass.
func BenchmarkSwiGLUFFN_Forward(b *testing.B) {
	backend := autodiff.New(cpu.New())

	cfg := SwiGLUFFNConfig{
		EmbedDim: 4096,
		FFNDim:   11008,
	}
	ffn := NewSwiGLUFFN(cfg, backend)

	input := tensor.Randn[float32](tensor.Shape{8, 512, 4096}, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ffn.Forward(input)
	}
}

// BenchmarkGELUFunc benchmarks GELU approximation.
func BenchmarkGELUFunc(b *testing.B) {
	backend := autodiff.New(cpu.New())
	input := tensor.Randn[float32](tensor.Shape{1024, 2048}, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GELUFunc(input)
	}
}
