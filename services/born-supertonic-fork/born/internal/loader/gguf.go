package loader

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// GGUF format (v3):
// [4 bytes: "GGUF" magic]
// [4 bytes: version (3)]
// [8 bytes: tensor_count]
// [8 bytes: metadata_kv_count]
// [metadata key-value pairs]
// [tensor infos]
// [alignment padding]
// [tensor data (32-byte aligned)]

const (
	ggufMagic     = 0x46554747 // "GGUF" in little-endian
	ggufVersion3  = 3
	ggufAlignment = 32 // GGUF uses 32-byte alignment for tensor data
)

// GGUFType represents GGUF value types.
type GGUFType uint32

// GGUF value types.
const (
	GGUFTypeUint8   GGUFType = 0
	GGUFTypeInt8    GGUFType = 1
	GGUFTypeUint16  GGUFType = 2
	GGUFTypeInt16   GGUFType = 3
	GGUFTypeUint32  GGUFType = 4
	GGUFTypeInt32   GGUFType = 5
	GGUFTypeFloat32 GGUFType = 6
	GGUFTypeBool    GGUFType = 7
	GGUFTypeString  GGUFType = 8
	GGUFTypeArray   GGUFType = 9
	GGUFTypeUint64  GGUFType = 10
	GGUFTypeInt64   GGUFType = 11
	GGUFTypeFloat64 GGUFType = 12
)

// GGUFDType represents GGUF tensor data types.
type GGUFDType uint32

// GGUF tensor dtypes.
const (
	GGUFDTypeF32  GGUFDType = 0
	GGUFDTypeF16  GGUFDType = 1
	GGUFDTypeQ4_0 GGUFDType = 2
	GGUFDTypeQ4_1 GGUFDType = 3
	GGUFDTypeQ8_0 GGUFDType = 8
)

// GGUFMetadata stores GGUF metadata key-value pairs.
type GGUFMetadata map[string]interface{}

// GGUFTensorInfo describes a tensor in GGUF format.
type GGUFTensorInfo struct {
	Name   string
	Dims   []uint64 // Dimensions (reversed compared to normal order)
	DType  GGUFDType
	Offset uint64 // Offset in data section
}

// GGUFReader reads GGUF format files.
type GGUFReader struct {
	file       *os.File
	version    uint32
	metadata   GGUFMetadata
	tensors    map[string]GGUFTensorInfo
	dataOffset uint64 // Offset where tensor data starts
}

// NewGGUFReader creates a new GGUF reader.
func NewGGUFReader(path string) (*GGUFReader, error) {
	//nolint:gosec // G304: File path comes from user input, which is expected for model loading
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	reader := &GGUFReader{
		file:     file,
		metadata: make(GGUFMetadata),
		tensors:  make(map[string]GGUFTensorInfo),
	}

	if err := reader.parseHeader(); err != nil {
		_ = file.Close() // Best effort close on error
		return nil, fmt.Errorf("failed to parse header: %w", err)
	}

	return reader, nil
}

// parseHeader parses the GGUF header.
func (r *GGUFReader) parseHeader() error {
	// Read magic
	var magic uint32
	if err := binary.Read(r.file, binary.LittleEndian, &magic); err != nil {
		return fmt.Errorf("failed to read magic: %w", err)
	}
	if magic != ggufMagic {
		return fmt.Errorf("invalid GGUF magic: 0x%X (expected 0x%X)", magic, ggufMagic)
	}

	// Read version
	if err := binary.Read(r.file, binary.LittleEndian, &r.version); err != nil {
		return fmt.Errorf("failed to read version: %w", err)
	}
	if r.version != ggufVersion3 {
		return fmt.Errorf("unsupported GGUF version: %d (only v3 supported)", r.version)
	}

	// Read tensor count
	var tensorCount uint64
	if err := binary.Read(r.file, binary.LittleEndian, &tensorCount); err != nil {
		return fmt.Errorf("failed to read tensor count: %w", err)
	}

	// Read metadata count
	var metadataCount uint64
	if err := binary.Read(r.file, binary.LittleEndian, &metadataCount); err != nil {
		return fmt.Errorf("failed to read metadata count: %w", err)
	}

	// Read metadata key-value pairs
	for i := uint64(0); i < metadataCount; i++ {
		key, value, err := r.readMetadataKV()
		if err != nil {
			return fmt.Errorf("failed to read metadata[%d]: %w", i, err)
		}
		r.metadata[key] = value
	}

	// Read tensor infos
	for i := uint64(0); i < tensorCount; i++ {
		info, err := r.readTensorInfo()
		if err != nil {
			return fmt.Errorf("failed to read tensor info[%d]: %w", i, err)
		}
		r.tensors[info.Name] = info
	}

	// Calculate data offset (aligned to 32 bytes)
	currentPos, err := r.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("failed to get current position: %w", err)
	}
	r.dataOffset = alignOffset(uint64(currentPos), ggufAlignment) //nolint:gosec // G115: integer overflow conversion int64 -> uint64

	return nil
}

