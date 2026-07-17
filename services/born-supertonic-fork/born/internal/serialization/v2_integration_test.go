package serialization

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// TestV2RoundTrip verifies v2 format write and read with checksum validation.
func TestV2RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test_v2.born")

	// Create test tensor
	backend := tensor.NewMockBackend()
	raw, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create tensor: %v", err)
	}
	data := raw.AsFloat32()
	copy(data, []float32{1.0, 2.0, 3.0, 4.0})

	stateDict := map[string]*tensor.RawTensor{
		"weight": raw,
	}

	// Write v2 file
	writer, err := NewBornWriter(path)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}

	if err := writer.WriteStateDictV2(stateDict, "TestModel", map[string]string{"test": "v2"}); err != nil {
		t.Fatalf("Failed to write v2 file: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Failed to close writer: %v", err)
	}

	// Read v2 file with checksum validation
	reader, err := NewBornReader(path)
	if err != nil {
		t.Fatalf("Failed to open v2 file: %v", err)
	}
	defer reader.Close()

	// Verify it's v2
	if reader.version != FormatVersionV2 {
		t.Errorf("Expected version %d, got %d", FormatVersionV2, reader.version)
	}

	// Read state dict
	loadedDict, err := reader.ReadStateDict(backend)
	if err != nil {
		t.Fatalf("Failed to read state dict: %v", err)
	}

	// Verify tensor
	loadedTensor, ok := loadedDict["weight"]
	if !ok {
		t.Fatal("Tensor 'weight' not found")
	}

	loadedData := loadedTensor.AsFloat32()
	expectedData := []float32{1.0, 2.0, 3.0, 4.0}
	if len(loadedData) != len(expectedData) {
		t.Fatalf("Expected %d elements, got %d", len(expectedData), len(loadedData))
	}

	for i, v := range expectedData {
		if loadedData[i] != v {
			t.Errorf("Element %d: expected %f, got %f", i, v, loadedData[i])
		}
	}
}

// TestV2CorruptionDetection verifies that corrupted tensor data is detected by checksum.
func TestV2CorruptionDetection(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test_corrupt.born")

	// Create and write v2 file
	backend := tensor.NewMockBackend()
	raw, err := tensor.NewRaw(tensor.Shape{2, 4}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create tensor: %v", err)
	}
	data := raw.AsFloat32()
	copy(data, []float32{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0})

	stateDict := map[string]*tensor.RawTensor{
		"data": raw,
	}

	writer, err := NewBornWriter(path)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}

	if err := writer.WriteStateDictV2(stateDict, "TestModel", nil); err != nil {
		t.Fatalf("Failed to write v2 file: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Failed to close writer: %v", err)
	}

	// Corrupt 1 byte in tensor data section
	// First, read the file to find where tensor data starts
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}
	fileSize := info.Size()

	// Open file for corruption
	file, err := os.OpenFile(path, os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("Failed to open file for corruption: %v", err)
	}

	// Corrupt the LAST byte (definitely in tensor data)
	if _, err := file.Seek(fileSize-1, 0); err != nil {
		t.Fatalf("Failed to seek: %v", err)
	}

	corruptByte := []byte{0xFF}
	if _, err := file.Write(corruptByte); err != nil {
		t.Fatalf("Failed to corrupt file: %v", err)
	}

	if err := file.Close(); err != nil {
		t.Fatalf("Failed to close file: %v", err)
	}

	// Try to read corrupted file - should fail with checksum mismatch
	_, err = NewBornReader(path)
	if err == nil {
		t.Fatal("Expected checksum validation to fail, but succeeded")
	}

	// Check if error contains ErrChecksumMismatch
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Errorf("Expected ErrChecksumMismatch, got: %v", err)
	}
}

