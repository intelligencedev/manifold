package cpu

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// TestConv2D_BasicForward tests basic Conv2D forward pass.
func TestConv2D_BasicForward(t *testing.T) {
	backend := New()

	// Input: [1, 1, 3, 3] - single channel 3x3 image
	input, _ := tensor.NewRaw(tensor.Shape{1, 1, 3, 3}, tensor.Float32, tensor.CPU)
	inputData := input.AsFloat32()
	// Simple pattern:
	// 1 2 3
	// 4 5 6
	// 7 8 9
	for i := 0; i < 9; i++ {
		inputData[i] = float32(i + 1)
	}

	// Kernel: [1, 1, 2, 2] - single 2x2 kernel
	kernel, _ := tensor.NewRaw(tensor.Shape{1, 1, 2, 2}, tensor.Float32, tensor.CPU)
	kernelData := kernel.AsFloat32()
	// Identity-like kernel:
	// 1 0
	// 0 1
	kernelData[0] = 1.0 // top-left
	kernelData[1] = 0.0 // top-right
	kernelData[2] = 0.0 // bottom-left
	kernelData[3] = 1.0 // bottom-right

	// Stride=1, Padding=0
	output := backend.Conv2D(input, kernel, 1, 0)

	// Output shape should be [1, 1, 2, 2]
	// out_h = (3 + 2*0 - 2) / 1 + 1 = 2
	// out_w = (3 + 2*0 - 2) / 1 + 1 = 2
	expectedShape := tensor.Shape{1, 1, 2, 2}
	if !output.Shape().Equal(expectedShape) {
		t.Fatalf("Expected shape %v, got %v", expectedShape, output.Shape())
	}

	outputData := output.AsFloat32()

	// Expected output (diagonal sum):
	// [1,0] patch: [1,2,4,5] -> 1*1 + 5*1 = 6
	// [0,1] patch: [2,3,5,6] -> 2*1 + 6*1 = 8
	// [1,0] patch: [4,5,7,8] -> 4*1 + 8*1 = 12
	// [1,1] patch: [5,6,8,9] -> 5*1 + 9*1 = 14
	expected := []float32{6, 8, 12, 14}

	for i, exp := range expected {
		if outputData[i] != exp {
			t.Errorf("Output[%d]: expected %.1f, got %.1f", i, exp, outputData[i])
		}
	}
}

// TestConv2D_WithPadding tests Conv2D with zero padding.
func TestConv2D_WithPadding(t *testing.T) {
	backend := New()

	// Input: [1, 1, 3, 3]
	input, _ := tensor.NewRaw(tensor.Shape{1, 1, 3, 3}, tensor.Float32, tensor.CPU)
	inputData := input.AsFloat32()
	for i := 0; i < 9; i++ {
		inputData[i] = 1.0 // All ones
	}

	// Kernel: [1, 1, 3, 3] - full 3x3 kernel
	kernel, _ := tensor.NewRaw(tensor.Shape{1, 1, 3, 3}, tensor.Float32, tensor.CPU)
	kernelData := kernel.AsFloat32()
	for i := 0; i < 9; i++ {
		kernelData[i] = 1.0 // All ones (sum kernel)
	}

	// Stride=1, Padding=1
	output := backend.Conv2D(input, kernel, 1, 1)

	// With padding=1, output shape = [1, 1, 3, 3]
	// out_h = (3 + 2*1 - 3) / 1 + 1 = 3
	expectedShape := tensor.Shape{1, 1, 3, 3}
	if !output.Shape().Equal(expectedShape) {
		t.Fatalf("Expected shape %v, got %v", expectedShape, output.Shape())
	}

	outputData := output.AsFloat32()

	// All input is 1, all kernel is 1, so output is sum of valid elements in 3x3 window
	// Corner: 4 valid elements -> 4
	// Edge: 6 valid elements -> 6
	// Center: 9 valid elements -> 9
	expected := []float32{
		4, 6, 4, // top row
		6, 9, 6, // middle row
		4, 6, 4, // bottom row
	}

	for i, exp := range expected {
		if outputData[i] != exp {
			t.Errorf("Output[%d]: expected %.1f, got %.1f", i, exp, outputData[i])
		}
	}
}

