//go:build !wasm

package operators

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

func makeResizeInput(t *testing.T, shape tensor.Shape, data []float32) *tensor.RawTensor {
	t.Helper()
	raw, err := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(raw.AsFloat32(), data)
	return raw
}

func makeScales(t *testing.T, scales []float32) *tensor.RawTensor {
	t.Helper()
	raw, err := tensor.NewRaw(tensor.Shape{len(scales)}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(raw.AsFloat32(), scales)
	return raw
}

func makeSizes(t *testing.T, sizes []int64) *tensor.RawTensor {
	t.Helper()
	raw, err := tensor.NewRaw(tensor.Shape{len(sizes)}, tensor.Int64, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(raw.AsInt64(), sizes)
	return raw
}

func emptyTensor(_ *testing.T) *tensor.RawTensor {
	return nil
}

func assertApproxEqual(t *testing.T, got, want []float32, tol float32, label string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: length mismatch: got %d, want %d", label, len(got), len(want))
	}
	for i := range got {
		diff := got[i] - want[i]
		if diff < 0 {
			diff = -diff
		}
		if diff > tol {
			t.Errorf("%s element[%d]: got %.6f, want %.6f (diff %.2e)", label, i, got[i], want[i], diff)
		}
	}
}

// TestResize_NearestAsymmetricFloor_Scales2x tests the exact YOLOv5 pattern:
// nearest mode, asymmetric coords, floor rounding, 2x upscale via scales.
func TestResize_NearestAsymmetricFloor_Scales2x(t *testing.T) {
	// 1x1x2x2 → 1x1x4x4
	input := makeResizeInput(t, tensor.Shape{1, 1, 2, 2}, []float32{
		1, 2,
		3, 4,
	})
	roi := emptyTensor(t)
	scales := makeScales(t, []float32{1, 1, 2, 2})

	node := &Node{
		OpType: "Resize",
		Attributes: []Attribute{
			{Name: "mode", S: []byte("nearest")},
			{Name: "coordinate_transformation_mode", S: []byte("asymmetric")},
			{Name: "nearest_mode", S: []byte("floor")},
		},
	}

	results, err := handleResize(nil, node, []*tensor.RawTensor{input, roi, scales})
	if err != nil {
		t.Fatal(err)
	}
	out := results[0]
	if !out.Shape().Equal(tensor.Shape{1, 1, 4, 4}) {
		t.Fatalf("shape: got %v, want [1,1,4,4]", out.Shape())
	}

	// asymmetric + floor: outIdx/scale → floor
	// oh=0: 0/2=0→0, oh=1: 1/2=0.5→0, oh=2: 2/2=1→1, oh=3: 3/2=1.5→1
	want := []float32{
		1, 1, 2, 2,
		1, 1, 2, 2,
		3, 3, 4, 4,
		3, 3, 4, 4,
	}
	assertApproxEqual(t, out.AsFloat32(), want, 1e-6, "nearest_asymmetric_2x")
}

// TestResize_NearestHalfPixel_Scales2x tests nearest + half_pixel + round_prefer_floor.
func TestResize_NearestHalfPixel_Scales2x(t *testing.T) {
	// 1x1x2x2 → 1x1x4x4
	input := makeResizeInput(t, tensor.Shape{1, 1, 2, 2}, []float32{
		1, 2,
		3, 4,
	})
	roi := emptyTensor(t)
	scales := makeScales(t, []float32{1, 1, 2, 2})

	node := &Node{
		OpType: "Resize",
		Attributes: []Attribute{
			{Name: "mode", S: []byte("nearest")},
			{Name: "coordinate_transformation_mode", S: []byte("half_pixel")},
			{Name: "nearest_mode", S: []byte("round_prefer_floor")},
		},
	}

	results, err := handleResize(nil, node, []*tensor.RawTensor{input, roi, scales})
	if err != nil {
		t.Fatal(err)
	}
	out := results[0]
	if !out.Shape().Equal(tensor.Shape{1, 1, 4, 4}) {
		t.Fatalf("shape: got %v, want [1,1,4,4]", out.Shape())
	}

	// half_pixel: (outIdx+0.5)/scale - 0.5
	// oh=0: (0.5)/2-0.5=-0.25→round_prefer_floor→0(clamped), oh=1: (1.5)/2-0.5=0.25→0
	// oh=2: (2.5)/2-0.5=0.75→1(round_prefer_floor: ceil(0.75-0.5)=ceil(0.25)=1)
	// oh=3: (3.5)/2-0.5=1.25→1
	want := []float32{
		1, 1, 2, 2,
		1, 1, 2, 2,
		3, 3, 4, 4,
		3, 3, 4, 4,
	}
	assertApproxEqual(t, out.AsFloat32(), want, 1e-6, "nearest_halfpixel_2x")
}

// TestResize_NearestAsymmetricFloor_Downsample tests 2x downscale.
func TestResize_NearestAsymmetricFloor_Downsample(t *testing.T) {
	// 1x1x4x4 → 1x1x2x2
	input := makeResizeInput(t, tensor.Shape{1, 1, 4, 4}, []float32{
		1, 2, 3, 4,
		5, 6, 7, 8,
		9, 10, 11, 12,
		13, 14, 15, 16,
	})
	roi := emptyTensor(t)
	scales := makeScales(t, []float32{1, 1, 0.5, 0.5})

	node := &Node{
		OpType: "Resize",
		Attributes: []Attribute{
			{Name: "mode", S: []byte("nearest")},
			{Name: "coordinate_transformation_mode", S: []byte("asymmetric")},
			{Name: "nearest_mode", S: []byte("floor")},
		},
	}

	results, err := handleResize(nil, node, []*tensor.RawTensor{input, roi, scales})
	if err != nil {
		t.Fatal(err)
	}
	out := results[0]
	if !out.Shape().Equal(tensor.Shape{1, 1, 2, 2}) {
		t.Fatalf("shape: got %v, want [1,1,2,2]", out.Shape())
	}

	// asymmetric: outIdx/0.5 = outIdx*2 → floor
	// (0,0)→(0,0)=1, (0,1)→(0,2)=3, (1,0)→(2,0)=9, (1,1)→(2,2)=11
	want := []float32{1, 3, 9, 11}
	assertApproxEqual(t, out.AsFloat32(), want, 1e-6, "nearest_downsample")
}

// TestResize_NearestWithSizes tests sizes input instead of scales.
func TestResize_NearestWithSizes(t *testing.T) {
	// 1x1x2x2 → 1x1x7x8 via sizes
	input := makeResizeInput(t, tensor.Shape{1, 1, 2, 2}, []float32{
		1, 2,
		3, 4,
	})
	roi := emptyTensor(t)
	emptyScales := emptyTensor(t)
	sizes := makeSizes(t, []int64{1, 1, 7, 8})

	node := &Node{
		OpType: "Resize",
		Attributes: []Attribute{
			{Name: "mode", S: []byte("nearest")},
			{Name: "coordinate_transformation_mode", S: []byte("asymmetric")},
			{Name: "nearest_mode", S: []byte("floor")},
		},
	}

	results, err := handleResize(nil, node, []*tensor.RawTensor{input, roi, emptyScales, sizes})
	if err != nil {
		t.Fatal(err)
	}
	out := results[0]
	if !out.Shape().Equal(tensor.Shape{1, 1, 7, 8}) {
		t.Fatalf("shape: got %v, want [1,1,7,8]", out.Shape())
	}
	data := out.AsFloat32()
	if len(data) != 56 {
		t.Fatalf("output length: got %d, want 56", len(data))
	}
	// Spot check corners
	if data[0] != 1 {
		t.Errorf("top-left: got %f, want 1", data[0])
	}
	if data[7] != 2 {
		t.Errorf("top-right: got %f, want 2", data[7])
	}
	if data[48] != 3 {
		t.Errorf("bottom-left: got %f, want 3", data[48])
	}
	if data[55] != 4 {
		t.Errorf("bottom-right: got %f, want 4", data[55])
	}
}

// TestResize_LinearHalfPixel_Scales2x tests bilinear interpolation with half_pixel.
func TestResize_LinearHalfPixel_Scales2x(t *testing.T) {
	// 1x1x2x2 → 1x1x4x4
	input := makeResizeInput(t, tensor.Shape{1, 1, 2, 2}, []float32{
		1, 2,
		3, 4,
	})
	roi := emptyTensor(t)
	scales := makeScales(t, []float32{1, 1, 2, 2})

	node := &Node{
		OpType: "Resize",
		Attributes: []Attribute{
			{Name: "mode", S: []byte("linear")},
			{Name: "coordinate_transformation_mode", S: []byte("half_pixel")},
		},
	}

	results, err := handleResize(nil, node, []*tensor.RawTensor{input, roi, scales})
	if err != nil {
		t.Fatal(err)
	}
	out := results[0]
	if !out.Shape().Equal(tensor.Shape{1, 1, 4, 4}) {
		t.Fatalf("shape: got %v, want [1,1,4,4]", out.Shape())
	}

	// half_pixel bilinear: (outIdx+0.5)/scale - 0.5
	// Corners should be exactly the input values (clamped).
	data := out.AsFloat32()
	assertApproxEqual(t, data[:1], []float32{1.0}, 1e-5, "top-left")
	assertApproxEqual(t, data[3:4], []float32{2.0}, 1e-5, "top-right")
	assertApproxEqual(t, data[12:13], []float32{3.0}, 1e-5, "bottom-left")
	assertApproxEqual(t, data[15:16], []float32{4.0}, 1e-5, "bottom-right")
}

// TestResize_LinearAlignCorners tests bilinear with align_corners.
func TestResize_LinearAlignCorners(t *testing.T) {
	// 1x1x2x2 → 1x1x4x4
	input := makeResizeInput(t, tensor.Shape{1, 1, 2, 2}, []float32{
		1, 2,
		3, 4,
	})
	roi := emptyTensor(t)
	scales := makeScales(t, []float32{1, 1, 2, 2})

	node := &Node{
		OpType: "Resize",
		Attributes: []Attribute{
			{Name: "mode", S: []byte("linear")},
			{Name: "coordinate_transformation_mode", S: []byte("align_corners")},
		},
	}

	results, err := handleResize(nil, node, []*tensor.RawTensor{input, roi, scales})
	if err != nil {
		t.Fatal(err)
	}
	out := results[0]
	if !out.Shape().Equal(tensor.Shape{1, 1, 4, 4}) {
		t.Fatalf("shape: got %v, want [1,1,4,4]", out.Shape())
	}

	data := out.AsFloat32()
	// align_corners: corners must match exactly
	assertApproxEqual(t, data[:1], []float32{1.0}, 1e-5, "top-left")
	assertApproxEqual(t, data[3:4], []float32{2.0}, 1e-5, "top-right")
	assertApproxEqual(t, data[12:13], []float32{3.0}, 1e-5, "bottom-left")
	assertApproxEqual(t, data[15:16], []float32{4.0}, 1e-5, "bottom-right")

	// Center value: bilinear interpolation of all 4 corners
	// align_corners (1,1): origH = 1*(2-1)/(4-1) = 1/3, origW = 1/3
	// v = 1*(2/3)*(2/3) + 2*(2/3)*(1/3) + 3*(1/3)*(2/3) + 4*(1/3)*(1/3)
	//   = 4/9 + 4/9 + 6/9 + 4/9 = 18/9 = 2.0
	assertApproxEqual(t, data[5:6], []float32{2.0}, 1e-5, "center(1,1)")
}

// TestResize_MultiBatchMultiChannel verifies N>1, C>1.
func TestResize_MultiBatchMultiChannel(t *testing.T) {
	// 2x2x2x2 → 2x2x4x4
	data := make([]float32, 2*2*2*2)
	for i := range data {
		data[i] = float32(i + 1)
	}
	input := makeResizeInput(t, tensor.Shape{2, 2, 2, 2}, data)
	roi := emptyTensor(t)
	scales := makeScales(t, []float32{1, 1, 2, 2})

	node := &Node{
		OpType: "Resize",
		Attributes: []Attribute{
			{Name: "mode", S: []byte("nearest")},
			{Name: "coordinate_transformation_mode", S: []byte("asymmetric")},
			{Name: "nearest_mode", S: []byte("floor")},
		},
	}

	results, err := handleResize(nil, node, []*tensor.RawTensor{input, roi, scales})
	if err != nil {
		t.Fatal(err)
	}
	out := results[0]
	if !out.Shape().Equal(tensor.Shape{2, 2, 4, 4}) {
		t.Fatalf("shape: got %v, want [2,2,4,4]", out.Shape())
	}
	// Verify first pixel of each (n,c) slice
	outData := out.AsFloat32()
	if outData[0] != 1 { // (0,0,0,0) → input[0]=1
		t.Errorf("batch0 chan0 top-left: got %f, want 1", outData[0])
	}
	if outData[16] != 5 { // (0,1,0,0) → input[4]=5
		t.Errorf("batch0 chan1 top-left: got %f, want 5", outData[16])
	}
	if outData[32] != 9 { // (1,0,0,0) → input[8]=9
		t.Errorf("batch1 chan0 top-left: got %f, want 9", outData[32])
	}
	if outData[48] != 13 { // (1,1,0,0) → input[12]=13
		t.Errorf("batch1 chan1 top-left: got %f, want 13", outData[48])
	}
}

// TestResize_UnsupportedMode verifies cubic returns an error.
func TestResize_UnsupportedMode(t *testing.T) {
	input := makeResizeInput(t, tensor.Shape{1, 1, 2, 2}, []float32{1, 2, 3, 4})
	roi := emptyTensor(t)
	scales := makeScales(t, []float32{1, 1, 2, 2})

	node := &Node{
		OpType: "Resize",
		Attributes: []Attribute{
			{Name: "mode", S: []byte("cubic")},
		},
	}

	_, err := handleResize(nil, node, []*tensor.RawTensor{input, roi, scales})
	if err == nil {
		t.Fatal("expected error for cubic mode")
	}
}

// TestResize_UnsupportedCoordTransform verifies tf_crop_and_resize returns an error.
func TestResize_UnsupportedCoordTransform(t *testing.T) {
	input := makeResizeInput(t, tensor.Shape{1, 1, 2, 2}, []float32{1, 2, 3, 4})
	roi := emptyTensor(t)
	scales := makeScales(t, []float32{1, 1, 2, 2})

	node := &Node{
		OpType: "Resize",
		Attributes: []Attribute{
			{Name: "mode", S: []byte("nearest")},
			{Name: "coordinate_transformation_mode", S: []byte("tf_crop_and_resize")},
		},
	}

	_, err := handleResize(nil, node, []*tensor.RawTensor{input, roi, scales})
	if err == nil {
		t.Fatal("expected error for tf_crop_and_resize")
	}
}

// TestResize_NoScalesNoSizes verifies error when neither scales nor sizes provided.
func TestResize_NoScalesNoSizes(t *testing.T) {
	input := makeResizeInput(t, tensor.Shape{1, 1, 2, 2}, []float32{1, 2, 3, 4})

	node := &Node{
		OpType: "Resize",
		Attributes: []Attribute{
			{Name: "mode", S: []byte("nearest")},
		},
	}

	_, err := handleResize(nil, node, []*tensor.RawTensor{input})
	if err == nil {
		t.Fatal("expected error when neither scales nor sizes provided")
	}
}

// TestResize_IdentityScale verifies scale=1.0 is a no-op.
func TestResize_IdentityScale(t *testing.T) {
	data := []float32{1, 2, 3, 4, 5, 6, 7, 8, 9}
	input := makeResizeInput(t, tensor.Shape{1, 1, 3, 3}, data)
	roi := emptyTensor(t)
	scales := makeScales(t, []float32{1, 1, 1, 1})

	node := &Node{
		OpType: "Resize",
		Attributes: []Attribute{
			{Name: "mode", S: []byte("nearest")},
			{Name: "coordinate_transformation_mode", S: []byte("asymmetric")},
		},
	}

	results, err := handleResize(nil, node, []*tensor.RawTensor{input, roi, scales})
	if err != nil {
		t.Fatal(err)
	}
	assertApproxEqual(t, results[0].AsFloat32(), data, 1e-6, "identity")
}

// TestResize_3xScale tests non-power-of-2 scale factor.
func TestResize_3xScale(t *testing.T) {
	input := makeResizeInput(t, tensor.Shape{1, 1, 2, 2}, []float32{
		1, 2,
		3, 4,
	})
	roi := emptyTensor(t)
	scales := makeScales(t, []float32{1, 1, 3, 3})

	node := &Node{
		OpType: "Resize",
		Attributes: []Attribute{
			{Name: "mode", S: []byte("nearest")},
			{Name: "coordinate_transformation_mode", S: []byte("asymmetric")},
			{Name: "nearest_mode", S: []byte("floor")},
		},
	}

	results, err := handleResize(nil, node, []*tensor.RawTensor{input, roi, scales})
	if err != nil {
		t.Fatal(err)
	}
	out := results[0]
	if !out.Shape().Equal(tensor.Shape{1, 1, 6, 6}) {
		t.Fatalf("shape: got %v, want [1,1,6,6]", out.Shape())
	}
	data := out.AsFloat32()
	// Row 0-2 should be [1,1,1,2,2,2], rows 3-5 should be [3,3,3,4,4,4]
	want := []float32{
		1, 1, 1, 2, 2, 2,
		1, 1, 1, 2, 2, 2,
		1, 1, 1, 2, 2, 2,
		3, 3, 3, 4, 4, 4,
		3, 3, 3, 4, 4, 4,
		3, 3, 3, 4, 4, 4,
	}
	assertApproxEqual(t, data, want, 1e-6, "3x_scale")
}

// TestResize_Registered verifies Resize is in the registry.
func TestResize_Registered(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Get("Resize"); !ok {
		t.Fatal("Resize not registered")
	}
}

// TestResize_NearestCeilMode tests nearest with ceil rounding.
func TestResize_NearestCeilMode(t *testing.T) {
	input := makeResizeInput(t, tensor.Shape{1, 1, 2, 2}, []float32{
		1, 2,
		3, 4,
	})
	roi := emptyTensor(t)
	scales := makeScales(t, []float32{1, 1, 2, 2})

	node := &Node{
		OpType: "Resize",
		Attributes: []Attribute{
			{Name: "mode", S: []byte("nearest")},
			{Name: "coordinate_transformation_mode", S: []byte("asymmetric")},
			{Name: "nearest_mode", S: []byte("ceil")},
		},
	}

	results, err := handleResize(nil, node, []*tensor.RawTensor{input, roi, scales})
	if err != nil {
		t.Fatal(err)
	}
	out := results[0]
	if !out.Shape().Equal(tensor.Shape{1, 1, 4, 4}) {
		t.Fatalf("shape: got %v, want [1,1,4,4]", out.Shape())
	}

	// asymmetric+ceil: outIdx/scale → ceil
	// ow=0: ceil(0)=0, ow=1: ceil(0.5)=1, ow=2: ceil(1)=1, ow=3: ceil(1.5)=2→clamped to 1
	// oh=0→0, oh=1→ceil(0.5)=1, oh=2→ceil(1)=1, oh=3→clamped 1
	want := []float32{
		1, 2, 2, 2,
		3, 4, 4, 4,
		3, 4, 4, 4,
		3, 4, 4, 4,
	}
	assertApproxEqual(t, out.AsFloat32(), want, 1e-6, "nearest_ceil")
}

// TestResize_LinearAsymmetric tests bilinear with asymmetric coordinate transform.
func TestResize_LinearAsymmetric(t *testing.T) {
	input := makeResizeInput(t, tensor.Shape{1, 1, 2, 2}, []float32{
		1, 3,
		5, 7,
	})
	roi := emptyTensor(t)
	scales := makeScales(t, []float32{1, 1, 4, 4})

	node := &Node{
		OpType: "Resize",
		Attributes: []Attribute{
			{Name: "mode", S: []byte("linear")},
			{Name: "coordinate_transformation_mode", S: []byte("asymmetric")},
		},
	}

	results, err := handleResize(nil, node, []*tensor.RawTensor{input, roi, scales})
	if err != nil {
		t.Fatal(err)
	}
	out := results[0]
	if !out.Shape().Equal(tensor.Shape{1, 1, 8, 8}) {
		t.Fatalf("shape: got %v, want [1,1,8,8]", out.Shape())
	}
	data := out.AsFloat32()
	// Corners must match input exactly (asymmetric maps (0,0)→(0,0))
	assertApproxEqual(t, data[:1], []float32{1.0}, 1e-5, "top-left")
	// Verify smooth interpolation: values should increase monotonically along each row
	for i := 0; i < 7; i++ {
		if data[i] > data[i+1] {
			t.Errorf("row 0 not monotonic: data[%d]=%f > data[%d]=%f", i, data[i], i+1, data[i+1])
		}
	}
}

// TestRoundPreferFloor verifies the rounding tie-breaking.
func TestRoundPreferFloor(t *testing.T) {
	tests := []struct {
		in   float64
		want float64
	}{
		{0.5, 0},
		{1.5, 1},
		{2.5, 2},
		{0.4, 0},
		{0.6, 1},
		{-0.5, -1},
	}
	for _, tt := range tests {
		got := roundPreferFloor(tt.in)
		if got != tt.want {
			t.Errorf("roundPreferFloor(%f): got %f, want %f", tt.in, got, tt.want)
		}
	}
}

// TestRoundPreferCeil verifies the rounding tie-breaking.
func TestRoundPreferCeil(t *testing.T) {
	tests := []struct {
		in   float64
		want float64
	}{
		{0.5, 1},
		{1.5, 2},
		{2.5, 3},
		{0.4, 0},
		{0.6, 1},
		{-0.5, 0},
	}
	for _, tt := range tests {
		got := roundPreferCeil(tt.in)
		if got != tt.want {
			t.Errorf("roundPreferCeil(%f): got %f, want %f", tt.in, got, tt.want)
		}
	}
}

// Ensure math import is used.
var _ = math.Floor
