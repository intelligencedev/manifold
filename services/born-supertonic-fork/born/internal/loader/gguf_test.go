package loader

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

//nolint:gocognit // Test helper function with sequential file writing operations
func createTestGGUFFile(t *testing.T, path string) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer file.Close()

	// Write magic: "GGUF" = 0x46554747
	magic := uint32(0x46554747)
	if err := binary.Write(file, binary.LittleEndian, magic); err != nil {
		t.Fatalf("Failed to write magic: %v", err)
	}

	// Write version: 3
	version := uint32(3)
	if err := binary.Write(file, binary.LittleEndian, version); err != nil {
		t.Fatalf("Failed to write version: %v", err)
	}

	// Write tensor count: 1
	tensorCount := uint64(1)
	if err := binary.Write(file, binary.LittleEndian, tensorCount); err != nil {
		t.Fatalf("Failed to write tensor count: %v", err)
	}

	// Write metadata count: 2
	metadataCount := uint64(2)
	if err := binary.Write(file, binary.LittleEndian, metadataCount); err != nil {
		t.Fatalf("Failed to write metadata count: %v", err)
	}

	// Write metadata: "model.name" = "test"
	writeString := func(s string) error {
		length := uint64(len(s))
		if err := binary.Write(file, binary.LittleEndian, length); err != nil {
			return err
		}
		_, err := file.WriteString(s)
		return err
	}

	// Metadata 1: model.name = "test"
	if err := writeString("model.name"); err != nil {
		t.Fatalf("Failed to write metadata key: %v", err)
	}
	metaType := uint32(GGUFTypeString)
	if err := binary.Write(file, binary.LittleEndian, metaType); err != nil {
		t.Fatalf("Failed to write metadata type: %v", err)
	}
	if err := writeString("test"); err != nil {
		t.Fatalf("Failed to write metadata value: %v", err)
	}

	// Metadata 2: vocab_size = 1000
	if err := writeString("vocab_size"); err != nil {
		t.Fatalf("Failed to write metadata key: %v", err)
	}
	metaType = uint32(GGUFTypeUint32)
	if err := binary.Write(file, binary.LittleEndian, metaType); err != nil {
		t.Fatalf("Failed to write metadata type: %v", err)
	}
	vocabSize := uint32(1000)
	if err := binary.Write(file, binary.LittleEndian, vocabSize); err != nil {
		t.Fatalf("Failed to write vocab_size: %v", err)
	}

	// Write tensor info: "weight" [2, 3] F32
	if err := writeString("weight"); err != nil {
		t.Fatalf("Failed to write tensor name: %v", err)
	}

	// n_dims = 2
	nDims := uint32(2)
	if err := binary.Write(file, binary.LittleEndian, nDims); err != nil {
		t.Fatalf("Failed to write n_dims: %v", err)
	}

	// dims = [2, 3] (reversed order in GGUF)
	dims := []uint64{2, 3}
	for _, dim := range dims {
		if err := binary.Write(file, binary.LittleEndian, dim); err != nil {
			t.Fatalf("Failed to write dim: %v", err)
		}
	}

	// dtype = F32 (0)
	dtype := uint32(GGUFDTypeF32)
	if err := binary.Write(file, binary.LittleEndian, dtype); err != nil {
		t.Fatalf("Failed to write dtype: %v", err)
	}

	// offset = 0
	offset := uint64(0)
	if err := binary.Write(file, binary.LittleEndian, offset); err != nil {
		t.Fatalf("Failed to write offset: %v", err)
	}

	// Get current position
	currentPos, err := file.Seek(0, 1)
	if err != nil {
		t.Fatalf("Failed to get current position: %v", err)
	}

	// Align to 32 bytes
	alignedPos := alignOffset(uint64(currentPos), 32)
	padding := alignedPos - uint64(currentPos)
	if padding > 0 {
		paddingBytes := make([]byte, padding)
		if _, err := file.Write(paddingBytes); err != nil {
			t.Fatalf("Failed to write padding: %v", err)
		}
	}

	// Write tensor data: [1, 2, 3, 4, 5, 6]
	tensorData := []float32{1, 2, 3, 4, 5, 6}
	for _, v := range tensorData {
		if err := binary.Write(file, binary.LittleEndian, v); err != nil {
			t.Fatalf("Failed to write tensor data: %v", err)
		}
	}
}

