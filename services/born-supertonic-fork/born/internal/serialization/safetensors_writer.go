package serialization

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/born-ml/born/internal/tensor"
)

// SafeTensorsWriter writes models in SafeTensors format.
// SafeTensors is the standard format for HuggingFace models.
type SafeTensorsWriter struct {
	file   *os.File
	closed bool
}

// SafeTensorHeader represents a tensor in the SafeTensors header.
type SafeTensorHeader struct {
	DType       string   `json:"dtype"`
	Shape       []int64  `json:"shape"`
	DataOffsets [2]int64 `json:"data_offsets"`
}

// NewSafeTensorsWriter creates a new SafeTensors file writer.
func NewSafeTensorsWriter(path string) (*SafeTensorsWriter, error) {
	//nolint:gosec // G304: File path comes from user input, which is expected for model saving
	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	return &SafeTensorsWriter{
		file:   file,
		closed: false,
	}, nil
}

// WriteSafeTensors writes tensors to a SafeTensors file.
//
// Format:
// [8 bytes: header_size (uint64 LE)]
// [header_size bytes: JSON header]
// [tensor data: raw bytes]
//
// Tensors are written in alphabetical order by name.
func WriteSafeTensors(path string, tensors map[string]*tensor.RawTensor, metadata map[string]string) error {
	writer, err := NewSafeTensorsWriter(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = writer.Close() // Best effort close
	}()

	return writer.WriteStateDict(tensors, metadata)
}

// WriteStateDict writes a state dictionary to the SafeTensors file.
//
// The state dictionary is a map from parameter names to tensors.
// Tensors are written in alphabetical order by name (SafeTensors requirement).
func (w *SafeTensorsWriter) WriteStateDict(stateDict map[string]*tensor.RawTensor, metadata map[string]string) error {
	if w.closed {
		return fmt.Errorf("writer is closed")
	}

	// Sort tensor names alphabetically (SafeTensors requirement)
	tensorNames := make([]string, 0, len(stateDict))
	for name := range stateDict {
		tensorNames = append(tensorNames, name)
	}
	sort.Strings(tensorNames)

	// Build header with tensor metadata
	header := make(map[string]interface{})

	// Add metadata if provided
	if len(metadata) > 0 {
		header["__metadata__"] = metadata
	}

	// Calculate data offsets for each tensor
	var currentOffset int64
	for _, name := range tensorNames {
		raw := stateDict[name]
		shape := raw.Shape()
		size := int64(raw.NumElements() * raw.DType().Size())

		// Convert shape to []int64 (SafeTensors requirement)
		shapeInt64 := make([]int64, len(shape))
		for i, dim := range shape {
			shapeInt64[i] = int64(dim)
		}

		tensorHeader := SafeTensorHeader{
			DType:       dtypeToSafeTensors(raw.DType()),
			Shape:       shapeInt64,
			DataOffsets: [2]int64{currentOffset, currentOffset + size},
		}

		header[name] = tensorHeader
		currentOffset += size
	}

	// Marshal header to JSON
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("failed to marshal header: %w", err)
	}

	// Write header size (8 bytes, little-endian uint64)
	headerSize := uint64(len(headerJSON))
	if err := binary.Write(w.file, binary.LittleEndian, headerSize); err != nil {
		return fmt.Errorf("failed to write header size: %w", err)
	}

	// Write header JSON
	if _, err := w.file.Write(headerJSON); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Write tensor data in alphabetical order
	for _, name := range tensorNames {
		raw := stateDict[name]
		data := raw.Data()
		if _, err := w.file.Write(data); err != nil {
			return fmt.Errorf("failed to write tensor %s: %w", name, err)
		}
	}

	return nil
}

// Close closes the writer and the underlying file.
func (w *SafeTensorsWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	return w.file.Close()
}

// dtypeToSafeTensors converts tensor.DataType to SafeTensors dtype string.
func dtypeToSafeTensors(dt tensor.DataType) string {
	switch dt {
	case tensor.Float32:
		return "F32"
	case tensor.Float64:
		return "F64"
	case tensor.Int32:
		return "I32"
	case tensor.Int64:
		return "I64"
	case tensor.Uint8:
		return "U8"
	case tensor.Bool:
		return "BOOL"
	default:
		return "F32" // Default to F32 for unknown types
	}
}
