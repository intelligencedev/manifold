package gguf

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// createTestGGUFFile creates a minimal GGUF file for testing.
func createTestGGUFFile(t *testing.T, tensorCount int) *File {
	t.Helper()

	// Create temporary file
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.gguf")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create test file: %v", err)
	}
	defer func() {
		_ = f.Close()
	}()

	buf := new(bytes.Buffer)
	order := binary.LittleEndian

	// Write header
	_ = binary.Write(buf, order, MagicGGUFLE)
	_ = binary.Write(buf, order, Version3)
	_ = binary.Write(buf, order, uint64(tensorCount))
	_ = binary.Write(buf, order, uint64(0)) // No metadata

	// Write tensor info
	tensors := make([]TensorInfo, tensorCount)
	currentOffset := uint64(0)

	for i := 0; i < tensorCount; i++ {
		name := "tensor" + string(rune('0'+i))
		tensors[i] = TensorInfo{
			Name:       name,
			NDims:      2,
			Dimensions: []uint64{2, 3},
			Type:       GGMLTypeF32,
			Offset:     currentOffset,
		}

		// Write tensor info
		writeTestString(buf, order, name)
		_ = binary.Write(buf, order, uint32(2))
		_ = binary.Write(buf, order, uint64(2))
		_ = binary.Write(buf, order, uint64(3))
		_ = binary.Write(buf, order, uint32(GGMLTypeF32))
		_ = binary.Write(buf, order, currentOffset)

		currentOffset += tensors[i].Size()
	}

	// Calculate tensor data offset
	headerSize := int64(buf.Len())
	tensorDataOffset := alignOffset(headerSize, DefaultAlignment)

	// Write to file
	if _, err := f.Write(buf.Bytes()); err != nil {
		t.Fatalf("write header: %v", err)
	}

	// Write padding
	padding := tensorDataOffset - headerSize
	if _, err := f.Write(make([]byte, padding)); err != nil {
		t.Fatalf("write padding: %v", err)
	}

	// Write tensor data (float32 values)
	for i := 0; i < tensorCount; i++ {
		for j := 0; j < 6; j++ { // 2x3 = 6 elements
			value := float32(i*10 + j)
			if err := binary.Write(f, order, value); err != nil {
				t.Fatalf("write tensor data: %v", err)
			}
		}
	}

	// Create File struct
	file := &File{
		Header: Header{
			Magic:           MagicGGUFLE,
			Version:         Version3,
			TensorCount:     uint64(tensorCount),
			MetadataKVCount: 0,
		},
		Metadata:         make(map[string]interface{}),
		TensorInfo:       tensors,
		Alignment:        DefaultAlignment,
		TensorDataOffset: tensorDataOffset,
		FilePath:         path,
	}

	return file
}

// writeTestString writes a GGUF string to the buffer (helper for tests).
func writeTestString(buf *bytes.Buffer, order binary.ByteOrder, s string) {
	_ = binary.Write(buf, order, uint64(len(s)))
	_, _ = buf.WriteString(s)
}

func TestLoadTensorData(t *testing.T) {
	file := createTestGGUFFile(t, 2)

	// Test loading first tensor
	data, err := LoadTensorData(file, "tensor0")
	if err != nil {
		t.Fatalf("LoadTensorData failed: %v", err)
	}

	// Check size
	expectedSize := 6 * 4 // 6 float32 elements
	if len(data) != expectedSize {
		t.Errorf("Expected size %d, got %d", expectedSize, len(data))
	}

	// Check first value
	reader := bytes.NewReader(data)
	var value float32
	if err := binary.Read(reader, binary.LittleEndian, &value); err != nil {
		t.Fatalf("Read float32: %v", err)
	}
	if value != 0.0 {
		t.Errorf("Expected first value 0.0, got %f", value)
	}
}

func TestLoadTensorData_NotFound(t *testing.T) {
	file := createTestGGUFFile(t, 1)

	_, err := LoadTensorData(file, "nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent tensor")
	}
	if err.Error() != "tensor not found: nonexistent" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestLoadTensorData_NoFilePath(t *testing.T) {
	file := &File{
		FilePath: "",
	}

	_, err := LoadTensorData(file, "tensor0")
	if err == nil {
		t.Fatal("Expected error when FilePath is empty")
	}
}