// TestV2SkipChecksumValidation verifies that checksum validation can be skipped.
func TestV2SkipChecksumValidation(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test_skip_checksum.born")

	// Create and write v2 file
	backend := tensor.NewMockBackend()
	raw, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create tensor: %v", err)
	}
	data := raw.AsFloat32()
	copy(data, []float32{1.0, 2.0, 3.0, 4.0})

	stateDict := map[string]*tensor.RawTensor{
		"data": raw,
	}

	writer, err := NewBornWriter(path)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}

	if err := writer.WriteStateDictV2(stateDict, "TestModel", nil); err != nil {
		t.Fatalf("Failed to write v2 file: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Failed to close writer: %v", err)
	}

	// Corrupt the file (last byte = tensor data)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Failed to stat file: %v", err)
	}

	file, err := os.OpenFile(path, os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	if _, err := file.Seek(info.Size()-1, 0); err != nil {
		t.Fatalf("Failed to seek: %v", err)
	}
	if _, err := file.Write([]byte{0xFF}); err != nil {
		t.Fatalf("Failed to corrupt: %v", err)
	}
	file.Close()

	// Read with checksum validation ENABLED - should fail
	_, err = NewBornReaderWithOptions(path, ReaderOptions{
		SkipChecksumValidation: false,
		ValidationLevel:        ValidationStrict,
	})
	if err == nil {
		t.Fatal("Expected checksum validation to fail")
	}

	// Read with checksum validation DISABLED - should succeed
	reader, err := NewBornReaderWithOptions(path, ReaderOptions{
		SkipChecksumValidation: true, // Skip validation
		ValidationLevel:        ValidationNormal,
	})
	if err != nil {
		t.Fatalf("Expected to succeed with skipped validation, got: %v", err)
	}
	defer reader.Close()

	// Should be able to read (though data is corrupt)
	if reader.version != FormatVersionV2 {
		t.Errorf("Expected v2, got v%d", reader.version)
	}
}

// TestV2WithCheckpoint verifies v2 format with checkpoint metadata.
func TestV2WithCheckpoint(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test_checkpoint_v2.born")

	// Create test tensors
	backend := tensor.NewMockBackend()
	weightsRaw, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create weights tensor: %v", err)
	}
	weightsData := weightsRaw.AsFloat32()
	copy(weightsData, []float32{1.0, 2.0, 3.0, 4.0})

	// Optimizer state
	momentumRaw, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create momentum tensor: %v", err)
	}
	momentumData := momentumRaw.AsFloat32()
	copy(momentumData, []float32{0.1, 0.2, 0.3, 0.4})

	stateDict := map[string]*tensor.RawTensor{
		"model.weight":       weightsRaw,
		"optimizer.momentum": momentumRaw,
	}

	// Create header with checkpoint metadata
	header := Header{
		FormatVersion: FormatVersionV2,
		BornVersion:   "0.5.4",
		ModelType:     "TestModel",
		Metadata:      map[string]string{"dataset": "MNIST"},
		CheckpointMeta: &CheckpointMeta{
			IsCheckpoint:  true,
			Epoch:         10,
			Step:          1000,
			Loss:          0.05,
			OptimizerType: "SGD",
			OptimizerConfig: map[string]any{
				"learning_rate": 0.01,
				"momentum":      0.9,
			},
		},
	}

	// Write v2 file with checkpoint
	writer, err := NewBornWriter(path)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}

	if err := writer.WriteStateDictWithHeaderV2(stateDict, header); err != nil {
		t.Fatalf("Failed to write v2 checkpoint: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Failed to close writer: %v", err)
	}

	// Read and verify
	reader, err := NewBornReader(path)
	if err != nil {
		t.Fatalf("Failed to open checkpoint: %v", err)
	}
	defer reader.Close()

	// Verify checkpoint metadata
	readHeader := reader.Header()
	if readHeader.CheckpointMeta == nil {
		t.Fatal("CheckpointMeta is nil")
	}

	if !readHeader.CheckpointMeta.IsCheckpoint {
		t.Error("Expected IsCheckpoint=true")
	}

	if readHeader.CheckpointMeta.Epoch != 10 {
		t.Errorf("Expected epoch 10, got %d", readHeader.CheckpointMeta.Epoch)
	}

	if readHeader.CheckpointMeta.Loss != 0.05 {
		t.Errorf("Expected loss 0.05, got %f", readHeader.CheckpointMeta.Loss)
	}

	// Verify tensors
	loadedDict, err := reader.ReadStateDict(backend)
	if err != nil {
		t.Fatalf("Failed to read state dict: %v", err)
	}

	if len(loadedDict) != 2 {
		t.Errorf("Expected 2 tensors, got %d", len(loadedDict))
	}

	// Verify weight tensor
	if _, ok := loadedDict["model.weight"]; !ok {
		t.Error("model.weight not found")
	}

	// Verify optimizer tensor
	if _, ok := loadedDict["optimizer.momentum"]; !ok {
		t.Error("optimizer.momentum not found")
	}
}