// readString reads a GGUF string (uint64 length + UTF-8 bytes).
func (r *GGUFReader) readString() (string, error) {
	var length uint64
	if err := binary.Read(r.file, binary.LittleEndian, &length); err != nil {
		return "", err
	}
	if length > 1024*1024 { // Sanity check: max 1MB string
		return "", fmt.Errorf("string length too large: %d", length)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r.file, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

// readMetadataKV reads a single metadata key-value pair.
func (r *GGUFReader) readMetadataKV() (string, interface{}, error) {
	// Read key
	key, err := r.readString()
	if err != nil {
		return "", nil, fmt.Errorf("failed to read key: %w", err)
	}

	// Read value type
	var valueType GGUFType
	if err := binary.Read(r.file, binary.LittleEndian, &valueType); err != nil {
		return "", nil, fmt.Errorf("failed to read value type: %w", err)
	}

	// Read value based on type
	value, err := r.readMetadataValue(valueType)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read value: %w", err)
	}

	return key, value, nil
}

// readMetadataValue reads a metadata value based on its type.
func (r *GGUFReader) readMetadataValue(valueType GGUFType) (interface{}, error) {
	switch valueType {
	case GGUFTypeUint8:
		var v uint8
		err := binary.Read(r.file, binary.LittleEndian, &v)
		return v, err
	case GGUFTypeInt8:
		var v int8
		err := binary.Read(r.file, binary.LittleEndian, &v)
		return v, err
	case GGUFTypeUint16:
		var v uint16
		err := binary.Read(r.file, binary.LittleEndian, &v)
		return v, err
	case GGUFTypeInt16:
		var v int16
		err := binary.Read(r.file, binary.LittleEndian, &v)
		return v, err
	case GGUFTypeUint32:
		var v uint32
		err := binary.Read(r.file, binary.LittleEndian, &v)
		return v, err
	case GGUFTypeInt32:
		var v int32
		err := binary.Read(r.file, binary.LittleEndian, &v)
		return v, err
	case GGUFTypeFloat32:
		var v float32
		err := binary.Read(r.file, binary.LittleEndian, &v)
		return v, err
	case GGUFTypeBool:
		var v bool
		err := binary.Read(r.file, binary.LittleEndian, &v)
		return v, err
	case GGUFTypeString:
		return r.readString()
	case GGUFTypeUint64:
		var v uint64
		err := binary.Read(r.file, binary.LittleEndian, &v)
		return v, err
	case GGUFTypeInt64:
		var v int64
		err := binary.Read(r.file, binary.LittleEndian, &v)
		return v, err
	case GGUFTypeFloat64:
		var v float64
		err := binary.Read(r.file, binary.LittleEndian, &v)
		return v, err
	case GGUFTypeArray:
		// For simplicity, skip arrays for now
		return nil, fmt.Errorf("array type not yet supported")
	default:
		return nil, fmt.Errorf("unknown value type: %d", valueType)
	}
}

// readTensorInfo reads a single tensor info.
func (r *GGUFReader) readTensorInfo() (GGUFTensorInfo, error) {
	var info GGUFTensorInfo

	// Read name
	name, err := r.readString()
	if err != nil {
		return info, fmt.Errorf("failed to read tensor name: %w", err)
	}
	info.Name = name

	// Read number of dimensions
	var nDims uint32
	if err := binary.Read(r.file, binary.LittleEndian, &nDims); err != nil {
		return info, fmt.Errorf("failed to read n_dims: %w", err)
	}

	// Read dimensions
	info.Dims = make([]uint64, nDims)
	for i := uint32(0); i < nDims; i++ {
		if err := binary.Read(r.file, binary.LittleEndian, &info.Dims[i]); err != nil {
			return info, fmt.Errorf("failed to read dim[%d]: %w", i, err)
		}
	}

	// Read dtype
	if err := binary.Read(r.file, binary.LittleEndian, &info.DType); err != nil {
		return info, fmt.Errorf("failed to read dtype: %w", err)
	}

	// Read offset
	if err := binary.Read(r.file, binary.LittleEndian, &info.Offset); err != nil {
		return info, fmt.Errorf("failed to read offset: %w", err)
	}

	return info, nil
}

// Close closes the GGUF file.
func (r *GGUFReader) Close() error {
	if r.file != nil {
		return r.file.Close()
	}
	return nil
}

// Metadata returns the metadata map.
func (r *GGUFReader) Metadata() GGUFMetadata {
	return r.metadata
}

// TensorNames returns a list of all tensor names.
func (r *GGUFReader) TensorNames() []string {
	names := make([]string, 0, len(r.tensors))
	for name := range r.tensors {
		names = append(names, name)
	}
	return names
}

// TensorInfo returns information about a specific tensor.
func (r *GGUFReader) TensorInfo(name string) (*GGUFTensorInfo, error) {
	info, ok := r.tensors[name]
	if !ok {
		return nil, fmt.Errorf("tensor %s not found", name)
	}
	return &info, nil
}

// ReadTensorData reads raw tensor data for a given tensor.
func (r *GGUFReader) ReadTensorData(name string) ([]byte, error) {
	info, err := r.TensorInfo(name)
	if err != nil {
		return nil, err
	}

	// Calculate tensor size
	size := r.calculateTensorSize(info)

	// Seek to tensor data
	offset := int64(r.dataOffset + info.Offset) //nolint:gosec // G115: integer overflow conversion uint64 -> int64
	if _, err := r.file.Seek(offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek to tensor data: %w", err)
	}

	// Read data
	data := make([]byte, size)
	if _, err := io.ReadFull(r.file, data); err != nil {
		return nil, fmt.Errorf("failed to read tensor data: %w", err)
	}

	return data, nil
}

// calculateTensorSize calculates the byte size of a tensor.
func (r *GGUFReader) calculateTensorSize(info *GGUFTensorInfo) uint64 {
	// Calculate number of elements
	numElements := uint64(1)
	for _, dim := range info.Dims {
		numElements *= dim
	}

	// Calculate bytes per element based on dtype
	var bytesPerElement uint64
	switch info.DType {
	case GGUFDTypeF32:
		bytesPerElement = 4
	case GGUFDTypeF16:
		bytesPerElement = 2
	case GGUFDTypeQ4_0:
		// Q4_0: 32 values per block, 2 bytes (fp16 scale) + 16 bytes (4-bit values) = 18 bytes per block
		blocks := (numElements + 31) / 32
		return blocks * 18
	case GGUFDTypeQ4_1:
		// Q4_1: Similar to Q4_0 but with offset
		blocks := (numElements + 31) / 32
		return blocks * 20
	case GGUFDTypeQ8_0:
		// Q8_0: 32 values per block, 2 bytes (fp16 scale) + 32 bytes (8-bit values) = 34 bytes per block
		blocks := (numElements + 31) / 32
		return blocks * 34
	default:
		bytesPerElement = 4 // Default to float32
	}

	return numElements * bytesPerElement
}

// alignOffset aligns an offset to the specified alignment.
func alignOffset(offset, alignment uint64) uint64 {
	if offset%alignment == 0 {
		return offset
	}
	return offset + (alignment - offset%alignment)
}
