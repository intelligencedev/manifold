package gguf

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// Parse reads and parses a GGUF file from the given reader.
func Parse(r io.ReadSeeker) (*File, error) {
	p := &parser{
		r:     r,
		order: binary.LittleEndian, // Default to little-endian
	}
	return p.parse()
}

// ParseFile parses a GGUF file from disk.
//
//nolint:gosec // G304: path comes from trusted caller, not user input.
func ParseFile(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() {
		_ = f.Close() // Ignore close error on read-only file.
	}()

	// Get file size
	stat, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}

	gguf, err := Parse(f)
	if err != nil {
		return nil, err
	}

	gguf.FilePath = path
	gguf.FileSize = stat.Size()

	return gguf, nil
}

type parser struct {
	r     io.ReadSeeker
	order binary.ByteOrder
}

func (p *parser) parse() (*File, error) {
	file := &File{
		Metadata:  make(map[string]interface{}),
		Alignment: DefaultAlignment,
	}

	// Read and validate header
	if err := p.parseHeader(&file.Header); err != nil {
		return nil, fmt.Errorf("parse header: %w", err)
	}

	// Read metadata key-value pairs
	for i := uint64(0); i < file.Header.MetadataKVCount; i++ {
		kv, err := p.parseMetadataKV()
		if err != nil {
			return nil, fmt.Errorf("parse metadata kv %d: %w", i, err)
		}
		file.Metadata[kv.Key] = kv.Value

		// Check for alignment override
		if kv.Key == "general.alignment" {
			if align, ok := kv.Value.(uint32); ok {
				file.Alignment = int(align)
			}
		}
	}

	// Read tensor info
	file.TensorInfo = make([]TensorInfo, file.Header.TensorCount)
	for i := uint64(0); i < file.Header.TensorCount; i++ {
		if err := p.parseTensorInfo(&file.TensorInfo[i]); err != nil {
			return nil, fmt.Errorf("parse tensor info %d: %w", i, err)
		}
	}

	// Calculate tensor data offset (current position + padding)
	pos, err := p.r.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, fmt.Errorf("get position: %w", err)
	}
	file.TensorDataOffset = alignOffset(pos, file.Alignment)

	return file, nil
}

func (p *parser) parseHeader(h *Header) error {
	// Read magic
	if err := binary.Read(p.r, p.order, &h.Magic); err != nil {
		return fmt.Errorf("read magic: %w", err)
	}

	// Validate magic and detect byte order
	switch h.Magic {
	case MagicGGUFLE:
		p.order = binary.LittleEndian
	case MagicGGUFBE:
		p.order = binary.BigEndian
	default:
		return fmt.Errorf("invalid magic: 0x%08X (expected GGUF)", h.Magic)
	}

	// Read version
	if err := binary.Read(p.r, p.order, &h.Version); err != nil {
		return fmt.Errorf("read version: %w", err)
	}

	// Validate version
	if h.Version < Version1 || h.Version > Version3 {
		return fmt.Errorf("unsupported version: %d (supported: 1-3)", h.Version)
	}

	// Read tensor count
	if err := binary.Read(p.r, p.order, &h.TensorCount); err != nil {
		return fmt.Errorf("read tensor count: %w", err)
	}

	// Read metadata kv count
	if err := binary.Read(p.r, p.order, &h.MetadataKVCount); err != nil {
		return fmt.Errorf("read metadata kv count: %w", err)
	}

	return nil
}

func (p *parser) parseMetadataKV() (*MetadataKV, error) {
	kv := &MetadataKV{}

	// Read key
	key, err := readString(p.r, p.order)
	if err != nil {
		return nil, fmt.Errorf("read key: %w", err)
	}
	kv.Key = key

	// Read value type
	var valueType uint32
	if err := binary.Read(p.r, p.order, &valueType); err != nil {
		return nil, fmt.Errorf("read value type: %w", err)
	}
	kv.ValueType = ValueType(valueType)

	// Read value
	value, err := p.parseValue(kv.ValueType)
	if err != nil {
		return nil, fmt.Errorf("read value: %w", err)
	}
	kv.Value = value

	return kv, nil
}

