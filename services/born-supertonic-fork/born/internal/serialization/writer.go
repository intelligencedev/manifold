package serialization

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/born-ml/born/internal/tensor"
)

const bornVersion = "0.5.4" // Current Born version

// BornWriter writes models in .born format.
type BornWriter struct {
	w      io.Writer
	closer io.Closer
	closed bool
}

// NewBornWriter creates a new .born file writer.
func NewBornWriter(path string) (*BornWriter, error) {
	file, err := os.Create(path) //nolint:gosec // G304: Path comes from trusted caller
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	writer := newBornWriterToWriter(file)
	writer.closer = file
	return writer, nil
}

// newBornWriterToWriter creates a new .born writer over any io.Writer.
//
// The returned BornWriter has no closer, so Close() is a no-op unless one is
// set on the closer field. Otherwise, the caller is responsible for managing
// the underlying writer's lifecycle.
func newBornWriterToWriter(w io.Writer) *BornWriter {
	return &BornWriter{w: w}
}

// WriteStateDictWithHeader writes a state dictionary with custom header to the .born file.
//
// This allows setting CheckpointMeta and other custom header fields.
//
//nolint:gocyclo,cyclop // Complex writer logic is unavoidable for binary format
func (w *BornWriter) WriteStateDictWithHeader(stateDict map[string]*tensor.RawTensor, header Header) error {
	if w.closed {
		return fmt.Errorf("writer is closed")
	}

	// Calculate tensor offsets
	var currentOffset int64
	tensorOrder := make([]string, 0, len(stateDict))

	header.Tensors = make([]TensorMeta, 0, len(stateDict))
	for name, raw := range stateDict {
		tensorOrder = append(tensorOrder, name)
		shape := raw.Shape()
		size := int64(raw.NumElements() * raw.DType().Size())

		header.Tensors = append(header.Tensors, TensorMeta{
			Name:   name,
			DType:  dtypeToString(raw.DType()),
			Shape:  []int(shape),
			Offset: currentOffset,
			Size:   size,
		})

		currentOffset += size
	}

	// Ensure metadata map exists
	if header.Metadata == nil {
		header.Metadata = make(map[string]string)
	}

	// Marshal header to JSON
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("failed to marshal header: %w", err)
	}

	// Write magic bytes
	if _, err := io.WriteString(w.w, MagicBytes); err != nil {
		return fmt.Errorf("failed to write magic bytes: %w", err)
	}

	// Write version
	if err := binary.Write(w.w, binary.LittleEndian, uint32(FormatVersion)); err != nil {
		return fmt.Errorf("failed to write version: %w", err)
	}

	// Write flags
	flags := uint32(0)
	if len(header.Metadata) > 0 {
		flags |= FlagHasMetadata
	}
	if header.CheckpointMeta != nil && header.CheckpointMeta.IsCheckpoint {
		flags |= FlagHasOptimizer
	}
	if err := binary.Write(w.w, binary.LittleEndian, flags); err != nil {
		return fmt.Errorf("failed to write flags: %w", err)
	}

	// Write header size
	headerSize := uint64(len(headerJSON))
	if err := binary.Write(w.w, binary.LittleEndian, headerSize); err != nil {
		return fmt.Errorf("failed to write header size: %w", err)
	}

	// Write header JSON
	if _, err := w.w.Write(headerJSON); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Calculate padding
	currentPos := int64(4+4+4+8) + int64(headerSize) //nolint:gosec // G115: integer overflow conversion uint64 -> int64
	padding := (HeaderAlignment - (currentPos % HeaderAlignment)) % HeaderAlignment
	if padding > 0 {
		paddingBytes := make([]byte, padding)
		if _, err := w.w.Write(paddingBytes); err != nil {
			return fmt.Errorf("failed to write padding: %w", err)
		}
	}

	// Write tensor data
	for _, name := range tensorOrder {
		raw := stateDict[name]
		data := raw.Data()
		if _, err := w.w.Write(data); err != nil {
			return fmt.Errorf("failed to write tensor %s: %w", name, err)
		}
	}

	return nil
}