func TestNewGGUFReader(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.gguf")
	createTestGGUFFile(t, testFile)

	reader, err := NewGGUFReader(testFile)
	if err != nil {
		t.Fatalf("NewGGUFReader failed: %v", err)
	}
	defer reader.Close()

	// Check version
	if reader.version != 3 {
		t.Errorf("Expected version 3, got %d", reader.version)
	}

	// Check metadata
	metadata := reader.Metadata()
	if len(metadata) != 2 {
		t.Errorf("Expected 2 metadata entries, got %d", len(metadata))
	}

	modelName, ok := metadata["model.name"].(string)
	if !ok || modelName != "test" {
		t.Errorf("Expected model.name=test, got %v", metadata["model.name"])
	}

	vocabSize, ok := metadata["vocab_size"].(uint32)
	if !ok || vocabSize != 1000 {
		t.Errorf("Expected vocab_size=1000, got %v", metadata["vocab_size"])
	}
}

func TestGGUFReader_TensorNames(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.gguf")
	createTestGGUFFile(t, testFile)

	reader, err := NewGGUFReader(testFile)
	if err != nil {
		t.Fatalf("NewGGUFReader failed: %v", err)
	}
	defer reader.Close()

	names := reader.TensorNames()
	if len(names) != 1 {
		t.Errorf("Expected 1 tensor, got %d", len(names))
	}

	if names[0] != "weight" {
		t.Errorf("Expected tensor name 'weight', got '%s'", names[0])
	}
}

func TestGGUFReader_TensorInfo(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.gguf")
	createTestGGUFFile(t, testFile)

	reader, err := NewGGUFReader(testFile)
	if err != nil {
		t.Fatalf("NewGGUFReader failed: %v", err)
	}
	defer reader.Close()

	info, err := reader.TensorInfo("weight")
	if err != nil {
		t.Fatalf("TensorInfo failed: %v", err)
	}

	if info.Name != "weight" {
		t.Errorf("Expected name 'weight', got '%s'", info.Name)
	}

	if len(info.Dims) != 2 || info.Dims[0] != 2 || info.Dims[1] != 3 {
		t.Errorf("Expected dims [2, 3], got %v", info.Dims)
	}

	if info.DType != GGUFDTypeF32 {
		t.Errorf("Expected dtype F32, got %v", info.DType)
	}
}

func TestGGUFReader_ReadTensorData(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.gguf")
	createTestGGUFFile(t, testFile)

	reader, err := NewGGUFReader(testFile)
	if err != nil {
		t.Fatalf("NewGGUFReader failed: %v", err)
	}
	defer reader.Close()

	data, err := reader.ReadTensorData("weight")
	if err != nil {
		t.Fatalf("ReadTensorData failed: %v", err)
	}

	expectedSize := 2 * 3 * 4 // 2*3 elements * 4 bytes per float32
	if len(data) != expectedSize {
		t.Errorf("Expected %d bytes, got %d", expectedSize, len(data))
	}

	// Verify first value
	var firstValue float32
	if err := binary.Read(newBytesReader(data[0:4]), binary.LittleEndian, &firstValue); err != nil {
		t.Fatalf("Failed to read first value: %v", err)
	}

	if firstValue != 1.0 {
		t.Errorf("Expected first value 1.0, got %f", firstValue)
	}
}

func TestGGUFReader_NonExistentTensor(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.gguf")
	createTestGGUFFile(t, testFile)

	reader, err := NewGGUFReader(testFile)
	if err != nil {
		t.Fatalf("NewGGUFReader failed: %v", err)
	}
	defer reader.Close()

	_, err = reader.TensorInfo("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent tensor")
	}
}

// newBytesReader creates an io.Reader from a byte slice.
func newBytesReader(data []byte) *bytesReader {
	return &bytesReader{data: data, pos: 0}
}

type bytesReader struct {
	data []byte
	pos  int
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, os.ErrClosed
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