func TestLoadAllTensors(t *testing.T) {
	file := createTestGGUFFile(t, 3)

	tensors, err := LoadAllTensors(file)
	if err != nil {
		t.Fatalf("LoadAllTensors failed: %v", err)
	}

	// Check count
	if len(tensors) != 3 {
		t.Errorf("Expected 3 tensors, got %d", len(tensors))
	}

	// Check names
	for i := 0; i < 3; i++ {
		name := "tensor" + string(rune('0'+i))
		if _, ok := tensors[name]; !ok {
			t.Errorf("Tensor %s not found", name)
		}
	}

	// Check tensor0 data
	data := tensors["tensor0"]
	expectedSize := 6 * 4 // 6 float32 elements
	if len(data) != expectedSize {
		t.Errorf("Expected size %d, got %d", expectedSize, len(data))
	}
}

func TestNewTensorReader(t *testing.T) {
	file := createTestGGUFFile(t, 2)

	reader, err := NewTensorReader(file)
	if err != nil {
		t.Fatalf("NewTensorReader failed: %v", err)
	}
	defer func() {
		_ = reader.Close()
	}()

	if reader.file != file {
		t.Error("Reader file mismatch")
	}
	if reader.reader == nil {
		t.Error("Reader reader is nil")
	}
}

func TestTensorReader_ReadTensor(t *testing.T) {
	file := createTestGGUFFile(t, 2)

	reader, err := NewTensorReader(file)
	if err != nil {
		t.Fatalf("NewTensorReader failed: %v", err)
	}
	defer func() {
		_ = reader.Close()
	}()

	// Read tensor0
	data, err := reader.ReadTensor("tensor0")
	if err != nil {
		t.Fatalf("ReadTensor failed: %v", err)
	}

	expectedSize := 6 * 4 // 6 float32 elements
	if len(data) != expectedSize {
		t.Errorf("Expected size %d, got %d", expectedSize, len(data))
	}

	// Read tensor1
	data, err = reader.ReadTensor("tensor1")
	if err != nil {
		t.Fatalf("ReadTensor failed: %v", err)
	}

	// Check first value of tensor1 (should be 10.0)
	r := bytes.NewReader(data)
	var value float32
	if err := binary.Read(r, binary.LittleEndian, &value); err != nil {
		t.Fatalf("Read float32: %v", err)
	}
	if value != 10.0 {
		t.Errorf("Expected first value 10.0, got %f", value)
	}
}

func TestTensorReader_ReadTensorInto(t *testing.T) {
	file := createTestGGUFFile(t, 1)

	reader, err := NewTensorReader(file)
	if err != nil {
		t.Fatalf("NewTensorReader failed: %v", err)
	}
	defer func() {
		_ = reader.Close()
	}()

	// Create buffer
	size := 6 * 4 // 6 float32 elements
	buf := make([]byte, size)

	// Read into buffer
	if err := reader.ReadTensorInto("tensor0", buf); err != nil {
		t.Fatalf("ReadTensorInto failed: %v", err)
	}

	// Check first value
	r := bytes.NewReader(buf)
	var value float32
	if err := binary.Read(r, binary.LittleEndian, &value); err != nil {
		t.Fatalf("Read float32: %v", err)
	}
	if value != 0.0 {
		t.Errorf("Expected first value 0.0, got %f", value)
	}
}

func TestTensorReader_ReadTensorInto_BufferTooSmall(t *testing.T) {
	file := createTestGGUFFile(t, 1)

	reader, err := NewTensorReader(file)
	if err != nil {
		t.Fatalf("NewTensorReader failed: %v", err)
	}
	defer func() {
		_ = reader.Close()
	}()

	// Create too small buffer
	buf := make([]byte, 10)

	err = reader.ReadTensorInto("tensor0", buf)
	if err == nil {
		t.Fatal("Expected error for too small buffer")
	}
}

func TestTensorReader_Close(t *testing.T) {
	file := createTestGGUFFile(t, 1)

	reader, err := NewTensorReader(file)
	if err != nil {
		t.Fatalf("NewTensorReader failed: %v", err)
	}

	// Close should not error
	if err := reader.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Reading after close should fail
	_, err = reader.ReadTensor("tensor0")
	if err == nil {
		t.Error("Expected error when reading after close")
	}
}

func TestTensorReader_CloseWithNonCloser(t *testing.T) {
	// Create a reader that doesn't implement io.Closer
	file := &File{
		TensorDataOffset: 0,
	}

	// Use bytes.Reader which implements io.ReadSeeker but Close is a no-op
	reader := &TensorReader{
		file:   file,
		reader: bytes.NewReader([]byte{}),
	}

	// Close should not error (no-op since bytes.Reader doesn't implement Closer)
	if err := reader.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}