// WriteStateDict writes a state dictionary to the .born file.
//
// The state dictionary is a map from parameter names to tensors.
// All tensors must be on the same device.
func (w *BornWriter) WriteStateDict(stateDict map[string]*tensor.RawTensor, modelType string, metadata map[string]string) error {
	if w.closed {
		return fmt.Errorf("writer is closed")
	}

	// Build header
	header := Header{
		FormatVersion: FormatVersion,
		BornVersion:   bornVersion,
		ModelType:     modelType,
		CreatedAt:     time.Now().UTC(),
		Tensors:       make([]TensorMeta, 0, len(stateDict)),
		Metadata:      metadata,
	}

	if header.Metadata == nil {
		header.Metadata = make(map[string]string)
	}

	// Calculate tensor offsets
	var currentOffset int64
	tensorOrder := make([]string, 0, len(stateDict))

	for name, raw := range stateDict {
		tensorOrder = append(tensorOrder, name)
		shape := raw.Shape()
		size := int64(raw.NumElements() * raw.DType().Size())

		header.Tensors = append(header.Tensors, TensorMeta{
			Name:   name,
			DType:  dtypeToString(raw.DType()),
			Shape:  []int(shape),
			Offset: currentOffset,
			Size:   size,
		})

		currentOffset += size
	}

	// Marshal header to JSON
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("failed to marshal header: %w", err)
	}

	// Write magic bytes
	if _, err := io.WriteString(w.w, MagicBytes); err != nil {
		return fmt.Errorf("failed to write magic bytes: %w", err)
	}

	// Write version
	if err := binary.Write(w.w, binary.LittleEndian, uint32(FormatVersion)); err != nil {
		return fmt.Errorf("failed to write version: %w", err)
	}

	// Write flags (currently no flags set)
	flags := uint32(0)
	if len(metadata) > 0 {
		flags |= FlagHasMetadata
	}
	if err := binary.Write(w.w, binary.LittleEndian, flags); err != nil {
		return fmt.Errorf("failed to write flags: %w", err)
	}

	// Write header size
	headerSize := uint64(len(headerJSON))
	if err := binary.Write(w.w, binary.LittleEndian, headerSize); err != nil {
		return fmt.Errorf("failed to write header size: %w", err)
	}

	// Write header JSON
	if _, err := w.w.Write(headerJSON); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Calculate padding to align tensor data to HeaderAlignment

	currentPos := int64(4+4+4+8) + int64(headerSize) //nolint:gosec // G115: integer overflow conversion uint64 -> int64
	padding := (HeaderAlignment - (currentPos % HeaderAlignment)) % HeaderAlignment
	if padding > 0 {
		paddingBytes := make([]byte, padding)
		if _, err := w.w.Write(paddingBytes); err != nil {
			return fmt.Errorf("failed to write padding: %w", err)
		}
	}

	// Write tensor data in order
	for _, name := range tensorOrder {
		raw := stateDict[name]
		data := raw.Data()
		if _, err := w.w.Write(data); err != nil {
			return fmt.Errorf("failed to write tensor %s: %w", name, err)
		}
	}

	return nil
}

