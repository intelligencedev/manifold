package serialization

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/loader"
	"github.com/born-ml/born/internal/tensor"
)

// TestSafeTensorsExportBasic tests basic SafeTensors export.
func TestSafeTensorsExportBasic(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.safetensors")

	backend := cpu.New()

	// Create test tensors
	weight, err := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create weight tensor: %v", err)
	}
	weightData := weight.AsFloat32()
	for i := range weightData {
		weightData[i] = float32(i + 1)
	}

	bias, err := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create bias tensor: %v", err)
	}
	biasData := bias.AsFloat32()
	for i := range biasData {
		biasData[i] = float32(i+1) * 0.1
	}

	stateDict := map[string]*tensor.RawTensor{
		"weight": weight,
		"bias":   bias,
	}

	// Export
	metadata := map[string]string{
		"format":    "pt",
		"framework": "born",
	}

	err = WriteSafeTensors(testFile, stateDict, metadata)
	if err != nil {
		t.Fatalf("WriteSafeTensors failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Fatal("SafeTensors file was not created")
	}
}

// TestSafeTensorsExportRoundTrip tests round-trip: write → read → verify.
func TestSafeTensorsExportRoundTrip(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "roundtrip.safetensors")

	backend := cpu.New()

	// Create test tensors with known values
	weight, err := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create weight tensor: %v", err)
	}
	weightData := weight.AsFloat32()
	expectedWeight := []float32{1, 2, 3, 4, 5, 6}
	copy(weightData, expectedWeight)

	bias, err := tensor.NewRaw(tensor.Shape{3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create bias tensor: %v", err)
	}
	biasData := bias.AsFloat32()
	expectedBias := []float32{0.1, 0.2, 0.3}
	copy(biasData, expectedBias)

	original := map[string]*tensor.RawTensor{
		"weight": weight,
		"bias":   bias,
	}

	metadata := map[string]string{
		"format": "pt",
	}

	// Export
	err = WriteSafeTensors(testFile, original, metadata)
	if err != nil {
		t.Fatalf("WriteSafeTensors failed: %v", err)
	}

	// Read back with existing SafeTensorsReader
	reader, err := loader.NewSafeTensorsReader(testFile)
	if err != nil {
		t.Fatalf("NewSafeTensorsReader failed: %v", err)
	}
	defer reader.Close()

	// Verify metadata
	readMetadata := reader.Metadata()
	if readMetadata["format"] != "pt" {
		t.Errorf("Expected format=pt, got %s", readMetadata["format"])
	}

	// Verify tensor count
	names := reader.TensorNames()
	if len(names) != 2 {
		t.Errorf("Expected 2 tensors, got %d", len(names))
	}

	// Load and verify weight tensor
	loadedWeight, err := reader.LoadTensor("weight", backend)
	if err != nil {
		t.Fatalf("Failed to load weight: %v", err)
	}

	if !tensorEqual(weight, loadedWeight) {
		t.Error("Weight tensor mismatch after round-trip")
	}

	// Load and verify bias tensor
	loadedBias, err := reader.LoadTensor("bias", backend)
	if err != nil {
		t.Fatalf("Failed to load bias: %v", err)
	}

	if !tensorEqual(bias, loadedBias) {
		t.Error("Bias tensor mismatch after round-trip")
	}
}

