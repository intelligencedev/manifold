package serialization

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"

	"github.com/born-ml/born/internal/tensor"
)

// MmapReader provides memory-mapped access to .born files.
// This enables efficient loading of large models by only reading the header initially,
// and accessing tensor data on-demand via OS page cache.
type MmapReader struct {
	file       *os.File
	data       []byte // mmap'd region (read-only)
	size       int64
	header     Header
	version    uint32
	flags      uint32
	dataOffset int64
	dataSize   int64
	checksum   [32]byte
	closed     bool
}

// NewMmapReader creates a memory-mapped reader for a .born file.
// The file is opened read-only and mapped into memory.
// Only the header is parsed initially - tensor data is accessed on-demand.
//
// Important: Always call Close() when done to unmap the file (use defer).
func NewMmapReader(path string) (*MmapReader, error) {
	//nolint:gosec // G304: File path comes from user input, which is expected for model loading
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	stat, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Memory map the file (platform-specific implementation)
	data, err := mmapFile(file, stat.Size())
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("mmap failed: %w", err)
	}

	r := &MmapReader{
		file: file,
		data: data,
		size: stat.Size(),
	}

	if err := r.parseHeader(); err != nil {
		_ = r.Close()
		return nil, fmt.Errorf("failed to parse header: %w", err)
	}

	return r, nil
}

// parseHeader reads and parses the .born file header from mmap'd region.
func (r *MmapReader) parseHeader() error {
	if r.size < 20 {
		return fmt.Errorf("file too small: %d bytes (minimum 20 bytes required)", r.size)
	}

	// Read magic bytes
	if string(r.data[0:4]) != MagicBytes {
		return ErrInvalidMagic
	}

	// Read version
	r.version = binary.LittleEndian.Uint32(r.data[4:8])
	if r.version != FormatVersion && r.version != FormatVersionV2 {
		return fmt.Errorf("%w: got %d, expected %d or %d", ErrUnsupportedVersion, r.version, FormatVersion, FormatVersionV2)
	}

	// Read flags
	r.flags = binary.LittleEndian.Uint32(r.data[8:12])

	var headerSize uint64
	var jsonOffset int64

	if r.version == FormatVersionV2 {
		// v2: Fixed 64-byte header
		if r.size < FixedHeaderSizeV2 {
			return fmt.Errorf("file too small for v2: %d bytes (minimum 64 bytes required)", r.size)
		}

		// Read header size (offset 0x10)
		headerSize = binary.LittleEndian.Uint64(r.data[16:24])

		// Read data size (offset 0x18)
		dataSize64 := binary.LittleEndian.Uint64(r.data[24:32])
		if dataSize64 > 0x7FFFFFFFFFFFFFFF {
			return fmt.Errorf("data size too large: %d", dataSize64)
		}
		r.dataSize = int64(dataSize64)

		// Read checksum (offset 0x20)
		copy(r.checksum[:], r.data[ChecksumOffsetV2:ChecksumOffsetV2+ChecksumSize])

		jsonOffset = FixedHeaderSizeV2
	} else {
		// v1: Variable header (20 bytes + JSON)
		// Read header size (offset 0x0C)
		headerSize = binary.LittleEndian.Uint64(r.data[12:20])
		jsonOffset = 20
	}

	// Validate header size
	if headerSize > MaxHeaderSize {
		return ErrHeaderTooLarge
	}

	// Calculate header end position
	headerEnd := jsonOffset + int64(headerSize)
	if headerEnd > r.size {
		return fmt.Errorf("header extends beyond file: header_end=%d, file_size=%d", headerEnd, r.size)
	}

	// Parse JSON header
	if err := json.Unmarshal(r.data[jsonOffset:headerEnd], &r.header); err != nil {
		return fmt.Errorf("failed to parse header JSON: %w", err)
	}

	// Calculate data offset with 64-byte alignment
	r.dataOffset = ((headerEnd + 63) / 64) * 64

	// For v1, calculate data size from file size
	if r.version == FormatVersion {
		r.dataSize = r.size - r.dataOffset
	}

	// Validate header
	if err := ValidateHeader(&r.header, r.dataSize, ValidationStrict); err != nil {
		return fmt.Errorf("header validation failed: %w", err)
	}

	return nil
}

