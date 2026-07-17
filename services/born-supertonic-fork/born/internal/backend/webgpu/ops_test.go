//go:build windows || linux

package webgpu

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// Helper to create a float32 tensor with given data.
func createTensor(t *testing.T, shape tensor.Shape, data []float32) *tensor.RawTensor {
	t.Helper()
	raw, err := tensor.NewRaw(shape, tensor.Float32, tensor.WebGPU)
	if err != nil {
		t.Fatalf("failed to create tensor: %v", err)
	}
	// Copy data
	byteData := raw.Data()
	for i, v := range data {
		bits := math.Float32bits(v)
		byteData[i*4+0] = byte(bits)
		byteData[i*4+1] = byte(bits >> 8)
		byteData[i*4+2] = byte(bits >> 16)
		byteData[i*4+3] = byte(bits >> 24)
	}
	return raw
}

// Helper to extract float32 slice from tensor.
func extractData(t *testing.T, raw *tensor.RawTensor) []float32 {
	t.Helper()
	byteData := raw.Data()
	result := make([]float32, raw.NumElements())
	for i := range result {
		bits := uint32(byteData[i*4+0]) |
			uint32(byteData[i*4+1])<<8 |
			uint32(byteData[i*4+2])<<16 |
			uint32(byteData[i*4+3])<<24
		result[i] = math.Float32frombits(bits)
	}
	return result
}

// Helper to compare float32 slices with tolerance.
func compareSlices(t *testing.T, expected, actual []float32, tolerance float32) bool {
	t.Helper()
	if len(expected) != len(actual) {
		t.Errorf("length mismatch: expected %d, got %d", len(expected), len(actual))
		return false
	}
	for i := range expected {
		diff := expected[i] - actual[i]
		if diff < 0 {
			diff = -diff
		}
		if diff > tolerance {
			t.Errorf("value mismatch at index %d: expected %f, got %f (diff: %f)", i, expected[i], actual[i], diff)
			return false
		}
	}
	return true
}

func createInt32Tensor(t *testing.T, shape tensor.Shape, data []int32) *tensor.RawTensor {
	t.Helper()
	raw, err := tensor.NewRaw(shape, tensor.Int32, tensor.WebGPU)
	if err != nil {
		t.Fatalf("failed to create tensor: %v", err)
	}
	byteData := raw.Data()
	for i, v := range data {
		binary.LittleEndian.PutUint32(byteData[i*4:i*4+4], uint32(v))
	}
	return raw
}

func extractInt32Data(t *testing.T, raw *tensor.RawTensor) []int32 {
	t.Helper()
	byteData := raw.Data()
	result := make([]int32, raw.NumElements())
	for i := range result {
		result[i] = int32(binary.LittleEndian.Uint32(byteData[i*4 : i*4+4]))
	}
	return result
}

func compareInt32Slices(t *testing.T, expected, actual []int32) bool {
	t.Helper()
	if len(expected) != len(actual) {
		t.Errorf("length mismatch: expected %d, got %d", len(expected), len(actual))
		return false
	}
	for i := range expected {
		if expected[i] != actual[i] {
			t.Errorf("value mismatch at index %d: expected %d, got %d", i, expected[i], actual[i])
			return false
		}
	}
	return true
}
func TestAdd(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	// Test case: [1, 2, 3, 4] + [5, 6, 7, 8] = [6, 8, 10, 12]
	a := createTensor(t, tensor.Shape{4}, []float32{1, 2, 3, 4})
	b := createTensor(t, tensor.Shape{4}, []float32{5, 6, 7, 8})

	result := backend.Add(a, b)

	expected := []float32{6, 8, 10, 12}
	actual := extractData(t, result)

	if !compareSlices(t, expected, actual, 1e-6) {
		t.Errorf("Add failed: expected %v, got %v", expected, actual)
	}
}

func TestSub(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	// Test case: [10, 20, 30, 40] - [1, 2, 3, 4] = [9, 18, 27, 36]
	a := createTensor(t, tensor.Shape{4}, []float32{10, 20, 30, 40})
	b := createTensor(t, tensor.Shape{4}, []float32{1, 2, 3, 4})

	result := backend.Sub(a, b)

	expected := []float32{9, 18, 27, 36}
	actual := extractData(t, result)

	if !compareSlices(t, expected, actual, 1e-6) {
		t.Errorf("Sub failed: expected %v, got %v", expected, actual)
	}
}