// parseValue reads a metadata value of the given type.
func (p *parser) parseValue(t ValueType) (interface{}, error) {
	// Handle special cases first.
	switch t {
	case ValueTypeBool:
		var v uint8
		if err := binary.Read(p.r, p.order, &v); err != nil {
			return nil, err
		}
		return v != 0, nil

	case ValueTypeString:
		return readString(p.r, p.order)

	case ValueTypeArray:
		return p.parseArray()
	}

	// Handle numeric types using reflection-like approach.
	return p.parseNumericValue(t)
}

// parseNumericValue reads a numeric metadata value.
func (p *parser) parseNumericValue(t ValueType) (interface{}, error) {
	switch t {
	case ValueTypeUint8:
		var v uint8
		err := binary.Read(p.r, p.order, &v)
		return v, err

	case ValueTypeInt8:
		var v int8
		err := binary.Read(p.r, p.order, &v)
		return v, err

	case ValueTypeUint16:
		var v uint16
		err := binary.Read(p.r, p.order, &v)
		return v, err

	case ValueTypeInt16:
		var v int16
		err := binary.Read(p.r, p.order, &v)
		return v, err

	case ValueTypeUint32:
		var v uint32
		err := binary.Read(p.r, p.order, &v)
		return v, err

	case ValueTypeInt32:
		var v int32
		err := binary.Read(p.r, p.order, &v)
		return v, err

	case ValueTypeFloat32:
		var v float32
		err := binary.Read(p.r, p.order, &v)
		return v, err

	case ValueTypeUint64:
		var v uint64
		err := binary.Read(p.r, p.order, &v)
		return v, err

	case ValueTypeInt64:
		var v int64
		err := binary.Read(p.r, p.order, &v)
		return v, err

	case ValueTypeFloat64:
		var v float64
		err := binary.Read(p.r, p.order, &v)
		return v, err

	default:
		return nil, fmt.Errorf("unknown value type: %d", t)
	}
}

func (p *parser) parseArray() (interface{}, error) {
	// Read element type.
	var elemType uint32
	if err := binary.Read(p.r, p.order, &elemType); err != nil {
		return nil, fmt.Errorf("read array element type: %w", err)
	}

	// Read array length.
	var length uint64
	if err := binary.Read(p.r, p.order, &length); err != nil {
		return nil, fmt.Errorf("read array length: %w", err)
	}

	// Sanity check.
	if length > 100_000_000 {
		return nil, fmt.Errorf("array too large: %d elements", length)
	}

	vt := ValueType(elemType)

	// Dispatch to type-specific parser.
	return p.parseArrayOfType(vt, length)
}

// parseArrayOfType parses an array of the given element type.
func (p *parser) parseArrayOfType(vt ValueType, length uint64) (interface{}, error) {
	switch vt {
	case ValueTypeUint8:
		return p.readUint8Array(length)
	case ValueTypeInt8:
		return p.readInt8Array(length)
	case ValueTypeUint16:
		return p.readUint16Array(length)
	case ValueTypeInt16:
		return p.readInt16Array(length)
	case ValueTypeUint32:
		return p.readUint32Array(length)
	case ValueTypeInt32:
		return p.readInt32Array(length)
	case ValueTypeFloat32:
		return p.readFloat32Array(length)
	case ValueTypeUint64:
		return p.readUint64Array(length)
	case ValueTypeInt64:
		return p.readInt64Array(length)
	case ValueTypeFloat64:
		return p.readFloat64Array(length)
	case ValueTypeBool:
		return p.readBoolArray(length)
	case ValueTypeString:
		return p.readStringArray(length)
	default:
		return nil, fmt.Errorf("unsupported array element type: %s", vt)
	}
}

func (p *parser) readUint8Array(length uint64) ([]uint8, error) {
	arr := make([]uint8, length)
	for i := uint64(0); i < length; i++ {
		if err := binary.Read(p.r, p.order, &arr[i]); err != nil {
			return nil, err
		}
	}
	return arr, nil
}

func (p *parser) readInt8Array(length uint64) ([]int8, error) {
	arr := make([]int8, length)
	for i := uint64(0); i < length; i++ {
		if err := binary.Read(p.r, p.order, &arr[i]); err != nil {
			return nil, err
		}
	}
	return arr, nil
}

