package loader

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// createTestSafeTensorsFile creates a minimal SafeTensors file for testing.
func createTestSafeTensorsFile(t *testing.T, path string) {
	t.Helper()

	// Create test tensors
	tensors := map[string]SafeTensorInfo{
		"weight": {
			DType:       SafeTensorsF32,
			Shape:       []int{2, 3},
			DataOffsets: [2]int64{0, 24}, // 2*3*4 = 24 bytes
		},
		"bias": {
			DType:       SafeTensorsF32,
			Shape:       []int{3},
			DataOffsets: [2]int64{24, 36}, // 3*4 = 12 bytes
		},
	}

	// Create header JSON
	headerMap := make(map[string]interface{})
	headerMap["__metadata__"] = map[string]string{"format": "pt"}
	for name, info := range tensors {
		headerMap[name] = info
	}

	headerJSON, err := json.Marshal(headerMap)
	if err != nil {
		t.Fatalf("Failed to marshal header: %v", err)
	}

	// Create file
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer file.Close()

	// Write header size (8 bytes, little-endian)
	headerSize := uint64(len(headerJSON))
	if err := binary.Write(file, binary.LittleEndian, headerSize); err != nil {
		t.Fatalf("Failed to write header size: %v", err)
	}

	// Write header JSON
	if _, err := file.Write(headerJSON); err != nil {
		t.Fatalf("Failed to write header: %v", err)
	}

	// Write tensor data
	// weight: [2, 3] = [[1, 2, 3], [4, 5, 6]]
	weightData := []float32{1, 2, 3, 4, 5, 6}
	for _, v := range weightData {
		if err := binary.Write(file, binary.LittleEndian, v); err != nil {
			t.Fatalf("Failed to write weight data: %v", err)
		}
	}

	// bias: [3] = [0.1, 0.2, 0.3]
	biasData := []float32{0.1, 0.2, 0.3}
	for _, v := range biasData {
		if err := binary.Write(file, binary.LittleEndian, v); err != nil {
			t.Fatalf("Failed to write bias data: %v", err)
		}
	}
}

func TestNewSafeTensorsReader(t *testing.T) {
	// Create temporary directory
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.safetensors")

	// Create test file
	createTestSafeTensorsFile(t, testFile)

	// Open reader
	reader, err := NewSafeTensorsReader(testFile)
	if err != nil {
		t.Fatalf("NewSafeTensorsReader failed: %v", err)
	}
	defer reader.Close()

	// Check metadata
	metadata := reader.Metadata()
	if metadata["format"] != "pt" {
		t.Errorf("Expected format=pt, got %s", metadata["format"])
	}

	// Check tensor names
	names := reader.TensorNames()
	if len(names) != 2 {
		t.Errorf("Expected 2 tensors, got %d", len(names))
	}
}

func TestSafeTensorsReader_TensorInfo(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.safetensors")
	createTestSafeTensorsFile(t, testFile)

	reader, err := NewSafeTensorsReader(testFile)
	if err != nil {
		t.Fatalf("NewSafeTensorsReader failed: %v", err)
	}
	defer reader.Close()

	// Test existing tensor
	info, err := reader.TensorInfo("weight")
	if err != nil {
		t.Fatalf("TensorInfo failed: %v", err)
	}

	if info.DType != SafeTensorsF32 {
		t.Errorf("Expected dtype F32, got %s", info.DType)
	}

	if len(info.Shape) != 2 || info.Shape[0] != 2 || info.Shape[1] != 3 {
		t.Errorf("Expected shape [2, 3], got %v", info.Shape)
	}

	// Test non-existent tensor
	_, err = reader.TensorInfo("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent tensor")
	}
}

func TestSafeTensorsReader_ReadTensorData(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.safetensors")
	createTestSafeTensorsFile(t, testFile)

	reader, err := NewSafeTensorsReader(testFile)
	if err != nil {
		t.Fatalf("NewSafeTensorsReader failed: %v", err)
	}
	defer reader.Close()

	// Read weight tensor
	data, err := reader.ReadTensorData("weight")
	if err != nil {
		t.Fatalf("ReadTensorData failed: %v", err)
	}

	expectedSize := 2 * 3 * 4 // 2*3 elements * 4 bytes per float32
	if len(data) != expectedSize {
		t.Errorf("Expected %d bytes, got %d", expectedSize, len(data))
	}

	// Verify first float32 value (just check size for now)
	// Detailed value checking is done in TestSafeTensorsReader_LoadTensor
	if len(data) == 0 {
		t.Error("Expected non-empty data")
	}
}

func TestSafeTensorsReader_LoadTensor(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.safetensors")
	createTestSafeTensorsFile(t, testFile)

	reader, err := NewSafeTensorsReader(testFile)
	if err != nil {
		t.Fatalf("NewSafeTensorsReader failed: %v", err)
	}
	defer reader.Close()

	backend := cpu.New()

	// Load weight tensor
	raw, err := reader.LoadTensor("weight", backend)
	if err != nil {
		t.Fatalf("LoadTensor failed: %v", err)
	}

	// Check shape
	shape := raw.Shape()
	if len(shape) != 2 || shape[0] != 2 || shape[1] != 3 {
		t.Errorf("Expected shape [2, 3], got %v", shape)
	}

	// Check dtype
	if raw.DType() != tensor.Float32 {
		t.Errorf("Expected dtype Float32, got %v", raw.DType())
	}

	// Check data
	data := raw.AsFloat32()
	expected := []float32{1, 2, 3, 4, 5, 6}
	for i, v := range expected {
		if data[i] != v {
			t.Errorf("Expected data[%d]=%f, got %f", i, v, data[i])
		}
	}
}

func TestSafeTensorsReader_LoadBias(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.safetensors")
	createTestSafeTensorsFile(t, testFile)

	reader, err := NewSafeTensorsReader(testFile)
	if err != nil {
		t.Fatalf("NewSafeTensorsReader failed: %v", err)
	}
	defer reader.Close()

	backend := cpu.New()

	// Load bias tensor
	raw, err := reader.LoadTensor("bias", backend)
	if err != nil {
		t.Fatalf("LoadTensor failed: %v", err)
	}

	// Check shape
	shape := raw.Shape()
	if len(shape) != 1 || shape[0] != 3 {
		t.Errorf("Expected shape [3], got %v", shape)
	}

	// Check data
	data := raw.AsFloat32()
	expected := []float32{0.1, 0.2, 0.3}
	for i, v := range expected {
		if !floatEqual(data[i], v, 1e-6) {
			t.Errorf("Expected data[%d]=%f, got %f", i, v, data[i])
		}
	}
}

// Helper function for float comparison.
func floatEqual(a, b, epsilon float32) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}
