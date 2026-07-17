package serialization

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/born-ml/born/internal/tensor"
)

// BornReader reads models from .born format.
type BornReader struct {
	reader     io.ReadSeeker
	closer     io.Closer
	header     Header
	flags      uint32
	version    uint32
	dataOffset int64    // Offset where tensor data starts
	dataSize   int64    // Size of the data section
	checksum   [32]byte // SHA-256 checksum (v2 only)
	opts       ReaderOptions
	closed     bool
}

// ReaderOptions configures the behavior of BornReader.
type ReaderOptions struct {
	SkipChecksumValidation bool            // Skip checksum validation (faster but less safe)
	ValidationLevel        ValidationLevel // Validation strictness level
}

// NewBornReader creates a new .born file reader with default options (strict validation).
func NewBornReader(path string) (*BornReader, error) {
	return NewBornReaderWithOptions(path, ReaderOptions{
		ValidationLevel: ValidationStrict,
	})
}

// NewBornReaderWithOptions creates a new .born file reader with custom options.
func NewBornReaderWithOptions(path string, opts ReaderOptions) (*BornReader, error) {
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

	reader, err := newBornReaderFromReader(file, stat.Size(), opts)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	reader.closer = file
	return reader, nil
}

// newBornReaderFromReader creates a new .born reader from any io.ReadSeeker.
//
// The caller must provide the total readable size in bytes. If a closer is set
// on the returned BornReader (via the closer field), calling BornReader.Close()
// will close the underlying resource. Otherwise, the caller is responsible for
// managing the reader's lifecycle.
func newBornReaderFromReader(r io.ReadSeeker, size int64, opts ReaderOptions) (*BornReader, error) {
	reader := &BornReader{
		reader: r,
		opts:   opts,
	}

	if err := reader.parseHeader(); err != nil {
		return nil, fmt.Errorf("failed to parse header: %w", err)
	}

	// Calculate data section size
	reader.dataSize = size - reader.dataOffset

	// Validate header if requested
	if err := ValidateHeader(&reader.header, reader.dataSize, opts.ValidationLevel); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return reader, nil
}

// NewBornReaderFromBytes creates a new .born reader from a byte slice.
func NewBornReaderFromBytes(data []byte) (*BornReader, error) {
	return NewBornReaderFromBytesWithOptions(data, ReaderOptions{
		ValidationLevel: ValidationStrict,
	})
}

// NewBornReaderFromBytesWithOptions creates a new .born reader from a byte slice
// with custom options.
func NewBornReaderFromBytesWithOptions(data []byte, opts ReaderOptions) (*BornReader, error) {
	return newBornReaderFromReader(bytes.NewReader(data), int64(len(data)), opts)
}

// NewBornReaderFromReadSeeker creates a new .born reader from any io.ReadSeeker.
//
// The readable size is derived from the stream by seeking to its end. Validation
// defaults to ValidationStrict; use NewBornReaderFromReadSeekerWithOptions to
// override. The reader holds no closer, so the caller owns the stream lifecycle.
func NewBornReaderFromReadSeeker(r io.ReadSeeker) (*BornReader, error) {
	return NewBornReaderFromReadSeekerWithOptions(r, ReaderOptions{
		ValidationLevel: ValidationStrict,
	})
}

// NewBornReaderFromReadSeekerWithOptions creates a new .born reader from any
// io.ReadSeeker with custom options, deriving the readable size from the stream.
func NewBornReaderFromReadSeekerWithOptions(r io.ReadSeeker, opts ReaderOptions) (*BornReader, error) {
	size, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, fmt.Errorf("failed to determine size: %w", err)
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek to start: %w", err)
	}
	return newBornReaderFromReader(r, size, opts)
}

// parseHeader reads and parses the .born file header.
func (r *BornReader) parseHeader() error {
	// Read magic bytes
	magic := make([]byte, 4)
	if _, err := io.ReadFull(r.reader, magic); err != nil {
		return fmt.Errorf("failed to read magic bytes: %w", err)
	}
	if string(magic) != MagicBytes {
		return ErrInvalidMagic
	}

	// Read version
	if err := binary.Read(r.reader, binary.LittleEndian, &r.version); err != nil {
		return fmt.Errorf("failed to read version: %w", err)
	}

	// Handle different format versions
	switch r.version {
	case FormatVersion: // v1
		return r.parseHeaderV1()
	case FormatVersionV2: // v2
		return r.parseHeaderV2()
	default:
		return fmt.Errorf("%w: got %d, expected %d or %d", ErrUnsupportedVersion, r.version, FormatVersion, FormatVersionV2)
	}
}