func (p *parser) readUint16Array(length uint64) ([]uint16, error) {
	arr := make([]uint16, length)
	for i := uint64(0); i < length; i++ {
		if err := binary.Read(p.r, p.order, &arr[i]); err != nil {
			return nil, err
		}
	}
	return arr, nil
}

func (p *parser) readInt16Array(length uint64) ([]int16, error) {
	arr := make([]int16, length)
	for i := uint64(0); i < length; i++ {
		if err := binary.Read(p.r, p.order, &arr[i]); err != nil {
			return nil, err
		}
	}
	return arr, nil
}

func (p *parser) readUint32Array(length uint64) ([]uint32, error) {
	arr := make([]uint32, length)
	for i := uint64(0); i < length; i++ {
		if err := binary.Read(p.r, p.order, &arr[i]); err != nil {
			return nil, err
		}
	}
	return arr, nil
}

func (p *parser) readInt32Array(length uint64) ([]int32, error) {
	arr := make([]int32, length)
	for i := uint64(0); i < length; i++ {
		if err := binary.Read(p.r, p.order, &arr[i]); err != nil {
			return nil, err
		}
	}
	return arr, nil
}

func (p *parser) readFloat32Array(length uint64) ([]float32, error) {
	arr := make([]float32, length)
	for i := uint64(0); i < length; i++ {
		if err := binary.Read(p.r, p.order, &arr[i]); err != nil {
			return nil, err
		}
	}
	return arr, nil
}

func (p *parser) readUint64Array(length uint64) ([]uint64, error) {
	arr := make([]uint64, length)
	for i := uint64(0); i < length; i++ {
		if err := binary.Read(p.r, p.order, &arr[i]); err != nil {
			return nil, err
		}
	}
	return arr, nil
}

func (p *parser) readInt64Array(length uint64) ([]int64, error) {
	arr := make([]int64, length)
	for i := uint64(0); i < length; i++ {
		if err := binary.Read(p.r, p.order, &arr[i]); err != nil {
			return nil, err
		}
	}
	return arr, nil
}

func (p *parser) readFloat64Array(length uint64) ([]float64, error) {
	arr := make([]float64, length)
	for i := uint64(0); i < length; i++ {
		if err := binary.Read(p.r, p.order, &arr[i]); err != nil {
			return nil, err
		}
	}
	return arr, nil
}

func (p *parser) readBoolArray(length uint64) ([]bool, error) {
	arr := make([]bool, length)
	for i := uint64(0); i < length; i++ {
		var v uint8
		if err := binary.Read(p.r, p.order, &v); err != nil {
			return nil, err
		}
		arr[i] = v != 0
	}
	return arr, nil
}

func (p *parser) readStringArray(length uint64) ([]string, error) {
	arr := make([]string, length)
	for i := uint64(0); i < length; i++ {
		s, err := readString(p.r, p.order)
		if err != nil {
			return nil, err
		}
		arr[i] = s
	}
	return arr, nil
}

func (p *parser) parseTensorInfo(t *TensorInfo) error {
	// Read name
	name, err := readString(p.r, p.order)
	if err != nil {
		return fmt.Errorf("read tensor name: %w", err)
	}
	t.Name = name

	// Read number of dimensions
	if err := binary.Read(p.r, p.order, &t.NDims); err != nil {
		return fmt.Errorf("read ndims: %w", err)
	}

	// Validate dimensions
	if t.NDims > 8 {
		return fmt.Errorf("too many dimensions: %d", t.NDims)
	}

	// Read dimensions
	t.Dimensions = make([]uint64, t.NDims)
	for i := uint32(0); i < t.NDims; i++ {
		if err := binary.Read(p.r, p.order, &t.Dimensions[i]); err != nil {
			return fmt.Errorf("read dimension %d: %w", i, err)
		}
	}

	// Read type
	var ggmlType uint32
	if err := binary.Read(p.r, p.order, &ggmlType); err != nil {
		return fmt.Errorf("read type: %w", err)
	}
	t.Type = GGMLType(ggmlType)

	// Read offset
	if err := binary.Read(p.r, p.order, &t.Offset); err != nil {
		return fmt.Errorf("read offset: %w", err)
	}

	return nil
}
