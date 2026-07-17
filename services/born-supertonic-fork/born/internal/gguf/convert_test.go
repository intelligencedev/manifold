package gguf

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// createTestGGUFFileWithType creates a test GGUF file with specific tensor type.
func createTestGGUFFileWithType(t *testing.T, dtype GGMLType) *File {
	t.Helper()

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

	// Write header.
	_ = binary.Write(buf, order, MagicGGUFLE)
	_ = binary.Write(buf, order, Version3)
	_ = binary.Write(buf, order, uint64(1)) // 1 tensor.
	_ = binary.Write(buf, order, uint64(0)) // No metadata.

	// Create tensor info (2x3 matrix = 6 elements).
	tensorInfo := TensorInfo{
		Name:       "test_tensor",
		NDims:      2,
		Dimensions: []uint64{3, 2}, // GGUF order: [cols, rows].
		Type:       dtype,
		Offset:     0,
	}

	// Write tensor info.
	writeTestString(buf, order, tensorInfo.Name)
	_ = binary.Write(buf, order, tensorInfo.NDims)
	_ = binary.Write(buf, order, tensorInfo.Dimensions[0])
	_ = binary.Write(buf, order, tensorInfo.Dimensions[1])
	_ = binary.Write(buf, order, uint32(dtype))
	_ = binary.Write(buf, order, tensorInfo.Offset)

	// Calculate offsets.
	headerSize := int64(buf.Len())
	tensorDataOffset := alignOffset(headerSize, DefaultAlignment)

	// Write header + padding.
	if _, err := f.Write(buf.Bytes()); err != nil {
		t.Fatalf("write header: %v", err)
	}
	padding := tensorDataOffset - headerSize
	if _, err := f.Write(make([]byte, padding)); err != nil {
		t.Fatalf("write padding: %v", err)
	}

	// Write tensor data based on type.
	writeTensorData(t, f, dtype)

	file := &File{
		Header: Header{
			Magic:           MagicGGUFLE,
			Version:         Version3,
			TensorCount:     1,
			MetadataKVCount: 0,
		},
		Metadata:         make(map[string]interface{}),
		TensorInfo:       []TensorInfo{tensorInfo},
		Alignment:        DefaultAlignment,
		TensorDataOffset: tensorDataOffset,
		FilePath:         path,
	}

	return file
}

// writeTensorData writes test tensor data based on type.
func writeTensorData(t *testing.T, f *os.File, dtype GGMLType) {
	t.Helper()

	order := binary.LittleEndian

	switch dtype {
	case GGMLTypeF32:
		// Write 6 float32 values: [1.0, 2.0, 3.0, 4.0, 5.0, 6.0].
		for i := 1; i <= 6; i++ {
			_ = binary.Write(f, order, float32(i))
		}

	case GGMLTypeF16:
		// Write 6 float16 values.
		for i := 1; i <= 6; i++ {
			h := float32ToFloat16(float32(i))
			_ = binary.Write(f, order, h)
		}

	case GGMLTypeQ4_K:
		// Q4_K block: 256 elements, 144 bytes.
		// For simplicity, write zeros (will dequantize to near-zero values).
		blockData := make([]byte, 144)
		// Set d (scale) to 1.0 (float16).
		binary.LittleEndian.PutUint16(blockData[0:2], float32ToFloat16(1.0))
		_, _ = f.Write(blockData)

	default:
		t.Fatalf("unsupported test type: %v", dtype)
	}
}

// float32ToFloat16 converts float32 to float16 (IEEE 754 half precision).
func float32ToFloat16(f float32) uint16 {
	bits := math.Float32bits(f)
	sign := (bits >> 31) & 0x1
	exp := (bits >> 23) & 0xFF
	mant := bits & 0x7FFFFF

	if exp == 0 {
		// Zero or subnormal.
		return uint16(sign << 15)
	}

	// Convert exponent: float32 bias=127, float16 bias=15.
	newExp := int(exp) - 127 + 15

	if newExp <= 0 {
		// Underflow to zero.
		return uint16(sign << 15)
	}
	if newExp >= 31 {
		// Overflow to infinity.
		return uint16(sign<<15) | 0x7C00
	}

	// Normal number.
	return uint16(sign<<15) | uint16(newExp<<10) | uint16(mant>>13)
}

func TestNewTensorConverter(t *testing.T) {
	file := createTestGGUFFileWithType(t, GGMLTypeF32)

	converter, err := NewTensorConverter(file)
	if err != nil {
		t.Fatalf("NewTensorConverter failed: %v", err)
	}
	defer func() {
		_ = converter.Close()
	}()

	if converter.file != file {
		t.Error("Converter file mismatch")
	}
	if converter.reader == nil {
		t.Error("Converter reader is nil")
	}
}

