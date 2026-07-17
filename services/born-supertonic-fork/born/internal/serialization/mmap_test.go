package serialization

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"unsafe"

	"github.com/born-ml/born/internal/tensor"
)

// createTestFile creates a .born file for testing.
func createTestFile(t *testing.T, path string, stateDict map[string]*tensor.RawTensor) {
	t.Helper()

	writer, err := NewBornWriter(path)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}
	defer writer.Close()

	if err := writer.WriteStateDictV2(stateDict, "TestModel", nil); err != nil {
		t.Fatalf("Failed to write state dict: %v", err)
	}
}

func TestMmapReaderBasic(t *testing.T) {
	// Create test data
	backend := tensor.NewMockBackend()

	raw1, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create tensor: %v", err)
	}
	data1 := raw1.AsFloat32()
	copy(data1, []float32{1.0, 2.0, 3.0, 4.0})

	raw2, err := tensor.NewRaw(tensor.Shape{2}, tensor.Float64, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create tensor: %v", err)
	}
	data2 := raw2.AsFloat64()
	copy(data2, []float64{5.0, 6.0})

	stateDict := map[string]*tensor.RawTensor{
		"weight": raw1,
		"bias":   raw2,
	}

	// Write file
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.born")
	createTestFile(t, path, stateDict)

	// Open with MmapReader
	reader, err := NewMmapReader(path)
	if err != nil {
		t.Fatalf("Failed to create mmap reader: %v", err)
	}
	defer reader.Close()

	// Verify header
	header := reader.Header()
	if len(header.Tensors) != 2 {
		t.Errorf("Expected 2 tensors, got %d", len(header.Tensors))
	}

	// Verify tensor names
	names := reader.TensorNames()
	if len(names) != 2 {
		t.Errorf("Expected 2 tensor names, got %d", len(names))
	}

	// Verify tensor info
	weightInfo, err := reader.TensorInfo("weight")
	if err != nil {
		t.Fatalf("Failed to get weight info: %v", err)
	}
	if weightInfo.DType != "float32" {
		t.Errorf("Expected dtype float32, got %s", weightInfo.DType)
	}
	if !reflect.DeepEqual(weightInfo.Shape, []int{2, 2}) {
		t.Errorf("Expected shape [2, 2], got %v", weightInfo.Shape)
	}

	// Read tensor data
	weightData, err := reader.TensorData("weight")
	if err != nil {
		t.Fatalf("Failed to read weight data: %v", err)
	}

	expectedBytes := raw1.Data()
	if !reflect.DeepEqual(weightData, expectedBytes) {
		t.Errorf("Weight data mismatch")
	}

	// Test LoadTensor
	loadedWeight, err := reader.LoadTensor("weight", backend)
	if err != nil {
		t.Fatalf("Failed to load weight: %v", err)
	}

	loadedData := loadedWeight.AsFloat32()
	if !reflect.DeepEqual(loadedData, []float32{1.0, 2.0, 3.0, 4.0}) {
		t.Errorf("Loaded weight data mismatch:\nExpected: [1 2 3 4]\nGot: %v", loadedData)
	}

	// Test ReadStateDict
	loadedStateDict, err := reader.ReadStateDict(backend)
	if err != nil {
		t.Fatalf("Failed to read state dict: %v", err)
	}

	if len(loadedStateDict) != 2 {
		t.Errorf("Expected 2 tensors in state dict, got %d", len(loadedStateDict))
	}
}

func TestMmapReaderZeroCopy(t *testing.T) {
	// Create test data
	backend := tensor.NewMockBackend()
	raw, err := tensor.NewRaw(tensor.Shape{4}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create tensor: %v", err)
	}
	data := raw.AsFloat32()
	copy(data, []float32{1.0, 2.0, 3.0, 4.0})

	stateDict := map[string]*tensor.RawTensor{"data": raw}

	// Write file
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.born")
	createTestFile(t, path, stateDict)

	// Open with MmapReader
	reader, err := NewMmapReader(path)
	if err != nil {
		t.Fatalf("Failed to create mmap reader: %v", err)
	}
	defer reader.Close()

	// Get zero-copy data
	tensorData, err := reader.TensorData("data")
	if err != nil {
		t.Fatalf("Failed to get tensor data: %v", err)
	}

	// Verify it's within mmap bounds (address check)
	mmapStart := uintptr(unsafe.Pointer(&reader.data[0]))
	mmapEnd := mmapStart + uintptr(len(reader.data))
	dataStart := uintptr(unsafe.Pointer(&tensorData[0]))

	if dataStart < mmapStart || dataStart >= mmapEnd {
		t.Errorf("TensorData returned data outside mmap region:\nMmap: [%x, %x)\nData: %x",
			mmapStart, mmapEnd, dataStart)
	}

	// Verify TensorDataCopy returns a different address
	copiedData, err := reader.TensorDataCopy("data")
	if err != nil {
		t.Fatalf("Failed to copy tensor data: %v", err)
	}

	copiedStart := uintptr(unsafe.Pointer(&copiedData[0]))
	if copiedStart >= mmapStart && copiedStart < mmapEnd {
		t.Errorf("TensorDataCopy returned data inside mmap region (should be a copy)")
	}

	// Verify copied data matches
	if !reflect.DeepEqual(tensorData, copiedData) {
		t.Errorf("Copied data doesn't match original")
	}
}