// TestV1Compatibility verifies that v1 files can still be read (no checksum).
func TestV1Compatibility(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test_v1.born")

	// Create and write v1 file
	backend := tensor.NewMockBackend()
	raw, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create tensor: %v", err)
	}
	data := raw.AsFloat32()
	copy(data, []float32{1.0, 2.0, 3.0, 4.0})

	stateDict := map[string]*tensor.RawTensor{
		"weight": raw,
	}

	writer, err := NewBornWriter(path)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}

	// Use v1 writer (WriteStateDict, not WriteStateDictV2)
	if err := writer.WriteStateDict(stateDict, "TestModel", nil); err != nil {
		t.Fatalf("Failed to write v1 file: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Failed to close writer: %v", err)
	}

	// Read v1 file with v2 reader - should work (backward compatibility)
	reader, err := NewBornReader(path)
	if err != nil {
		t.Fatalf("Failed to open v1 file with v2 reader: %v", err)
	}
	defer reader.Close()

	// Should detect as v1
	if reader.version != FormatVersion {
		t.Errorf("Expected v1 format version %d, got %d", FormatVersion, reader.version)
	}

	// Should be able to read normally
	loadedDict, err := reader.ReadStateDict(backend)
	if err != nil {
		t.Fatalf("Failed to read v1 state dict: %v", err)
	}

	if len(loadedDict) != 1 {
		t.Fatalf("Expected 1 tensor, got %d", len(loadedDict))
	}
}

// BenchmarkChecksumOverhead measures checksum computation overhead for different file sizes.
func BenchmarkChecksumOverhead(b *testing.B) {
	sizes := []int{
		1024 * 1024,       // 1 MB
		10 * 1024 * 1024,  // 10 MB
		100 * 1024 * 1024, // 100 MB
	}

	for _, size := range sizes {
		data := make([]byte, size)
		// Fill with some data
		for i := range data {
			data[i] = byte(i % 256)
		}

		b.Run(fmt.Sprintf("%dMB", size/(1024*1024)), func(b *testing.B) {
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = ComputeChecksum(data)
			}
		})
	}
}

// BenchmarkV2WriteWithChecksum benchmarks v2 write performance with checksum.
func BenchmarkV2WriteWithChecksum(b *testing.B) {
	tmpDir := b.TempDir()
	backend := tensor.NewMockBackend()

	// Create 10MB tensor
	numElements := 10 * 1024 * 1024 / 4 // float32 = 4 bytes
	raw, err := tensor.NewRaw(tensor.Shape{numElements}, tensor.Float32, backend.Device())
	if err != nil {
		b.Fatalf("Failed to create tensor: %v", err)
	}
	data := raw.AsFloat32()
	for i := range data {
		data[i] = float32(i)
	}

	stateDict := map[string]*tensor.RawTensor{
		"large_weight": raw,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := filepath.Join(tmpDir, fmt.Sprintf("bench_%d.born", i))
		writer, err := NewBornWriter(path)
		if err != nil {
			b.Fatalf("Failed to create writer: %v", err)
		}

		if err := writer.WriteStateDictV2(stateDict, "BenchModel", nil); err != nil {
			b.Fatalf("Failed to write: %v", err)
		}

		if err := writer.Close(); err != nil {
			b.Fatalf("Failed to close: %v", err)
		}
	}
}

// BenchmarkV2ReadWithChecksum benchmarks v2 read performance with checksum validation.
func BenchmarkV2ReadWithChecksum(b *testing.B) {
	tmpDir := b.TempDir()
	path := filepath.Join(tmpDir, "bench_read.born")
	backend := tensor.NewMockBackend()

	// Create 10MB tensor
	numElements := 10 * 1024 * 1024 / 4
	raw, err := tensor.NewRaw(tensor.Shape{numElements}, tensor.Float32, backend.Device())
	if err != nil {
		b.Fatalf("Failed to create tensor: %v", err)
	}
	data := raw.AsFloat32()
	for i := range data {
		data[i] = float32(i)
	}

	stateDict := map[string]*tensor.RawTensor{
		"large_weight": raw,
	}

	// Write once
	writer, err := NewBornWriter(path)
	if err != nil {
		b.Fatalf("Failed to create writer: %v", err)
	}
	if err := writer.WriteStateDictV2(stateDict, "BenchModel", nil); err != nil {
		b.Fatalf("Failed to write: %v", err)
	}
	writer.Close()

	// Benchmark reading
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader, err := NewBornReader(path)
		if err != nil {
			b.Fatalf("Failed to open: %v", err)
		}

		_, err = reader.ReadStateDict(backend)
		if err != nil {
			b.Fatalf("Failed to read: %v", err)
		}

		reader.Close()
	}
}