// TestConv2D_WithStride tests Conv2D with stride > 1.
func TestConv2D_WithStride(t *testing.T) {
	backend := New()

	// Input: [1, 1, 4, 4]
	input, _ := tensor.NewRaw(tensor.Shape{1, 1, 4, 4}, tensor.Float32, tensor.CPU)
	inputData := input.AsFloat32()
	for i := 0; i < 16; i++ {
		inputData[i] = float32(i + 1)
	}

	// Kernel: [1, 1, 2, 2]
	kernel, _ := tensor.NewRaw(tensor.Shape{1, 1, 2, 2}, tensor.Float32, tensor.CPU)
	kernelData := kernel.AsFloat32()
	for i := 0; i < 4; i++ {
		kernelData[i] = 1.0 // Sum kernel
	}

	// Stride=2, Padding=0
	output := backend.Conv2D(input, kernel, 2, 0)

	// Output shape: [1, 1, 2, 2]
	// out_h = (4 + 2*0 - 2) / 2 + 1 = 2
	expectedShape := tensor.Shape{1, 1, 2, 2}
	if !output.Shape().Equal(expectedShape) {
		t.Fatalf("Expected shape %v, got %v", expectedShape, output.Shape())
	}

	outputData := output.AsFloat32()

	// With stride=2, we skip positions
	// [0,0] patch: [1,2,5,6] -> sum=14
	// [0,2] patch: [3,4,7,8] -> sum=22
	// [2,0] patch: [9,10,13,14] -> sum=46
	// [2,2] patch: [11,12,15,16] -> sum=54
	expected := []float32{14, 22, 46, 54}

	for i, exp := range expected {
		if outputData[i] != exp {
			t.Errorf("Output[%d]: expected %.1f, got %.1f", i, exp, outputData[i])
		}
	}
}

// TestConv2D_MultiChannel tests Conv2D with multiple input/output channels.
func TestConv2D_MultiChannel(t *testing.T) {
	backend := New()

	// Input: [1, 2, 3, 3] - 2 channels
	input, _ := tensor.NewRaw(tensor.Shape{1, 2, 3, 3}, tensor.Float32, tensor.CPU)
	inputData := input.AsFloat32()
	// Channel 0: all 1s
	// Channel 1: all 2s
	for i := 0; i < 9; i++ {
		inputData[i] = 1.0   // channel 0
		inputData[9+i] = 2.0 // channel 1
	}

	// Kernel: [2, 2, 2, 2] - 2 output channels, 2 input channels
	kernel, _ := tensor.NewRaw(tensor.Shape{2, 2, 2, 2}, tensor.Float32, tensor.CPU)
	kernelData := kernel.AsFloat32()
	// Output channel 0: all 1s (sums both input channels)
	// Output channel 1: all 0.5s
	for i := 0; i < 8; i++ {
		kernelData[i] = 1.0   // out channel 0
		kernelData[8+i] = 0.5 // out channel 1
	}

	// Stride=1, Padding=0
	output := backend.Conv2D(input, kernel, 1, 0)

	// Output shape: [1, 2, 2, 2]
	expectedShape := tensor.Shape{1, 2, 2, 2}
	if !output.Shape().Equal(expectedShape) {
		t.Fatalf("Expected shape %v, got %v", expectedShape, output.Shape())
	}

	outputData := output.AsFloat32()

	// Output channel 0: sum all 1s and 2s in 2x2 patches
	// Each patch: 4 values from ch0 (all 1) + 4 values from ch1 (all 2) = 4*1 + 4*2 = 12
	// All outputs for channel 0 should be 12

	// Output channel 1: multiply by 0.5
	// Each patch: 0.5 * (4*1 + 4*2) = 6

	// Channel 0 outputs
	for i := 0; i < 4; i++ {
		if outputData[i] != 12.0 {
			t.Errorf("Output channel 0 [%d]: expected 12.0, got %.1f", i, outputData[i])
		}
	}

	// Channel 1 outputs
	for i := 4; i < 8; i++ {
		if outputData[i] != 6.0 {
			t.Errorf("Output channel 1 [%d]: expected 6.0, got %.1f", i, outputData[i])
		}
	}
}