func TestMmapReaderNotFound(t *testing.T) {
	// Create test data
	backend := tensor.NewMockBackend()
	raw, err := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create tensor: %v", err)
	}

	stateDict := map[string]*tensor.RawTensor{"existing": raw}

	// Write file
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.born")
	createTestFile(t, path, stateDict)

	// Open with MmapReader
	reader, err := NewMmapReader(path)
	if err != nil {
		t.Fatalf("Failed to create mmap reader: %v", err)
	}
	defer reader.Close()

	// Try to get non-existent tensor
	_, err = reader.TensorInfo("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent tensor, got nil")
	}

	_, err = reader.TensorData("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent tensor data, got nil")
	}
}

func TestMmapReaderClosed(t *testing.T) {
	// Create test data
	backend := tensor.NewMockBackend()
	raw, err := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create tensor: %v", err)
	}

	stateDict := map[string]*tensor.RawTensor{"data": raw}

	// Write file
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.born")
	createTestFile(t, path, stateDict)

	// Open and close reader
	reader, err := NewMmapReader(path)
	if err != nil {
		t.Fatalf("Failed to create mmap reader: %v", err)
	}
	reader.Close()

	// Try to use closed reader
	_, err = reader.TensorData("data")
	if err == nil {
		t.Error("Expected error when accessing data from closed reader")
	}

	_, err = reader.LoadTensor("data", backend)
	if err == nil {
		t.Error("Expected error when loading tensor from closed reader")
	}

	// Close again should be safe
	if err := reader.Close(); err != nil {
		t.Errorf("Second close should not error, got: %v", err)
	}
}