// parseHeaderV1 parses v1 format header (no checksum).
func (r *BornReader) parseHeaderV1() error {
	// Read flags
	if err := binary.Read(r.reader, binary.LittleEndian, &r.flags); err != nil {
		return fmt.Errorf("failed to read flags: %w", err)
	}

	// Read header size
	var headerSize uint64
	if err := binary.Read(r.reader, binary.LittleEndian, &headerSize); err != nil {
		return fmt.Errorf("failed to read header size: %w", err)
	}

	// Validate header size
	if headerSize > 100*1024*1024 {
		return ErrHeaderTooLarge
	}

	// Read header JSON
	headerBytes := make([]byte, headerSize)
	if _, err := io.ReadFull(r.reader, headerBytes); err != nil {
		return fmt.Errorf("failed to read header: %w", err)
	}

	// Parse header
	if err := json.Unmarshal(headerBytes, &r.header); err != nil {
		return fmt.Errorf("failed to parse header JSON: %w", err)
	}

	// Calculate data offset (with alignment padding)
	currentPos := int64(4+4+4+8) + int64(headerSize) // magic + version + flags + headerSize + header
	padding := (HeaderAlignment - (currentPos % HeaderAlignment)) % HeaderAlignment
	r.dataOffset = currentPos + padding

	return nil
}

// parseHeaderV2 parses v2 format header (with checksum).
func (r *BornReader) parseHeaderV2() error {
	// Seek back to start to read fixed header
	if _, err := r.reader.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to start: %w", err)
	}

	// Read entire fixed header (64 bytes)
	fixedHeader := make([]byte, FixedHeaderSizeV2)
	if _, err := io.ReadFull(r.reader, fixedHeader); err != nil {
		return fmt.Errorf("failed to read fixed header: %w", err)
	}

	// Parse fixed header fields
	// 0x04-0x07: version (already read, but verify)
	version := binary.LittleEndian.Uint32(fixedHeader[4:8])
	if version != FormatVersionV2 {
		return fmt.Errorf("version mismatch in fixed header: got %d, expected %d", version, FormatVersionV2)
	}

	// 0x08-0x0B: flags
	r.flags = binary.LittleEndian.Uint32(fixedHeader[8:12])

	// 0x10-0x17: header size
	headerSize := binary.LittleEndian.Uint64(fixedHeader[16:24])

	// 0x18-0x1F: data size
	dataSize := binary.LittleEndian.Uint64(fixedHeader[24:32])

	// 0x20-0x3F: SHA-256 checksum
	copy(r.checksum[:], fixedHeader[ChecksumOffsetV2:ChecksumOffsetV2+ChecksumSize])

	// Validate header size
	if headerSize > 100*1024*1024 {
		return ErrHeaderTooLarge
	}

	// Read header JSON (already positioned at offset 0x40)
	headerBytes := make([]byte, headerSize)
	if _, err := io.ReadFull(r.reader, headerBytes); err != nil {
		return fmt.Errorf("failed to read header JSON: %w", err)
	}

	// Parse header
	if err := json.Unmarshal(headerBytes, &r.header); err != nil {
		return fmt.Errorf("failed to parse header JSON: %w", err)
	}

	// Calculate data offset (with alignment padding)
	currentPos := int64(FixedHeaderSizeV2) + int64(headerSize)
	padding := (HeaderAlignment - (currentPos % HeaderAlignment)) % HeaderAlignment
	r.dataOffset = currentPos + padding

	// Validate checksum if not skipped
	if !r.opts.SkipChecksumValidation {
		// Read all tensor data
		tensorData := make([]byte, dataSize)
		if _, err := r.reader.Seek(r.dataOffset, io.SeekStart); err != nil {
			return fmt.Errorf("failed to seek to tensor data: %w", err)
		}
		if _, err := io.ReadFull(r.reader, tensorData); err != nil {
			return fmt.Errorf("failed to read tensor data for checksum: %w", err)
		}

		// Compute and validate checksum
		computed := ComputeChecksum(tensorData)
		if err := ValidateChecksum(computed, r.checksum); err != nil {
			return err
		}
	}

	return nil
}

// Header returns the file header.
func (r *BornReader) Header() Header {
	return r.header
}

// Metadata returns the metadata map from the header.
func (r *BornReader) Metadata() map[string]string {
	return r.header.Metadata
}

// TensorNames returns a list of all tensor names in the file.
func (r *BornReader) TensorNames() []string {
	names := make([]string, len(r.header.Tensors))
	for i, meta := range r.header.Tensors {
		names[i] = meta.Name
	}
	return names
}

// TensorInfo returns information about a specific tensor.
func (r *BornReader) TensorInfo(name string) (*TensorMeta, error) {
	for _, meta := range r.header.Tensors {
		if meta.Name == name {
			return &meta, nil
		}
	}
	return nil, fmt.Errorf("tensor %s not found", name)
}