// TestSafeTensorsExportFloat64 tests export with float64 dtype.
func TestSafeTensorsExportFloat64(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "float64.safetensors")

	backend := cpu.New()

	// Create float64 tensor
	tensor64, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float64, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create tensor: %v", err)
	}
	data := tensor64.AsFloat64()
	expected := []float64{1.1, 2.2, 3.3, 4.4}
	copy(data, expected)

	stateDict := map[string]*tensor.RawTensor{
		"tensor64": tensor64,
	}

	// Export
	err = WriteSafeTensors(testFile, stateDict, nil)
	if err != nil {
		t.Fatalf("WriteSafeTensors failed: %v", err)
	}

	// Read back
	reader, err := loader.NewSafeTensorsReader(testFile)
	if err != nil {
		t.Fatalf("NewSafeTensorsReader failed: %v", err)
	}
	defer reader.Close()

	// Verify dtype
	info, err := reader.TensorInfo("tensor64")
	if err != nil {
		t.Fatalf("TensorInfo failed: %v", err)
	}

	if info.DType != loader.SafeTensorsF64 {
		t.Errorf("Expected dtype F64, got %s", info.DType)
	}

	// Load and verify
	loaded, err := reader.LoadTensor("tensor64", backend)
	if err != nil {
		t.Fatalf("Failed to load tensor: %v", err)
	}

	if !tensorEqual(tensor64, loaded) {
		t.Error("Float64 tensor mismatch after round-trip")
	}
}

// TestSafeTensorsExportInt32 tests export with int32 dtype.
func TestSafeTensorsExportInt32(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "int32.safetensors")

	backend := cpu.New()

	// Create int32 tensor
	tensorInt, err := tensor.NewRaw(tensor.Shape{4}, tensor.Int32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create tensor: %v", err)
	}
	data := tensorInt.AsInt32()
	expected := []int32{10, 20, 30, 40}
	copy(data, expected)

	stateDict := map[string]*tensor.RawTensor{
		"indices": tensorInt,
	}

	// Export
	err = WriteSafeTensors(testFile, stateDict, nil)
	if err != nil {
		t.Fatalf("WriteSafeTensors failed: %v", err)
	}

	// Read back
	reader, err := loader.NewSafeTensorsReader(testFile)
	if err != nil {
		t.Fatalf("NewSafeTensorsReader failed: %v", err)
	}
	defer reader.Close()

	// Verify dtype
	info, err := reader.TensorInfo("indices")
	if err != nil {
		t.Fatalf("TensorInfo failed: %v", err)
	}

	if info.DType != loader.SafeTensorsI32 {
		t.Errorf("Expected dtype I32, got %s", info.DType)
	}

	// Load and verify
	loaded, err := reader.LoadTensor("indices", backend)
	if err != nil {
		t.Fatalf("Failed to load tensor: %v", err)
	}

	if !tensorEqual(tensorInt, loaded) {
		t.Error("Int32 tensor mismatch after round-trip")
	}
}

// TestSafeTensorsExportMultipleShapes tests export with various tensor shapes.
func TestSafeTensorsExportMultipleShapes(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "shapes.safetensors")

	backend := cpu.New()

	// Create tensors with different shapes
	scalar, _ := tensor.NewRaw(tensor.Shape{}, tensor.Float32, backend.Device())
	scalar.AsFloat32()[0] = 42.0

	vector, _ := tensor.NewRaw(tensor.Shape{5}, tensor.Float32, backend.Device())
	matrix, _ := tensor.NewRaw(tensor.Shape{3, 4}, tensor.Float32, backend.Device())
	tensor3d, _ := tensor.NewRaw(tensor.Shape{2, 3, 4}, tensor.Float32, backend.Device())

	stateDict := map[string]*tensor.RawTensor{
		"scalar":   scalar,
		"vector":   vector,
		"matrix":   matrix,
		"tensor3d": tensor3d,
	}

	// Export
	err := WriteSafeTensors(testFile, stateDict, nil)
	if err != nil {
		t.Fatalf("WriteSafeTensors failed: %v", err)
	}

	// Read back
	reader, err := loader.NewSafeTensorsReader(testFile)
	if err != nil {
		t.Fatalf("NewSafeTensorsReader failed: %v", err)
	}
	defer reader.Close()

	// Verify shapes
	tests := []struct {
		name          string
		expectedShape []int
	}{
		{"scalar", []int{}},
		{"vector", []int{5}},
		{"matrix", []int{3, 4}},
		{"tensor3d", []int{2, 3, 4}},
	}

	for _, tt := range tests {
		info, err := reader.TensorInfo(tt.name)
		if err != nil {
			t.Errorf("TensorInfo(%s) failed: %v", tt.name, err)
			continue
		}

		if len(info.Shape) != len(tt.expectedShape) {
			t.Errorf("%s: expected shape length %d, got %d", tt.name, len(tt.expectedShape), len(info.Shape))
			continue
		}

		for i, dim := range tt.expectedShape {
			//nolint:unconvert // Conversion needed for int64 to int comparison
			if int(info.Shape[i]) != dim {
				t.Errorf("%s: shape[%d] expected %d, got %d", tt.name, i, dim, info.Shape[i])
			}
		}
	}
}