func TestMmapReaderInvalidFile(t *testing.T) {
	tests := []struct {
		name     string
		contents []byte
		wantErr  bool
	}{
		{
			name:     "empty file",
			contents: []byte{},
			wantErr:  true,
		},
		{
			name:     "too small",
			contents: []byte("BORN"),
			wantErr:  true,
		},
		{
			name:     "invalid magic",
			contents: []byte("XXXX\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, "invalid.born")

			if err := os.WriteFile(path, tt.contents, 0o600); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			reader, err := NewMmapReader(path)
			if reader != nil {
				defer reader.Close()
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("NewMmapReader() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMmapReaderMultipleTensors(t *testing.T) {
	// Create test data with multiple tensors of different types
	backend := tensor.NewMockBackend()

	// Float32 tensor
	raw1, err := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create tensor: %v", err)
	}
	data1 := raw1.AsFloat32()
	copy(data1, []float32{1, 2, 3, 4, 5, 6})

	// Float64 tensor
	raw2, err := tensor.NewRaw(tensor.Shape{2}, tensor.Float64, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create tensor: %v", err)
	}
	data2 := raw2.AsFloat64()
	copy(data2, []float64{7.5, 8.5})

	// Int32 tensor
	raw3, err := tensor.NewRaw(tensor.Shape{4}, tensor.Int32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create tensor: %v", err)
	}
	data3 := raw3.AsInt32()
	copy(data3, []int32{10, 20, 30, 40})

	// Int64 tensor
	raw4, err := tensor.NewRaw(tensor.Shape{3}, tensor.Int64, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create tensor: %v", err)
	}
	data4 := raw4.AsInt64()
	copy(data4, []int64{100, 200, 300})

	stateDict := map[string]*tensor.RawTensor{
		"float32_tensor": raw1,
		"float64_tensor": raw2,
		"int32_tensor":   raw3,
		"int64_tensor":   raw4,
	}

	// Write file
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.born")
	createTestFile(t, path, stateDict)

	// Open with MmapReader
	reader, err := NewMmapReader(path)
	if err != nil {
		t.Fatalf("Failed to create mmap reader: %v", err)
	}
	defer reader.Close()

	// Verify all tensors can be read
	tensorTests := []struct {
		name     string
		expected []byte
	}{
		{"float32_tensor", raw1.Data()},
		{"float64_tensor", raw2.Data()},
		{"int32_tensor", raw3.Data()},
		{"int64_tensor", raw4.Data()},
	}

	for _, tt := range tensorTests {
		data, err := reader.TensorData(tt.name)
		if err != nil {
			t.Errorf("Failed to read tensor %s: %v", tt.name, err)
			continue
		}

		if !reflect.DeepEqual(data, tt.expected) {
			t.Errorf("Tensor %s data mismatch", tt.name)
		}
	}
}

func TestMmapReaderVersionAndFlags(t *testing.T) {
	// Create test data
	backend := tensor.NewMockBackend()
	raw, err := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create tensor: %v", err)
	}

	stateDict := map[string]*tensor.RawTensor{"data": raw}

	// Write file
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.born")
	createTestFile(t, path, stateDict)

	// Open with MmapReader
	reader, err := NewMmapReader(path)
	if err != nil {
		t.Fatalf("Failed to create mmap reader: %v", err)
	}
	defer reader.Close()

	// Verify version (should be v2 since we're using WriteStateDictV2)
	version := reader.Version()
	if version != FormatVersionV2 {
		t.Errorf("Expected version %d, got %d", FormatVersionV2, version)
	}

	// Flags should be readable
	_ = reader.Flags()

	// Checksum should be present for v2
	checksum := reader.Checksum()
	allZero := true
	for _, b := range checksum {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("Expected non-zero checksum for v2 file")
	}
}

// BenchmarkMmapVsRegularSmall benchmarks small file loading (1MB).
func BenchmarkMmapVsRegularSmall(b *testing.B) {
	benchmarkMmapVsRegular(b, 1024*256) // 256K elements = 1MB float32
}

// BenchmarkMmapVsRegularMedium benchmarks medium file loading (10MB).
func BenchmarkMmapVsRegularMedium(b *testing.B) {
	benchmarkMmapVsRegular(b, 1024*1024*2) // 2M elements ~= 8MB float32
}

// BenchmarkMmapVsRegularLarge benchmarks large file loading (100MB).
func BenchmarkMmapVsRegularLarge(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping large file benchmark in short mode")
	}
	benchmarkMmapVsRegular(b, 1024*1024*25) // 25M elements = 100MB float32
}

// createBenchFile creates a test file for benchmarking.
func createBenchFile(b *testing.B, numElements int) (string, tensor.Backend) {
	b.Helper()

	backend := tensor.NewMockBackend()
	raw, err := tensor.NewRaw(tensor.Shape{numElements}, tensor.Float32, backend.Device())
	if err != nil {
		b.Fatalf("Failed to create tensor: %v", err)
	}
	data := raw.AsFloat32()
	for i := range data {
		data[i] = float32(i)
	}

	stateDict := map[string]*tensor.RawTensor{"large_tensor": raw}

	tmpDir := b.TempDir()
	path := filepath.Join(tmpDir, "bench.born")
	writer, err := NewBornWriter(path)
	if err != nil {
		b.Fatalf("Failed to create writer: %v", err)
	}
	if err := writer.WriteStateDictV2(stateDict, "BenchModel", nil); err != nil {
		b.Fatalf("Failed to write state dict: %v", err)
	}
	writer.Close()

	return path, backend
}

//nolint:gocognit // Benchmark requires testing multiple scenarios
func benchmarkMmapVsRegular(b *testing.B, numElements int) {
	path, backend := createBenchFile(b, numElements)

	b.Run("Regular", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			reader, err := NewBornReader(path)
			if err != nil {
				b.Fatalf("Failed to create reader: %v", err)
			}
			_, err = reader.LoadTensor("large_tensor", backend)
			if err != nil {
				b.Fatalf("Failed to load tensor: %v", err)
			}
			reader.Close()
		}
	})

	b.Run("Mmap", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			reader, err := NewMmapReader(path)
			if err != nil {
				b.Fatalf("Failed to create reader: %v", err)
			}
			_, err = reader.LoadTensor("large_tensor", backend)
			if err != nil {
				b.Fatalf("Failed to load tensor: %v", err)
			}
			reader.Close()
		}
	})

	b.Run("MmapZeroCopy", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			reader, err := NewMmapReader(path)
			if err != nil {
				b.Fatalf("Failed to create reader: %v", err)
			}
			_, err = reader.TensorData("large_tensor")
			if err != nil {
				b.Fatalf("Failed to get tensor data: %v", err)
			}
			reader.Close()
		}
	})
}