func TestNewTensorConverter_NoFilePath(t *testing.T) {
	file := &File{
		FilePath: "",
	}

	_, err := NewTensorConverter(file)
	if err == nil {
		t.Fatal("Expected error for file without path")
	}
}

func TestConvert_F32(t *testing.T) {
	file := createTestGGUFFileWithType(t, GGMLTypeF32)

	converter, err := NewTensorConverter(file)
	if err != nil {
		t.Fatalf("NewTensorConverter: %v", err)
	}
	defer func() {
		_ = converter.Close()
	}()

	data, shape, err := converter.Convert("test_tensor")
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	// Check shape (GGUF [3, 2] → Born [2, 3]).
	expectedShape := []int{2, 3}
	if len(shape) != len(expectedShape) {
		t.Fatalf("Expected shape length %d, got %d", len(expectedShape), len(shape))
	}
	for i, dim := range expectedShape {
		if shape[i] != dim {
			t.Errorf("Shape[%d]: expected %d, got %d", i, dim, shape[i])
		}
	}

	// Check data.
	expectedData := []float32{1.0, 2.0, 3.0, 4.0, 5.0, 6.0}
	if len(data) != len(expectedData) {
		t.Fatalf("Expected %d elements, got %d", len(expectedData), len(data))
	}
	for i, expected := range expectedData {
		if data[i] != expected {
			t.Errorf("Data[%d]: expected %f, got %f", i, expected, data[i])
		}
	}
}

func TestConvert_F16(t *testing.T) {
	file := createTestGGUFFileWithType(t, GGMLTypeF16)

	converter, err := NewTensorConverter(file)
	if err != nil {
		t.Fatalf("NewTensorConverter: %v", err)
	}
	defer func() {
		_ = converter.Close()
	}()

	data, shape, err := converter.Convert("test_tensor")
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	// Check shape.
	if len(shape) != 2 || shape[0] != 2 || shape[1] != 3 {
		t.Errorf("Expected shape [2, 3], got %v", shape)
	}

	// Check data (should match original within float16 precision).
	expectedData := []float32{1.0, 2.0, 3.0, 4.0, 5.0, 6.0}
	if len(data) != len(expectedData) {
		t.Fatalf("Expected %d elements, got %d", len(expectedData), len(data))
	}
	for i, expected := range expectedData {
		if math.Abs(float64(data[i]-expected)) > 0.01 {
			t.Errorf("Data[%d]: expected %f, got %f", i, expected, data[i])
		}
	}
}

func TestConvert_Q4_K(t *testing.T) {
	file := createTestGGUFFileWithType(t, GGMLTypeQ4_K)

	converter, err := NewTensorConverter(file)
	if err != nil {
		t.Fatalf("NewTensorConverter: %v", err)
	}
	defer func() {
		_ = converter.Close()
	}()

	data, shape, err := converter.Convert("test_tensor")
	if err != nil {
		t.Fatalf("Convert failed: %v", err)
	}

	// Q4_K block size is 256, so we'll have 256 elements.
	// GGUF dims [3, 2] are ignored for Q4_K (block-based format).
	// Shape should be [2, 3] = 6 elements total, but Q4_K works on 256-element blocks.
	// Actually, NumElements() = 3*2 = 6, but dequant returns full block (256).
	if len(data) != 6 {
		t.Errorf("Expected 6 elements, got %d", len(data))
	}

	if len(shape) != 2 {
		t.Errorf("Expected 2D shape, got %v", shape)
	}
}