// ReadTensorData reads raw tensor data for a given tensor name.
func (r *BornReader) ReadTensorData(name string) ([]byte, error) {
	if r.closed {
		return nil, fmt.Errorf("reader is closed")
	}

	meta, err := r.TensorInfo(name)
	if err != nil {
		return nil, err
	}

	// Calculate absolute offset
	absoluteOffset := r.dataOffset + meta.Offset

	// Seek to tensor data
	if _, err := r.reader.Seek(absoluteOffset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek to tensor data: %w", err)
	}

	// Read data
	data := make([]byte, meta.Size)
	if _, err := io.ReadFull(r.reader, data); err != nil {
		return nil, fmt.Errorf("failed to read tensor data: %w", err)
	}

	return data, nil
}

// LoadTensor loads a single tensor from the file.
func (r *BornReader) LoadTensor(name string, backend tensor.Backend) (*tensor.RawTensor, error) {
	if r.closed {
		return nil, fmt.Errorf("reader is closed")
	}

	meta, err := r.TensorInfo(name)
	if err != nil {
		return nil, err
	}

	// Convert dtype
	dtype, ok := stringToDtype(meta.DType)
	if !ok {
		return nil, fmt.Errorf("unsupported dtype: %s", meta.DType)
	}

	// Convert shape
	shape := tensor.Shape(meta.Shape)
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

// ReadStateDict reads all tensors into a state dictionary.
func (r *BornReader) ReadStateDict(backend tensor.Backend) (map[string]*tensor.RawTensor, error) {
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

// Close closes the reader and its underlying resource (if any).
func (r *BornReader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	if r.closer != nil {
		return r.closer.Close()
	}
	return nil
}

// ReadFrom reads a state dictionary from an io.Reader.
// This is useful for reading from buffers or network connections.
//
//nolint:gocognit,gocyclo,cyclop // Complex reader logic is unavoidable
func ReadFrom(reader io.Reader, backend tensor.Backend) (map[string]*tensor.RawTensor, Header, error) {
	// Read magic bytes
	magic := make([]byte, 4)
	if _, err := io.ReadFull(reader, magic); err != nil {
		return nil, Header{}, fmt.Errorf("failed to read magic bytes: %w", err)
	}
	if string(magic) != MagicBytes {
		return nil, Header{}, fmt.Errorf("invalid magic bytes: got %q, expected %q", string(magic), MagicBytes)
	}

	// Read version
	var version uint32
	if err := binary.Read(reader, binary.LittleEndian, &version); err != nil {
		return nil, Header{}, fmt.Errorf("failed to read version: %w", err)
	}
	if version != FormatVersion {
		return nil, Header{}, fmt.Errorf("unsupported format version: got %d, expected %d", version, FormatVersion)
	}

	// Read flags
	var flags uint32
	if err := binary.Read(reader, binary.LittleEndian, &flags); err != nil {
		return nil, Header{}, fmt.Errorf("failed to read flags: %w", err)
	}

	// Read header size
	var headerSize uint64
	if err := binary.Read(reader, binary.LittleEndian, &headerSize); err != nil {
		return nil, Header{}, fmt.Errorf("failed to read header size: %w", err)
	}

	// Validate header size
	if headerSize > 100*1024*1024 {
		return nil, Header{}, fmt.Errorf("invalid header size: %d (too large)", headerSize)
	}

	// Read header JSON
	headerBytes := make([]byte, headerSize)
	if _, err := io.ReadFull(reader, headerBytes); err != nil {
		return nil, Header{}, fmt.Errorf("failed to read header: %w", err)
	}

	// Parse header
	var header Header
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, Header{}, fmt.Errorf("failed to parse header JSON: %w", err)
	}

	// Calculate and skip padding
	currentPos := int64(4+4+4+8) + int64(headerSize)
	padding := (HeaderAlignment - (currentPos % HeaderAlignment)) % HeaderAlignment
	if padding > 0 {
		paddingBytes := make([]byte, padding)
		if _, err := io.ReadFull(reader, paddingBytes); err != nil {
			return nil, Header{}, fmt.Errorf("failed to read padding: %w", err)
		}
	}

	// Read all tensors
	stateDict := make(map[string]*tensor.RawTensor)
	for _, meta := range header.Tensors {
		// Convert dtype
		dtype, ok := stringToDtype(meta.DType)
		if !ok {
			return nil, Header{}, fmt.Errorf("unsupported dtype: %s", meta.DType)
		}

		// Convert shape
		shape := tensor.Shape(meta.Shape)
		if err := shape.Validate(); err != nil {
			return nil, Header{}, fmt.Errorf("invalid shape for tensor %s: %w", meta.Name, err)
		}

		// Read tensor data
		data := make([]byte, meta.Size)
		if _, err := io.ReadFull(reader, data); err != nil {
			return nil, Header{}, fmt.Errorf("failed to read tensor %s: %w", meta.Name, err)
		}

		// Create RawTensor
		raw, err := tensor.NewRaw(shape, dtype, backend.Device())
		if err != nil {
			return nil, Header{}, fmt.Errorf("failed to create tensor: %w", err)
		}

		// Copy data
		copy(raw.Data(), data)

		stateDict[meta.Name] = raw
	}

	return stateDict, header, nil
}