func TestMul(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	// Test case: [1, 2, 3, 4] * [2, 3, 4, 5] = [2, 6, 12, 20]
	a := createTensor(t, tensor.Shape{4}, []float32{1, 2, 3, 4})
	b := createTensor(t, tensor.Shape{4}, []float32{2, 3, 4, 5})

	result := backend.Mul(a, b)

	expected := []float32{2, 6, 12, 20}
	actual := extractData(t, result)

	if !compareSlices(t, expected, actual, 1e-6) {
		t.Errorf("Mul failed: expected %v, got %v", expected, actual)
	}
}

func TestDiv(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	// Test case: [10, 20, 30, 40] / [2, 4, 5, 8] = [5, 5, 6, 5]
	a := createTensor(t, tensor.Shape{4}, []float32{10, 20, 30, 40})
	b := createTensor(t, tensor.Shape{4}, []float32{2, 4, 5, 8})

	result := backend.Div(a, b)

	expected := []float32{5, 5, 6, 5}
	actual := extractData(t, result)

	if !compareSlices(t, expected, actual, 1e-6) {
		t.Errorf("Div failed: expected %v, got %v", expected, actual)
	}
}

func TestMatMul(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	// Test case: [2x3] @ [3x2] = [2x2]
	// A = [[1, 2, 3], [4, 5, 6]]
	// B = [[1, 2], [3, 4], [5, 6]]
	// C = [[1*1+2*3+3*5, 1*2+2*4+3*6], [4*1+5*3+6*5, 4*2+5*4+6*6]]
	//   = [[22, 28], [49, 64]]
	a := createTensor(t, tensor.Shape{2, 3}, []float32{1, 2, 3, 4, 5, 6})
	b := createTensor(t, tensor.Shape{3, 2}, []float32{1, 2, 3, 4, 5, 6})

	result := backend.MatMul(a, b)

	expected := []float32{22, 28, 49, 64}
	actual := extractData(t, result)

	if !compareSlices(t, expected, actual, 1e-5) {
		t.Errorf("MatMul failed: expected %v, got %v", expected, actual)
	}

	// Verify shape
	if !result.Shape().Equal(tensor.Shape{2, 2}) {
		t.Errorf("MatMul shape mismatch: expected [2,2], got %v", result.Shape())
	}
}

func TestTranspose(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	// Test case: transpose [2x3] to [3x2]
	// A = [[1, 2, 3], [4, 5, 6]]
	// A^T = [[1, 4], [2, 5], [3, 6]]
	a := createTensor(t, tensor.Shape{2, 3}, []float32{1, 2, 3, 4, 5, 6})

	result := backend.Transpose(a)

	expected := []float32{1, 4, 2, 5, 3, 6}
	actual := extractData(t, result)

	if !compareSlices(t, expected, actual, 1e-6) {
		t.Errorf("Transpose failed: expected %v, got %v", expected, actual)
	}

	// Verify shape
	if !result.Shape().Equal(tensor.Shape{3, 2}) {
		t.Errorf("Transpose shape mismatch: expected [3,2], got %v", result.Shape())
	}
}

func TestReshape(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	// Test case: reshape [2x3] to [3x2]
	a := createTensor(t, tensor.Shape{2, 3}, []float32{1, 2, 3, 4, 5, 6})

	result := backend.Reshape(a, tensor.Shape{3, 2})

	expected := []float32{1, 2, 3, 4, 5, 6}
	actual := extractData(t, result)

	if !compareSlices(t, expected, actual, 1e-6) {
		t.Errorf("Reshape failed: expected %v, got %v", expected, actual)
	}

	// Verify shape
	if !result.Shape().Equal(tensor.Shape{3, 2}) {
		t.Errorf("Reshape shape mismatch: expected [3,2], got %v", result.Shape())
	}
}