// WriteStateDictV2 writes a state dictionary to the .born file using format v2 with SHA-256 checksum.
//
// Format v2 includes:
// - 64-byte fixed header with SHA-256 checksum at offset 0x20
// - Backward compatible: v1 readers will reject, but v2 readers can read v1.
func (w *BornWriter) WriteStateDictV2(stateDict map[string]*tensor.RawTensor, modelType string, metadata map[string]string) error {
	if w.closed {
		return fmt.Errorf("writer is closed")
	}

	// Build header
	header := Header{
		FormatVersion: FormatVersionV2,
		BornVersion:   bornVersion,
		ModelType:     modelType,
		CreatedAt:     time.Now().UTC(),
		Tensors:       make([]TensorMeta, 0, len(stateDict)),
		Metadata:      metadata,
	}

	if header.Metadata == nil {
		header.Metadata = make(map[string]string)
	}

	// Calculate tensor offsets and collect tensor data
	var currentOffset int64
	tensorOrder := make([]string, 0, len(stateDict))
	var tensorDataBuf []byte // Buffer to collect all tensor data for checksum

	for name, raw := range stateDict {
		tensorOrder = append(tensorOrder, name)
		shape := raw.Shape()
		size := int64(raw.NumElements() * raw.DType().Size())

		header.Tensors = append(header.Tensors, TensorMeta{
			Name:   name,
			DType:  dtypeToString(raw.DType()),
			Shape:  []int(shape),
			Offset: currentOffset,
			Size:   size,
		})

		currentOffset += size
	}

	// Collect all tensor data to compute checksum
	for _, name := range tensorOrder {
		raw := stateDict[name]
		tensorDataBuf = append(tensorDataBuf, raw.Data()...)
	}

	// Compute SHA-256 checksum of tensor data
	checksum := ComputeChecksum(tensorDataBuf)

	// Marshal header to JSON
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("failed to marshal header: %w", err)
	}

	// Calculate sizes
	headerSize := uint64(len(headerJSON))
	dataSize := uint64(len(tensorDataBuf))

	// Write v2 fixed header (64 bytes)
	fixedHeader := make([]byte, FixedHeaderSizeV2)

	// 0x00-0x03: Magic bytes "BORN"
	copy(fixedHeader[0:4], MagicBytes)

	// 0x04-0x07: Version (2)
	binary.LittleEndian.PutUint32(fixedHeader[4:8], uint32(FormatVersionV2))

	// 0x08-0x0B: Flags
	flags := uint32(0)
	if len(metadata) > 0 {
		flags |= FlagHasMetadata
	}
	binary.LittleEndian.PutUint32(fixedHeader[8:12], flags)

	// 0x0C-0x0F: Reserved (0)
	// Already zero from make()

	// 0x10-0x17: Header size (8 bytes)
	binary.LittleEndian.PutUint64(fixedHeader[16:24], headerSize)

	// 0x18-0x1F: Data size (8 bytes)
	binary.LittleEndian.PutUint64(fixedHeader[24:32], dataSize)

	// 0x20-0x3F: SHA-256 checksum (32 bytes)
	copy(fixedHeader[ChecksumOffsetV2:ChecksumOffsetV2+ChecksumSize], checksum[:])

	// Write fixed header
	if _, err := w.w.Write(fixedHeader); err != nil {
		return fmt.Errorf("failed to write fixed header: %w", err)
	}

	// Write header JSON
	if _, err := w.w.Write(headerJSON); err != nil {
		return fmt.Errorf("failed to write header JSON: %w", err)
	}

	// Calculate padding to align tensor data to 64-byte boundary

	currentPos := int64(FixedHeaderSizeV2) + int64(headerSize) //nolint:gosec // G115: integer overflow conversion uint64 -> int64
	padding := (HeaderAlignment - (currentPos % HeaderAlignment)) % HeaderAlignment
	if padding > 0 {
		paddingBytes := make([]byte, padding)
		if _, err := w.w.Write(paddingBytes); err != nil {
			return fmt.Errorf("failed to write padding: %w", err)
		}
	}

	// Write tensor data
	if _, err := w.w.Write(tensorDataBuf); err != nil {
		return fmt.Errorf("failed to write tensor data: %w", err)
	}

	return nil
}