// TestConv2D_Batch tests Conv2D with batch size > 1.
func TestConv2D_Batch(t *testing.T) {
	backend := New()

	// Input: [2, 1, 2, 2] - batch of 2
	input, _ := tensor.NewRaw(tensor.Shape{2, 1, 2, 2}, tensor.Float32, tensor.CPU)
	inputData := input.AsFloat32()
	// Batch 0: [1,2,3,4]
	// Batch 1: [5,6,7,8]
	for i := 0; i < 4; i++ {
		inputData[i] = float32(i + 1)
		inputData[4+i] = float32(i + 5)
	}

	// Kernel: [1, 1, 2, 2] - sum kernel
	kernel, _ := tensor.NewRaw(tensor.Shape{1, 1, 2, 2}, tensor.Float32, tensor.CPU)
	kernelData := kernel.AsFloat32()
	for i := 0; i < 4; i++ {
		kernelData[i] = 1.0
	}

	// Stride=1, Padding=0
	output := backend.Conv2D(input, kernel, 1, 0)

	// Output shape: [2, 1, 1, 1] (single output per batch)
	expectedShape := tensor.Shape{2, 1, 1, 1}
	if !output.Shape().Equal(expectedShape) {
		t.Fatalf("Expected shape %v, got %v", expectedShape, output.Shape())
	}

	outputData := output.AsFloat32()

	// Batch 0: 1+2+3+4 = 10
	// Batch 1: 5+6+7+8 = 26
	if outputData[0] != 10.0 {
		t.Errorf("Batch 0: expected 10.0, got %.1f", outputData[0])
	}
	if outputData[1] != 26.0 {
		t.Errorf("Batch 1: expected 26.0, got %.1f", outputData[1])
	}
}

// TestConv2D_MatchesMockBackend verifies CPU implementation matches naive MockBackend.
func TestConv2D_MatchesMockBackend(t *testing.T) {
	cpuBackend := New()
	mockBackend := tensor.NewMockBackend()

	// Input: [1, 2, 4, 4]
	input, _ := tensor.NewRaw(tensor.Shape{1, 2, 4, 4}, tensor.Float32, tensor.CPU)
	inputData := input.AsFloat32()
	for i := range inputData {
		inputData[i] = float32(i % 7) // Some pattern
	}

	// Kernel: [3, 2, 3, 3]
	kernel, _ := tensor.NewRaw(tensor.Shape{3, 2, 3, 3}, tensor.Float32, tensor.CPU)
	kernelData := kernel.AsFloat32()
	for i := range kernelData {
		kernelData[i] = float32((i % 5) - 2) // Range [-2, 2]
	}

	// Test with different configurations
	configs := [][2]int{
		{1, 0}, // stride=1, padding=0
		{1, 1}, // stride=1, padding=1
		{2, 0}, // stride=2, padding=0
	}

	for _, cfg := range configs {
		stride, padding := cfg[0], cfg[1]

		cpuOutput := cpuBackend.Conv2D(input, kernel, stride, padding)
		mockOutput := mockBackend.Conv2D(input, kernel, stride, padding)

		if !cpuOutput.Shape().Equal(mockOutput.Shape()) {
			t.Fatalf("Shape mismatch (stride=%d, padding=%d): CPU=%v, Mock=%v",
				stride, padding, cpuOutput.Shape(), mockOutput.Shape())
		}

		cpuData := cpuOutput.AsFloat32()
		mockData := mockOutput.AsFloat32()

		for i := range cpuData {
			diff := cpuData[i] - mockData[i]
			if diff < -0.001 || diff > 0.001 {
				t.Errorf("Value mismatch at index %d (stride=%d, padding=%d): CPU=%.4f, Mock=%.4f",
					i, stride, padding, cpuData[i], mockData[i])
			}
		}
	}
}

// fillPointwiseConv writes deterministic values f(i) into a CPU float32/float64
// tensor, so the same case table drives both dtypes.
func fillPointwiseConv(t *tensor.RawTensor, f func(i int) float64) {
	switch t.DType() {
	case tensor.Float32:
		d := t.AsFloat32()
		for i := range d {
			d[i] = float32(f(i))
		}
	case tensor.Float64:
		d := t.AsFloat64()
		for i := range d {
			d[i] = f(i)
		}
	}
}

// maxPointwiseConvDiff returns the largest absolute element difference between
// two same-dtype tensors and the index where it occurred.
func maxPointwiseConvDiff(a, b *tensor.RawTensor) (maxD float64, idx int) {
	idx = -1
	switch a.DType() {
	case tensor.Float32:
		ad, bd := a.AsFloat32(), b.AsFloat32()
		for i := range ad {
			if d := math.Abs(float64(ad[i]) - float64(bd[i])); d > maxD {
				maxD, idx = d, i
			}
		}
	case tensor.Float64:
		ad, bd := a.AsFloat64(), b.AsFloat64()
		for i := range ad {
			if d := math.Abs(ad[i] - bd[i]); d > maxD {
				maxD, idx = d, i
			}
		}
	}
	return maxD, idx
}

