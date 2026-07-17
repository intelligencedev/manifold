package loader

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/born-ml/born/internal/tensor"
)

// SafeTensors format:
// [8 bytes: header_size (uint64 LE)]
// [header_size bytes: JSON header]
// [tensor data: raw bytes]

// SafeTensorsDType represents supported SafeTensors data types.
type SafeTensorsDType string

// Supported SafeTensors dtypes.
const (
	SafeTensorsF16  SafeTensorsDType = "F16"
	SafeTensorsF32  SafeTensorsDType = "F32"
	SafeTensorsF64  SafeTensorsDType = "F64"
	SafeTensorsBF16 SafeTensorsDType = "BF16"
	SafeTensorsI32  SafeTensorsDType = "I32"
	SafeTensorsI64  SafeTensorsDType = "I64"
	SafeTensorsU8   SafeTensorsDType = "U8"
	SafeTensorsBool SafeTensorsDType = "BOOL"
)

// SafeTensorInfo describes a tensor in SafeTensors format.
type SafeTensorInfo struct {
	DType       SafeTensorsDType `json:"dtype"`
	Shape       []int            `json:"shape"`
	DataOffsets [2]int64         `json:"data_offsets"` // [start, end]
}

// SafeTensorsHeader is the JSON header in SafeTensors format.
type SafeTensorsHeader struct {
	Metadata map[string]string          `json:"__metadata__"`
	Tensors  map[string]SafeTensorInfo  `json:"-"`
	RawMap   map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON implements custom JSON unmarshaling for SafeTensorsHeader.
func (h *SafeTensorsHeader) UnmarshalJSON(data []byte) error {
	// First parse as generic map
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return err
	}
	h.RawMap = rawMap

	// Extract metadata
	if metadataRaw, ok := rawMap["__metadata__"]; ok {
		if err := json.Unmarshal(metadataRaw, &h.Metadata); err != nil {
			return fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	// Extract tensors (everything except __metadata__)
	h.Tensors = make(map[string]SafeTensorInfo)
	for key, value := range rawMap {
		if key == "__metadata__" {
			continue
		}
		var info SafeTensorInfo
		if err := json.Unmarshal(value, &info); err != nil {
			return fmt.Errorf("failed to unmarshal tensor %s: %w", key, err)
		}
		h.Tensors[key] = info
	}

	return nil
}

// SafeTensorsReader reads SafeTensors format files.
type SafeTensorsReader struct {
	file       *os.File
	header     SafeTensorsHeader
	headerSize uint64
	dataOffset int64 // Offset where tensor data starts
}

// NewSafeTensorsReader creates a new SafeTensors reader.
func NewSafeTensorsReader(path string) (*SafeTensorsReader, error) {
	//nolint:gosec // G304: File path comes from user input, which is expected for model loading
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	// Read header size (8 bytes, little-endian uint64)
	var headerSize uint64
	if err := binary.Read(file, binary.LittleEndian, &headerSize); err != nil {
		_ = file.Close() // Best effort close on error
		return nil, fmt.Errorf("failed to read header size: %w", err)
	}

	// Validate header size (should be reasonable, < 100MB)
	if headerSize > 100*1024*1024 {
		_ = file.Close() // Best effort close on error
		return nil, fmt.Errorf("invalid header size: %d (too large)", headerSize)
	}

	// Read header JSON
	headerBytes := make([]byte, headerSize)
	if _, err := io.ReadFull(file, headerBytes); err != nil {
		_ = file.Close() // Best effort close on error
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	// Parse header
	var header SafeTensorsHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		_ = file.Close() // Best effort close on error
		return nil, fmt.Errorf("failed to parse header JSON: %w", err)
	}

	// Calculate data offset
	dataOffset := int64(8 + headerSize)

	return &SafeTensorsReader{
		file:       file,
		header:     header,
		headerSize: headerSize,
		dataOffset: dataOffset,
	}, nil
}

// Close closes the SafeTensors file.
func (r *SafeTensorsReader) Close() error {
	if r.file != nil {
		return r.file.Close()
	}
	return nil
}

// Metadata returns the metadata map from the header.
func (r *SafeTensorsReader) Metadata() map[string]string {
	return r.header.Metadata
}

// TensorNames returns a list of all tensor names in the file.
func (r *SafeTensorsReader) TensorNames() []string {
	names := make([]string, 0, len(r.header.Tensors))
	for name := range r.header.Tensors {
		names = append(names, name)
	}
	return names
}

// TensorInfo returns information about a specific tensor.
func (r *SafeTensorsReader) TensorInfo(name string) (*SafeTensorInfo, error) {
	info, ok := r.header.Tensors[name]
	if !ok {
		return nil, fmt.Errorf("tensor %s not found", name)
	}
	return &info, nil
}

// ReadTensorData reads raw tensor data for a given tensor name.
func (r *SafeTensorsReader) ReadTensorData(name string) ([]byte, error) {
	info, err := r.TensorInfo(name)
	if err != nil {
		return nil, err
	}

	// Calculate absolute offsets
	start := r.dataOffset + info.DataOffsets[0]
	end := r.dataOffset + info.DataOffsets[1]
	size := end - start

	// Validate offsets
	if size < 0 {
		return nil, fmt.Errorf("invalid data offsets for tensor %s: [%d, %d]",
			name, info.DataOffsets[0], info.DataOffsets[1])
	}

	// Seek to start
	if _, err := r.file.Seek(start, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek to tensor data: %w", err)
	}

	// Read data
	data := make([]byte, size)
	if _, err := io.ReadFull(r.file, data); err != nil {
		return nil, fmt.Errorf("failed to read tensor data: %w", err)
	}

	return data, nil
}

// safeTensorsDTypeToDataType converts SafeTensors dtype to Born DataType.
func safeTensorsDTypeToDataType(dtype SafeTensorsDType) (tensor.DataType, error) {
	switch dtype {
	case SafeTensorsF32:
		return tensor.Float32, nil
	case SafeTensorsF64:
		return tensor.Float64, nil
	case SafeTensorsI32:
		return tensor.Int32, nil
	case SafeTensorsI64:
		return tensor.Int64, nil
	case SafeTensorsU8:
		return tensor.Uint8, nil
	case SafeTensorsBool:
		return tensor.Bool, nil
	case SafeTensorsF16, SafeTensorsBF16:
		// F16 and BF16 not directly supported - caller must handle conversion
		return 0, fmt.Errorf("dtype %s requires conversion (not directly supported)", dtype)
	default:
		return 0, fmt.Errorf("unsupported dtype: %s", dtype)
	}
}

// LoadTensor loads a tensor from SafeTensors file into Born tensor.
// For F16/BF16, this function returns an error - caller must use ReadTensorData and convert manually.
func (r *SafeTensorsReader) LoadTensor(name string, backend tensor.Backend) (*tensor.RawTensor, error) {
	info, err := r.TensorInfo(name)
	if err != nil {
		return nil, err
	}

	// Convert dtype
	dtype, err := safeTensorsDTypeToDataType(info.DType)
	if err != nil {
		return nil, fmt.Errorf("failed to convert dtype for tensor %s: %w", name, err)
	}

	// Convert shape
	shape := tensor.Shape(info.Shape)
	if err := shape.Validate(); err != nil {
		return nil, fmt.Errorf("invalid shape for tensor %s: %w", name, err)
	}

	// Read raw data
	data, err := r.ReadTensorData(name)
	if err != nil {
		return nil, err
	}

	// Create RawTensor
	raw, err := tensor.NewRaw(shape, dtype, backend.Device())
	if err != nil {
		return nil, fmt.Errorf("failed to create tensor: %w", err)
	}

	// Copy data
	copy(raw.Data(), data)

	return raw, nil
}