// WriteStateDictWithHeaderV2 writes a state dictionary with custom header to the .born file using format v2.
//
// This allows setting CheckpointMeta and other custom header fields.
func (w *BornWriter) WriteStateDictWithHeaderV2(stateDict map[string]*tensor.RawTensor, header Header) error {
	if w.closed {
		return fmt.Errorf("writer is closed")
	}

	// Force v2 format
	header.FormatVersion = FormatVersionV2
	header.BornVersion = bornVersion
	header.CreatedAt = time.Now().UTC()

	// Calculate tensor offsets
	var currentOffset int64
	tensorOrder := make([]string, 0, len(stateDict))

	header.Tensors = make([]TensorMeta, 0, len(stateDict))
	for name, raw := range stateDict {
		tensorOrder = append(tensorOrder, name)
		shape := raw.Shape()
		size := int64(raw.NumElements() * raw.DType().Size())

		header.Tensors = append(header.Tensors, TensorMeta{
			Name:   name,
			DType:  dtypeToString(raw.DType()),
			Shape:  []int(shape),
			Offset: currentOffset,
			Size:   size,
		})

		currentOffset += size
	}

	// Ensure metadata map exists
	if header.Metadata == nil {
		header.Metadata = make(map[string]string)
	}

	// Collect all tensor data to compute checksum
	var tensorDataBuf []byte
	for _, name := range tensorOrder {
		raw := stateDict[name]
		tensorDataBuf = append(tensorDataBuf, raw.Data()...)
	}

	// Compute SHA-256 checksum
	checksum := ComputeChecksum(tensorDataBuf)

	// Marshal header to JSON
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("failed to marshal header: %w", err)
	}

	// Calculate sizes
	headerSize := uint64(len(headerJSON))
	dataSize := uint64(len(tensorDataBuf))

	// Write v2 fixed header (64 bytes)
	fixedHeader := make([]byte, FixedHeaderSizeV2)

	// 0x00-0x03: Magic bytes "BORN"
	copy(fixedHeader[0:4], MagicBytes)

	// 0x04-0x07: Version (2)
	binary.LittleEndian.PutUint32(fixedHeader[4:8], uint32(FormatVersionV2))

	// 0x08-0x0B: Flags
	flags := uint32(0)
	if len(header.Metadata) > 0 {
		flags |= FlagHasMetadata
	}
	if header.CheckpointMeta != nil && header.CheckpointMeta.IsCheckpoint {
		flags |= FlagHasOptimizer
	}
	binary.LittleEndian.PutUint32(fixedHeader[8:12], flags)

	// 0x0C-0x0F: Reserved (0)

	// 0x10-0x17: Header size
	binary.LittleEndian.PutUint64(fixedHeader[16:24], headerSize)

	// 0x18-0x1F: Data size
	binary.LittleEndian.PutUint64(fixedHeader[24:32], dataSize)

	// 0x20-0x3F: SHA-256 checksum
	copy(fixedHeader[ChecksumOffsetV2:ChecksumOffsetV2+ChecksumSize], checksum[:])

	// Write fixed header
	if _, err := w.w.Write(fixedHeader); err != nil {
		return fmt.Errorf("failed to write fixed header: %w", err)
	}

	// Write header JSON
	if _, err := w.w.Write(headerJSON); err != nil {
		return fmt.Errorf("failed to write header JSON: %w", err)
	}

	// Calculate padding

	currentPos := int64(FixedHeaderSizeV2) + int64(headerSize) //nolint:gosec // G115: integer overflow conversion uint64 -> int64
	padding := (HeaderAlignment - (currentPos % HeaderAlignment)) % HeaderAlignment
	if padding > 0 {
		paddingBytes := make([]byte, padding)
		if _, err := w.w.Write(paddingBytes); err != nil {
			return fmt.Errorf("failed to write padding: %w", err)
		}
	}

	// Write tensor data
	if _, err := w.w.Write(tensorDataBuf); err != nil {
		return fmt.Errorf("failed to write tensor data: %w", err)
	}

	return nil
}

// Close closes the writer and the underlying file.
func (w *BornWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if w.closer == nil {
		return nil
	}
	return w.closer.Close()
}

// WriteTo writes the state dictionary to an io.Writer.
// This is useful for writing to buffers or network connections.
func WriteTo(writer io.Writer, stateDict map[string]*tensor.RawTensor, modelType string, metadata map[string]string) error {
	return newBornWriterToWriter(writer).WriteStateDict(stateDict, modelType, metadata)
}