// TestConv2D_Pointwise1x1 checks the 1x1 (pointwise) fast path against the naive
// mock backend across channel counts, spatial sizes, and batch sizes, for both
// float32 and float64. A 1x1 conv with stride=1, padding=0 reduces to a per-batch
// GEMM (kernel[COut,CIn] @ input[CIn,H*W]), so the fast path must match the im2col
// result. The float64 path (pointwiseConvFloat64) was previously uncovered.
func TestConv2D_Pointwise1x1(t *testing.T) {
	cpuBackend := New()
	mockBackend := tensor.NewMockBackend()

	shapes := []struct {
		n, cIn, h, w, cOut int
	}{
		{1, 3, 4, 4, 5},     // small
		{1, 64, 8, 8, 32},   // channel reduction
		{1, 16, 5, 7, 48},   // non-square spatial, expansion
		{2, 8, 3, 3, 4},     // batched
		{1, 1, 1, 1, 1},     // degenerate
		{3, 32, 6, 6, 16},   // larger batch
		{1, 96, 12, 12, 96}, // model-ish pointwise
	}

	for _, dt := range []tensor.DataType{tensor.Float32, tensor.Float64} {
		t.Run(dt.String(), func(t *testing.T) {
			// float32 GEMM-vs-naive reordering stays comfortably under 1e-5 for
			// these value ranges; float64 is effectively exact.
			tol := 1e-5
			if dt == tensor.Float64 {
				tol = 1e-9
			}
			for _, s := range shapes {
				input, _ := tensor.NewRaw(tensor.Shape{s.n, s.cIn, s.h, s.w}, dt, tensor.CPU)
				kernel, _ := tensor.NewRaw(tensor.Shape{s.cOut, s.cIn, 1, 1}, dt, tensor.CPU)
				fillPointwiseConv(input, func(i int) float64 { return float64((i%13)-6) * 0.25 })
				fillPointwiseConv(kernel, func(i int) float64 { return float64((i%7)-3) * 0.5 })

				got := cpuBackend.Conv2D(input, kernel, 1, 0)
				want := mockBackend.Conv2D(input, kernel, 1, 0)

				if !got.Shape().Equal(want.Shape()) {
					t.Fatalf("shape %+v: CPU=%v Mock=%v", s, got.Shape(), want.Shape())
				}
				if d, idx := maxPointwiseConvDiff(got, want); d > tol {
					t.Errorf("shape %+v idx %d: max diff %.3g exceeds tol %.3g", s, idx, d, tol)
				}
			}
		})
	}
}

func BenchmarkConv2D(b *testing.B) {
	backend := New()

	input := tensor.Randn[float32](tensor.Shape{1, 1, 28, 28}, backend).Raw()
	kernel := tensor.Randn[float32](tensor.Shape{6, 1, 5, 5}, backend).Raw()

	b.ResetTimer()
	for b.Loop() {
		backend.Conv2D(input, kernel, 1, 0)
	}
}

// benchPointwise1x1 exercises the 1x1 fast path (direct per-batch GEMM, no
// im2col/rearrange) at a channel/spatial size typical of pointwise layers.
func benchPointwise1x1(b *testing.B, n, cIn, h, w, cOut int) {
	backend := New()
	input := tensor.Randn[float32](tensor.Shape{n, cIn, h, w}, backend).Raw()
	kernel := tensor.Randn[float32](tensor.Shape{cOut, cIn, 1, 1}, backend).Raw()

	b.ResetTimer()
	for b.Loop() {
		backend.Conv2D(input, kernel, 1, 0)
	}
}

// Pointwise (1x1) layers as they appear in MobileNet/EfficientNet-style nets:
// channel expansion and reduction over a feature map.
func BenchmarkConv2D_Pointwise1x1_96to96(b *testing.B)   { benchPointwise1x1(b, 1, 96, 12, 12, 96) }
func BenchmarkConv2D_Pointwise1x1_256to512(b *testing.B) { benchPointwise1x1(b, 1, 256, 14, 14, 512) }

func BenchmarkConv2D_Batch(b *testing.B) {
	backend := New()

	input := tensor.Randn[float32](tensor.Shape{64, 1, 28, 28}, backend).Raw()
	kernel := tensor.Randn[float32](tensor.Shape{32, 1, 3, 3}, backend).Raw()

	b.ResetTimer()
	for b.Loop() {
		backend.Conv2D(input, kernel, 1, 1)
	}
}