// Close unmaps and closes the file.
func (r *MmapReader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true

	var err error
	if r.data != nil {
		err = munmapFile(r.data)
		r.data = nil
	}

	if closeErr := r.file.Close(); closeErr != nil && err == nil {
		err = closeErr
	}

	return err
}

// Header returns the file header.
func (r *MmapReader) Header() Header {
	return r.header
}

// Version returns the format version (1 or 2).
func (r *MmapReader) Version() uint32 {
	return r.version
}

// Flags returns the flags bitfield.
func (r *MmapReader) Flags() uint32 {
	return r.flags
}

// Checksum returns the SHA-256 checksum (v2 only, all zeros for v1).
func (r *MmapReader) Checksum() [32]byte {
	return r.checksum
}

// TensorNames returns a list of all tensor names in the file.
func (r *MmapReader) TensorNames() []string {
	names := make([]string, len(r.header.Tensors))
	for i, t := range r.header.Tensors {
		names[i] = t.Name
	}
	return names
}

// TensorInfo returns metadata about a specific tensor.
func (r *MmapReader) TensorInfo(name string) (*TensorMeta, error) {
	for i := range r.header.Tensors {
		if r.header.Tensors[i].Name == name {
			return &r.header.Tensors[i], nil
		}
	}
	return nil, fmt.Errorf("tensor %q not found", name)
}

// TensorData returns a zero-copy slice to tensor data.
// The returned slice is valid only while the reader is open.
// WARNING: The data is read-only - writing to it will cause undefined behavior.
//
// For cases where you need to modify the data, use TensorDataCopy instead.
func (r *MmapReader) TensorData(name string) ([]byte, error) {
	if r.closed {
		return nil, fmt.Errorf("reader is closed")
	}

	meta, err := r.TensorInfo(name)
	if err != nil {
		return nil, err
	}

	start := r.dataOffset + meta.Offset
	end := start + meta.Size

	if end > r.size {
		return nil, fmt.Errorf("%w: tensor %q: offset %d + size %d > file_size %d",
			ErrOutOfBounds, name, start, meta.Size, r.size)
	}

	// Zero-copy: return slice into mmap'd region
	return r.data[start:end], nil
}

// TensorDataCopy returns a copy of tensor data (for modification).
// This allocates a new buffer and copies the data.
// Use this when you need to modify the tensor data.
func (r *MmapReader) TensorDataCopy(name string) ([]byte, error) {
	data, err := r.TensorData(name)
	if err != nil {
		return nil, err
	}

	result := make([]byte, len(data))
	copy(result, data)
	return result, nil
}

// LoadTensor loads a tensor using the mmap'd data.
// This is a convenience method that creates a RawTensor and copies data into it.
func (r *MmapReader) LoadTensor(name string, backend tensor.Backend) (*tensor.RawTensor, error) {
	if r.closed {
		return nil, fmt.Errorf("reader is closed")
	}

	meta, err := r.TensorInfo(name)
	if err != nil {
		return nil, err
	}

	dtype, ok := stringToDtype(meta.DType)
	if !ok {
		return nil, fmt.Errorf("unsupported dtype: %s", meta.DType)
	}

	shape := tensor.Shape(meta.Shape)
	if err := shape.Validate(); err != nil {
		return nil, fmt.Errorf("invalid shape for tensor %s: %w", name, err)
	}

	raw, err := tensor.NewRaw(shape, dtype, backend.Device())
	if err != nil {
		return nil, fmt.Errorf("failed to create tensor: %w", err)
	}

	// Get zero-copy data and copy to tensor
	data, err := r.TensorData(name)
	if err != nil {
		return nil, err
	}
	copy(raw.Data(), data)

	return raw, nil
}

// ReadStateDict reads all tensors into a state dictionary.
func (r *MmapReader) ReadStateDict(backend tensor.Backend) (map[string]*tensor.RawTensor, error) {
	if r.closed {
		return nil, fmt.Errorf("reader is closed")
	}

	stateDict := make(map[string]*tensor.RawTensor)

	for _, meta := range r.header.Tensors {
		raw, err := r.LoadTensor(meta.Name, backend)
		if err != nil {
			return nil, fmt.Errorf("failed to load tensor %s: %w", meta.Name, err)
		}
		stateDict[meta.Name] = raw
	}

	return stateDict, nil
}