// TestSafeTensorsExportEmptyMetadata tests export with nil metadata.
func TestSafeTensorsExportEmptyMetadata(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "no_metadata.safetensors")

	backend := cpu.New()

	tensor1, _ := tensor.NewRaw(tensor.Shape{2}, tensor.Float32, backend.Device())
	stateDict := map[string]*tensor.RawTensor{
		"tensor": tensor1,
	}

	// Export with nil metadata
	err := WriteSafeTensors(testFile, stateDict, nil)
	if err != nil {
		t.Fatalf("WriteSafeTensors failed: %v", err)
	}

	// Read back
	reader, err := loader.NewSafeTensorsReader(testFile)
	if err != nil {
		t.Fatalf("NewSafeTensorsReader failed: %v", err)
	}
	defer reader.Close()

	// Metadata should be empty (SafeTensorsReader returns nil for empty metadata)
	metadata := reader.Metadata()
	if len(metadata) > 0 {
		t.Errorf("Expected empty metadata, got %v", metadata)
	}
}

// TestSafeTensorsExportAlphabeticalOrder tests that tensors are written in alphabetical order.
func TestSafeTensorsExportAlphabeticalOrder(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "order.safetensors")

	backend := cpu.New()

	// Create tensors with non-alphabetical insertion order
	z, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	z.AsFloat32()[0] = 3.0

	a, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	a.AsFloat32()[0] = 1.0

	m, _ := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	m.AsFloat32()[0] = 2.0

	stateDict := map[string]*tensor.RawTensor{
		"z_last":  z,
		"a_first": a,
		"m_mid":   m,
	}

	// Export
	err := WriteSafeTensors(testFile, stateDict, nil)
	if err != nil {
		t.Fatalf("WriteSafeTensors failed: %v", err)
	}

	// Read back
	reader, err := loader.NewSafeTensorsReader(testFile)
	if err != nil {
		t.Fatalf("NewSafeTensorsReader failed: %v", err)
	}
	defer reader.Close()

	// Verify all tensors can be loaded correctly
	loadedA, _ := reader.LoadTensor("a_first", backend)
	loadedM, _ := reader.LoadTensor("m_mid", backend)
	loadedZ, _ := reader.LoadTensor("z_last", backend)

	if loadedA.AsFloat32()[0] != 1.0 {
		t.Errorf("Expected a_first=1.0, got %f", loadedA.AsFloat32()[0])
	}
	if loadedM.AsFloat32()[0] != 2.0 {
		t.Errorf("Expected m_mid=2.0, got %f", loadedM.AsFloat32()[0])
	}
	if loadedZ.AsFloat32()[0] != 3.0 {
		t.Errorf("Expected z_last=3.0, got %f", loadedZ.AsFloat32()[0])
	}
}

// Helper function to compare two RawTensors.
func tensorEqual(a, b *tensor.RawTensor) bool {
	// Check shape
	if len(a.Shape()) != len(b.Shape()) {
		return false
	}
	for i := range a.Shape() {
		if a.Shape()[i] != b.Shape()[i] {
			return false
		}
	}

	// Check dtype
	if a.DType() != b.DType() {
		return false
	}

	// Check data
	aData := a.Data()
	bData := b.Data()
	if len(aData) != len(bData) {
		return false
	}
	for i := range aData {
		if aData[i] != bData[i] {
			return false
		}
	}

	return true
}