func TestLargeAdd(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	// Test with larger tensor (1024 elements)
	size := 1024
	aData := make([]float32, size)
	bData := make([]float32, size)
	expected := make([]float32, size)
	for i := 0; i < size; i++ {
		aData[i] = float32(i)
		bData[i] = float32(i * 2)
		expected[i] = float32(i * 3)
	}

	a := createTensor(t, tensor.Shape{size}, aData)
	b := createTensor(t, tensor.Shape{size}, bData)

	result := backend.Add(a, b)
	actual := extractData(t, result)

	if !compareSlices(t, expected, actual, 1e-5) {
		t.Errorf("Large Add failed")
	}
}

func TestLargeMatMul(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	// Test with larger matrices: [64x64] @ [64x64]
	size := 64
	aData := make([]float32, size*size)
	bData := make([]float32, size*size)
	for i := 0; i < size*size; i++ {
		aData[i] = 1.0
		bData[i] = 1.0
	}

	a := createTensor(t, tensor.Shape{size, size}, aData)
	b := createTensor(t, tensor.Shape{size, size}, bData)

	result := backend.MatMul(a, b)

	// When multiplying identity-like matrices, each element should be `size`
	actual := extractData(t, result)
	for i, v := range actual {
		if math.Abs(float64(v-float32(size))) > 1e-3 {
			t.Errorf("Large MatMul failed at index %d: expected %d, got %f", i, size, v)
			break
		}
	}
}

func TestReLU(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	// Test case: ReLU([-2, -1, 0, 1, 2]) = [0, 0, 0, 1, 2]
	x := createTensor(t, tensor.Shape{5}, []float32{-2, -1, 0, 1, 2})

	result := backend.ReLU(x)

	expected := []float32{0, 0, 0, 1, 2}
	actual := extractData(t, result)

	if !compareSlices(t, expected, actual, 1e-6) {
		t.Errorf("ReLU failed: expected %v, got %v", expected, actual)
	}
}

func TestSigmoid(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	// Test case: Sigmoid([0]) = [0.5]
	x := createTensor(t, tensor.Shape{3}, []float32{0, -100, 100})

	result := backend.Sigmoid(x)

	expected := []float32{0.5, 0, 1}
	actual := extractData(t, result)

	if !compareSlices(t, expected, actual, 1e-4) {
		t.Errorf("Sigmoid failed: expected %v, got %v", expected, actual)
	}
}

func TestTanh(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	// Test case: Tanh([0]) = [0]
	x := createTensor(t, tensor.Shape{3}, []float32{0, -100, 100})

	result := backend.Tanh(x)

	expected := []float32{0, -1, 1}
	actual := extractData(t, result)

	if !compareSlices(t, expected, actual, 1e-4) {
		t.Errorf("Tanh failed: expected %v, got %v", expected, actual)
	}
}

func TestSoftmax(t *testing.T) {
	if !computeAvailable {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Release()

	// Test case: Softmax([[1, 2, 3], [1, 1, 1]]) - should sum to 1 per row
	x := createTensor(t, tensor.Shape{2, 3}, []float32{1, 2, 3, 1, 1, 1})

	result := backend.Softmax(x, -1)
	actual := extractData(t, result)

	// Check that each row sums to 1
	sum1 := actual[0] + actual[1] + actual[2]
	sum2 := actual[3] + actual[4] + actual[5]

	if math.Abs(float64(sum1-1.0)) > 1e-5 {
		t.Errorf("Softmax row 1 doesn't sum to 1: %v (sum=%f)", actual[:3], sum1)
	}
	if math.Abs(float64(sum2-1.0)) > 1e-5 {
		t.Errorf("Softmax row 2 doesn't sum to 1: %v (sum=%f)", actual[3:6], sum2)
	}

	// Second row should be uniform distribution [1/3, 1/3, 1/3]
	expectedUniform := float32(1.0 / 3.0)
	for i := 3; i < 6; i++ {
		if math.Abs(float64(actual[i]-expectedUniform)) > 1e-5 {
			t.Errorf("Softmax uniform distribution failed at %d: expected %f, got %f", i, expectedUniform, actual[i])
		}
	}
}
