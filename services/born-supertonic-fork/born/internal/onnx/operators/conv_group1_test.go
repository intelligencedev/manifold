//go:build !wasm

package operators

import (
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// TestConv_Group1MultiChannelSpatial covers the stem-conv shape class: a
// group=1 convolution with Cin>1 and a spatial kernel, which sums over
// multiple input channels inside born's Conv2D (the path the real BirdNET
// v2.4 stem conv uses; previously untested directly).
func TestConv_Group1MultiChannelSpatial(t *testing.T) {
	// Cin=2: ch0 = 1..9, ch1 = 10..90 (step 10). Both kernels all-ones 2x2,
	// so each output is the sum of the ch0 window plus the ch1 window.
	x := convF32(t, tensor.Shape{1, 2, 3, 3}, []float32{
		1, 2, 3, 4, 5, 6, 7, 8, 9,
		10, 20, 30, 40, 50, 60, 70, 80, 90,
	})
	w := convF32(t, tensor.Shape{1, 2, 2, 2}, []float32{
		1, 1, 1, 1,
		1, 1, 1, 1,
	})
	out := runConv(t, []Attribute{cAttrInts("kernel_shape", 2, 2)}, x, w)
	convAssertShape(t, out, tensor.Shape{1, 1, 2, 2})
	convAssertClose(t, out.AsFloat32(), []float32{132, 176, 264, 308})
}

// TestConv_ChannelMismatchError: a group=1 weight whose in-channels disagree
// with the input must return an error, not panic (the underlying Conv2D
// kernel panics on this; the operator must guard it).
func TestConv_ChannelMismatchError(t *testing.T) {
	x := convF32(t, tensor.Shape{1, 3, 3, 3}, make([]float32, 27))
	w := convF32(t, tensor.Shape{1, 1, 2, 2}, []float32{1, 2, 3, 4})
	if err := runConvErr([]Attribute{cAttrInts("kernel_shape", 2, 2)}, x, w); err == nil {
		t.Fatal("expected error for input/weight channel mismatch, got nil")
	}
}

// TestConv_KernelTooLargeError: a kernel larger than the (padded) input must
// return an error, not panic.
func TestConv_KernelTooLargeError(t *testing.T) {
	x := convF32(t, tensor.Shape{1, 1, 2, 2}, []float32{1, 2, 3, 4})
	w := convF32(t, tensor.Shape{1, 1, 3, 3}, make([]float32, 9))
	if err := runConvErr([]Attribute{cAttrInts("kernel_shape", 3, 3)}, x, w); err == nil {
		t.Fatal("expected error for kernel larger than input, got nil")
	}
}