func BenchmarkConv2D_MultiChannel(b *testing.B) {
	backend := New()

	input := tensor.Randn[float32](tensor.Shape{1, 16, 14, 14}, backend).Raw()
	kernel := tensor.Randn[float32](tensor.Shape{32, 16, 3, 3}, backend).Raw()

	b.ResetTimer()
	for b.Loop() {
		backend.Conv2D(input, kernel, 1, 1)
	}
}

func BenchmarkConv2D_Stride2(b *testing.B) {
	backend := New()

	input := tensor.Randn[float32](tensor.Shape{1, 8, 32, 32}, backend).Raw()
	kernel := tensor.Randn[float32](tensor.Shape{16, 8, 3, 3}, backend).Raw()

	b.ResetTimer()
	for b.Loop() {
		backend.Conv2D(input, kernel, 2, 0)
	}
}

func BenchmarkConv2D_Deep(b *testing.B) {
	backend := New()

	input := tensor.Randn[float32](tensor.Shape{8, 64, 14, 14}, backend).Raw()
	kernel := tensor.Randn[float32](tensor.Shape{128, 64, 3, 3}, backend).Raw()

	b.ResetTimer()
	for b.Loop() {
		backend.Conv2D(input, kernel, 1, 1)
	}
}

func BenchmarkConv2DInputBackward_Batch(b *testing.B) {
	backend := New()

	input := tensor.Randn[float32](tensor.Shape{64, 1, 28, 28}, backend).Raw()
	kernel := tensor.Randn[float32](tensor.Shape{32, 1, 3, 3}, backend).Raw()
	grad := tensor.Randn[float32](tensor.Shape{64, 32, 26, 26}, backend).Raw()

	b.ResetTimer()
	for b.Loop() {
		backend.Conv2DInputBackward(input, kernel, grad, 1, 1)
	}
}

func BenchmarkConv2DInputBackward_MultiChannel(b *testing.B) {
	backend := New()

	input := tensor.Randn[float32](tensor.Shape{1, 16, 14, 14}, backend).Raw()
	kernel := tensor.Randn[float32](tensor.Shape{32, 16, 3, 3}, backend).Raw()
	grad := tensor.Randn[float32](tensor.Shape{1, 32, 12, 12}, backend).Raw()

	b.ResetTimer()
	for b.Loop() {
		backend.Conv2DInputBackward(input, kernel, grad, 1, 1)
	}
}

func BenchmarkConv2DInputBackward_Deep(b *testing.B) {
	backend := New()

	input := tensor.Randn[float32](tensor.Shape{8, 64, 14, 14}, backend).Raw()
	kernel := tensor.Randn[float32](tensor.Shape{128, 64, 3, 3}, backend).Raw()
	grad := tensor.Randn[float32](tensor.Shape{8, 128, 12, 12}, backend).Raw()

	b.ResetTimer()
	for b.Loop() {
		backend.Conv2DInputBackward(input, kernel, grad, 1, 1)
	}
}

func BenchmarkConv2DKernelBackward_Batch(b *testing.B) {
	backend := New()

	input := tensor.Randn[float32](tensor.Shape{64, 1, 28, 28}, backend).Raw()
	kernel := tensor.Randn[float32](tensor.Shape{32, 1, 3, 3}, backend).Raw()
	grad := tensor.Randn[float32](tensor.Shape{64, 32, 26, 26}, backend).Raw()

	b.ResetTimer()
	for b.Loop() {
		backend.Conv2DKernelBackward(input, kernel, grad, 1, 1)
	}
}

func BenchmarkConv2DKernelBackward_MultiChannel(b *testing.B) {
	backend := New()

	input := tensor.Randn[float32](tensor.Shape{1, 16, 14, 14}, backend).Raw()
	kernel := tensor.Randn[float32](tensor.Shape{32, 16, 3, 3}, backend).Raw()
	grad := tensor.Randn[float32](tensor.Shape{1, 32, 12, 12}, backend).Raw()

	b.ResetTimer()
	for b.Loop() {
		backend.Conv2DKernelBackward(input, kernel, grad, 1, 1)
	}
}

func BenchmarkConv2DKernelBackward_Deep(b *testing.B) {
	backend := New()

	input := tensor.Randn[float32](tensor.Shape{8, 64, 14, 14}, backend).Raw()
	kernel := tensor.Randn[float32](tensor.Shape{128, 64, 3, 3}, backend).Raw()
	grad := tensor.Randn[float32](tensor.Shape{8, 128, 12, 12}, backend).Raw()

	b.ResetTimer()
	for b.Loop() {
		backend.Conv2DKernelBackward(input, kernel, grad, 1, 1)
	}
}