func TestConvert_NotFound(t *testing.T) {
	file := createTestGGUFFileWithType(t, GGMLTypeF32)

	converter, err := NewTensorConverter(file)
	if err != nil {
		t.Fatalf("NewTensorConverter: %v", err)
	}
	defer func() {
		_ = converter.Close()
	}()

	_, _, err = converter.Convert("nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent tensor")
	}

	expectedMsg := "tensor not found: nonexistent"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestConvertAll(t *testing.T) {
	// Create file with 2 F32 tensors.
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.gguf")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	defer func() {
		_ = f.Close()
	}()

	buf := new(bytes.Buffer)
	order := binary.LittleEndian

	// Header.
	_ = binary.Write(buf, order, MagicGGUFLE)
	_ = binary.Write(buf, order, Version3)
	_ = binary.Write(buf, order, uint64(2)) // 2 tensors.
	_ = binary.Write(buf, order, uint64(0)) // No metadata.

	// Tensor 1: "tensor_a" [2, 3].
	writeTestString(buf, order, "tensor_a")
	_ = binary.Write(buf, order, uint32(2))
	_ = binary.Write(buf, order, uint64(3))
	_ = binary.Write(buf, order, uint64(2))
	_ = binary.Write(buf, order, uint32(GGMLTypeF32))
	_ = binary.Write(buf, order, uint64(0))

	// Tensor 2: "tensor_b" [2, 2].
	writeTestString(buf, order, "tensor_b")
	_ = binary.Write(buf, order, uint32(2))
	_ = binary.Write(buf, order, uint64(2))
	_ = binary.Write(buf, order, uint64(2))
	_ = binary.Write(buf, order, uint32(GGMLTypeF32))
	_ = binary.Write(buf, order, uint64(24)) // Offset after tensor_a (6 floats * 4 bytes).

	// Write header + padding.
	headerSize := int64(buf.Len())
	tensorDataOffset := alignOffset(headerSize, DefaultAlignment)
	_, _ = f.Write(buf.Bytes())
	padding := tensorDataOffset - headerSize
	_, _ = f.Write(make([]byte, padding))

	// Write tensor_a data: [1.0, 2.0, 3.0, 4.0, 5.0, 6.0].
	for i := 1; i <= 6; i++ {
		_ = binary.Write(f, order, float32(i))
	}

	// Write tensor_b data: [10.0, 20.0, 30.0, 40.0].
	for i := 1; i <= 4; i++ {
		_ = binary.Write(f, order, float32(i*10))
	}

	file := &File{
		Header: Header{
			Magic:           MagicGGUFLE,
			Version:         Version3,
			TensorCount:     2,
			MetadataKVCount: 0,
		},
		Metadata: make(map[string]interface{}),
		TensorInfo: []TensorInfo{
			{Name: "tensor_a", NDims: 2, Dimensions: []uint64{3, 2}, Type: GGMLTypeF32, Offset: 0},
			{Name: "tensor_b", NDims: 2, Dimensions: []uint64{2, 2}, Type: GGMLTypeF32, Offset: 24},
		},
		Alignment:        DefaultAlignment,
		TensorDataOffset: tensorDataOffset,
		FilePath:         path,
	}

	converter, err := NewTensorConverter(file)
	if err != nil {
		t.Fatalf("NewTensorConverter: %v", err)
	}
	defer func() {
		_ = converter.Close()
	}()

	tensors, err := converter.ConvertAll()
	if err != nil {
		t.Fatalf("ConvertAll failed: %v", err)
	}

	if len(tensors) != 2 {
		t.Fatalf("Expected 2 tensors, got %d", len(tensors))
	}

	// Check tensor_a.
	ta, ok := tensors["tensor_a"]
	if !ok {
		t.Fatal("tensor_a not found")
	}
	if ta.Name != "tensor_a" {
		t.Errorf("Expected name 'tensor_a', got '%s'", ta.Name)
	}
	if len(ta.Shape) != 2 || ta.Shape[0] != 2 || ta.Shape[1] != 3 {
		t.Errorf("Expected shape [2, 3], got %v", ta.Shape)
	}
	if len(ta.Data) != 6 {
		t.Errorf("Expected 6 elements, got %d", len(ta.Data))
	}
	if ta.OriginalType != GGMLTypeF32 {
		t.Errorf("Expected type F32, got %v", ta.OriginalType)
	}

	// Check tensor_b.
	tb, ok := tensors["tensor_b"]
	if !ok {
		t.Fatal("tensor_b not found")
	}
	if tb.Name != "tensor_b" {
		t.Errorf("Expected name 'tensor_b', got '%s'", tb.Name)
	}
	if len(tb.Shape) != 2 || tb.Shape[0] != 2 || tb.Shape[1] != 2 {
		t.Errorf("Expected shape [2, 2], got %v", tb.Shape)
	}
	if len(tb.Data) != 4 {
		t.Errorf("Expected 4 elements, got %d", len(tb.Data))
	}
}

func TestTensorConverter_Close(t *testing.T) {
	file := createTestGGUFFileWithType(t, GGMLTypeF32)

	converter, err := NewTensorConverter(file)
	if err != nil {
		t.Fatalf("NewTensorConverter: %v", err)
	}

	// Should not error on close.
	if err := converter.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Second close is expected to fail (file already closed).
	// This is normal behavior - we just verify it doesn't panic.
	_ = converter.Close()
}
