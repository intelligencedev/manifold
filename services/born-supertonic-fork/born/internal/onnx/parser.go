package onnx

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

// ParseFile parses an ONNX model from file.
//
//nolint:gosec // G304: Path is provided by user, file inclusion is intentional for ONNX model loading
func ParseFile(path string) (*ModelProto, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return Parse(data)
}

// Parse parses an ONNX model from bytes.
func Parse(data []byte) (*ModelProto, error) {
	p := &parser{data: data, pos: 0}
	model := &ModelProto{}
	if err := p.readMessage(model); err != nil {
		return nil, fmt.Errorf("failed to parse model: %w", err)
	}
	return model, nil
}

// parser implements a minimal protobuf wire format decoder.
type parser struct {
	data []byte
	pos  int
}

// Protobuf wire types.
const (
	wireVarint = 0 // int32, int64, uint32, uint64, sint32, sint64, bool, enum
	wire64Bit  = 1 // fixed64, sfixed64, double
	wireBytes  = 2 // string, bytes, embedded messages, packed repeated fields
	wire32Bit  = 5 // fixed32, sfixed32, float
)

// readMessage reads a protobuf message into the given struct.
func (p *parser) readMessage(msg interface{}) error {
	switch m := msg.(type) {
	case *ModelProto:
		return p.readModelProto(m)
	case *GraphProto:
		return p.readGraphProto(m)
	case *NodeProto:
		return p.readNodeProto(m)
	case *TensorProto:
		return p.readTensorProto(m)
	case *ValueInfoProto:
		return p.readValueInfoProto(m)
	case *TypeProto:
		return p.readTypeProto(m)
	case *TensorTypeProto:
		return p.readTensorTypeProto(m)
	case *TensorShapeProto:
		return p.readTensorShapeProto(m)
	case *DimensionProto:
		return p.readDimensionProto(m)
	case *AttributeProto:
		return p.readAttributeProto(m)
	case *OperatorSetID:
		return p.readOperatorSetID(m)
	case *StringStringEntry:
		return p.readStringStringEntry(m)
	default:
		return fmt.Errorf("unknown message type: %T", msg)
	}
}

// readModelProto reads ModelProto message.
//
//nolint:gocognit,gocyclo,cyclop,funlen // Protobuf parsing requires field-by-field switch logic for all ONNX message types
func (p *parser) readModelProto(m *ModelProto) error {
	for p.pos < len(p.data) {
		fieldNum, wireType, err := p.readTag()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		switch fieldNum {
		case 1: // ir_version
			m.IRVersion, err = p.readVarint()
		case 8: // opset_import
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			sub := &parser{data: data, pos: 0}
			opset := OperatorSetID{}
			if err2 := sub.readMessage(&opset); err2 != nil {
				return err2
			}
			m.OpsetImport = append(m.OpsetImport, opset)
			continue
		case 2: // producer_name
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			m.ProducerName = string(data)
			continue
		case 3: // producer_version
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			m.ProducerVersion = string(data)
			continue
		case 4: // domain
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			m.Domain = string(data)
			continue
		case 5: // model_version
			m.ModelVersion, err = p.readVarint()
		case 6: // doc_string
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			m.DocString = string(data)
			continue
		case 7: // graph
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			sub := &parser{data: data, pos: 0}
			m.Graph = &GraphProto{}
			if err2 := sub.readMessage(m.Graph); err2 != nil {
				return err2
			}
			continue
		case 14: // metadata_props
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			sub := &parser{data: data, pos: 0}
			entry := StringStringEntry{}
			if err2 := sub.readMessage(&entry); err2 != nil {
				return err2
			}
			m.MetadataProps = append(m.MetadataProps, entry)
			continue
		default:
			err = p.skipField(wireType)
		}

		if err != nil {
			return err
		}
	}
	return nil
}

// readGraphProto reads GraphProto message.
//
//nolint:gocognit,gocyclo,cyclop // Protobuf parsing requires field-by-field switch logic
func (p *parser) readGraphProto(m *GraphProto) error {
	for p.pos < len(p.data) {
		fieldNum, wireType, err := p.readTag()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		switch fieldNum {
		case 1: // node
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			sub := &parser{data: data, pos: 0}
			node := NodeProto{}
			if err2 := sub.readMessage(&node); err2 != nil {
				return err2
			}
			m.Nodes = append(m.Nodes, node)
			continue
		case 2: // name
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			m.Name = string(data)
			continue
		case 5: // initializer
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			sub := &parser{data: data, pos: 0}
			tensor := TensorProto{}
			if err2 := sub.readMessage(&tensor); err2 != nil {
				return err2
			}
			m.Initializers = append(m.Initializers, tensor)
			continue
		case 11: // input
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			sub := &parser{data: data, pos: 0}
			vi := ValueInfoProto{}
			if err2 := sub.readMessage(&vi); err2 != nil {
				return err2
			}
			m.Inputs = append(m.Inputs, vi)
			continue
		case 12: // output
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			sub := &parser{data: data, pos: 0}
			vi := ValueInfoProto{}
			if err2 := sub.readMessage(&vi); err2 != nil {
				return err2
			}
			m.Outputs = append(m.Outputs, vi)
			continue
		default:
			err = p.skipField(wireType)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// readNodeProto reads NodeProto message.
//
//nolint:gocognit,gocyclo,cyclop // Protobuf parsing requires field-by-field switch logic
func (p *parser) readNodeProto(m *NodeProto) error {
	for p.pos < len(p.data) {
		fieldNum, wireType, err := p.readTag()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		switch fieldNum {
		case 1: // input
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			m.Inputs = append(m.Inputs, string(data))
			continue
		case 2: // output
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			m.Outputs = append(m.Outputs, string(data))
			continue
		case 3: // name
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			m.Name = string(data)
			continue
		case 4: // op_type
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			m.OpType = string(data)
			continue
		case 5: // attribute
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			sub := &parser{data: data, pos: 0}
			attr := AttributeProto{}
			if err2 := sub.readMessage(&attr); err2 != nil {
				return err2
			}
			m.Attributes = append(m.Attributes, attr)
			continue
		case 7: // domain
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			m.Domain = string(data)
			continue
		default:
			err = p.skipField(wireType)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// readTensorProto reads TensorProto message.
//
//nolint:gocognit,gocyclo,cyclop,funlen // Protobuf parsing; int conversions are safe for tensor dimensions
func (p *parser) readTensorProto(m *TensorProto) error {
	for p.pos < len(p.data) {
		fieldNum, wireType, err := p.readTag()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		switch fieldNum {
		case 1: // dims (repeated int64)
			if wireType == wireBytes {
				// packed repeated
				data, err2 := p.readBytes()
				if err2 != nil {
					return err2
				}
				sub := &parser{data: data, pos: 0}
				for sub.pos < len(sub.data) {
					v, err3 := sub.readVarint()
					if err3 != nil {
						break
					}
					m.Dims = append(m.Dims, v)
				}
				continue
			}
			v, err2 := p.readVarint()
			if err2 != nil {
				return err2
			}
			m.Dims = append(m.Dims, v)
			continue
		case 2: // data_type
			m.DataType, err = p.readInt32()
		case 4: // float_data (packed)
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			for i := 0; i+4 <= len(data); i += 4 {
				bits := binary.LittleEndian.Uint32(data[i:])
				m.FloatData = append(m.FloatData, math.Float32frombits(bits))
			}
			continue
		case 5: // int32_data (packed)
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			sub := &parser{data: data, pos: 0}
			for sub.pos < len(sub.data) {
				v, err3 := sub.readVarint()
				if err3 != nil {
					break
				}
				m.Int32Data = append(m.Int32Data, int32(v)) //nolint:gosec // G115: integer overflow conversion int64 -> int32
			}
			continue
		case 7: // int64_data (packed)
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			sub := &parser{data: data, pos: 0}
			for sub.pos < len(sub.data) {
				v, err3 := sub.readVarint()
				if err3 != nil {
					break
				}
				m.Int64Data = append(m.Int64Data, v)
			}
			continue
		case 8: // name
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			m.Name = string(data)
			continue
		case 9: // raw_data
			m.RawData, err = p.readBytes()
		default:
			err = p.skipField(wireType)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// readValueInfoProto reads ValueInfoProto message.
func (p *parser) readValueInfoProto(m *ValueInfoProto) error {
	for p.pos < len(p.data) {
		fieldNum, wireType, err := p.readTag()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		switch fieldNum {
		case 1: // name
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			m.Name = string(data)
			continue
		case 2: // type
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			sub := &parser{data: data, pos: 0}
			m.Type = &TypeProto{}
			if err2 := sub.readMessage(m.Type); err2 != nil {
				return err2
			}
			continue
		default:
			err = p.skipField(wireType)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// readTypeProto reads TypeProto message.
func (p *parser) readTypeProto(m *TypeProto) error {
	for p.pos < len(p.data) {
		fieldNum, wireType, err := p.readTag()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		switch fieldNum {
		case 1: // tensor_type
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			sub := &parser{data: data, pos: 0}
			m.TensorType = &TensorTypeProto{}
			if err2 := sub.readMessage(m.TensorType); err2 != nil {
				return err2
			}
			continue
		default:
			err = p.skipField(wireType)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// readTensorTypeProto reads TensorTypeProto message.
func (p *parser) readTensorTypeProto(m *TensorTypeProto) error {
	for p.pos < len(p.data) {
		fieldNum, wireType, err := p.readTag()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		switch fieldNum {
		case 1: // elem_type
			m.ElemType, err = p.readInt32()
		case 2: // shape
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			sub := &parser{data: data, pos: 0}
			m.Shape = &TensorShapeProto{}
			if err2 := sub.readMessage(m.Shape); err2 != nil {
				return err2
			}
			continue
		default:
			err = p.skipField(wireType)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// readTensorShapeProto reads TensorShapeProto message.
func (p *parser) readTensorShapeProto(m *TensorShapeProto) error {
	for p.pos < len(p.data) {
		fieldNum, wireType, err := p.readTag()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		switch fieldNum {
		case 1: // dim
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			sub := &parser{data: data, pos: 0}
			dim := DimensionProto{}
			if err2 := sub.readMessage(&dim); err2 != nil {
				return err2
			}
			m.Dims = append(m.Dims, dim)
			continue
		default:
			err = p.skipField(wireType)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// readDimensionProto reads DimensionProto message.
func (p *parser) readDimensionProto(m *DimensionProto) error {
	for p.pos < len(p.data) {
		fieldNum, wireType, err := p.readTag()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		switch fieldNum {
		case 1: // dim_value
			m.DimValue, err = p.readVarint()
		case 2: // dim_param
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			m.DimParam = string(data)
			continue
		default:
			err = p.skipField(wireType)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// readAttributeProto reads AttributeProto message.
//
//nolint:gocognit,gocyclo,cyclop,funlen // Protobuf parsing requires field-by-field switch logic
func (p *parser) readAttributeProto(m *AttributeProto) error {
	for p.pos < len(p.data) {
		fieldNum, wireType, err := p.readTag()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		switch fieldNum {
		case 1: // name
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			m.Name = string(data)
			continue
		case 2: // f (float)
			m.F, err = p.readFloat32()
		case 3: // i (int)
			m.I, err = p.readVarint()
		case 4: // s (bytes)
			m.S, err = p.readBytes()
		case 5: // t (tensor)
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			sub := &parser{data: data, pos: 0}
			tensor := TensorProto{}
			if err2 := sub.readMessage(&tensor); err2 != nil {
				return err2
			}
			m.T = &tensor
			continue
		case 7: // floats
			switch wireType {
			case wire32Bit:
				v, err := p.readFloat32()
				if err != nil {
					return err
				}
				m.Floats = append(m.Floats, v)
			case wireBytes:
				data, err := p.readBytes()
				if err != nil {
					return err
				}
				for i := 0; i+4 <= len(data); i += 4 {
					bits := binary.LittleEndian.Uint32(data[i:])
					m.Floats = append(m.Floats, math.Float32frombits(bits))
				}
			default:
				return fmt.Errorf("unsupported wire type %d for floats", wireType)
			}
			continue
		case 8: // ints
			switch wireType {
			case wireVarint:
				v, err := p.readVarint()
				if err != nil {
					return err
				}
				m.Ints = append(m.Ints, v)
			case wireBytes:
				data, err := p.readBytes()
				if err != nil {
					return err
				}
				sub := &parser{data: data, pos: 0}
				for sub.pos < len(sub.data) {
					v, err := sub.readVarint()
					if err != nil {
						break
					}
					m.Ints = append(m.Ints, v)
				}
			default:
				return fmt.Errorf("unsupported wire type %d for ints", wireType)
			}
			continue
		case 9: // strings
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			m.Strings = append(m.Strings, data)
			continue
		case 20: // type
			v, err2 := p.readVarint()
			if err2 != nil {
				return err2
			}
			m.Type = int32(v) //nolint:gosec // G115: integer overflow conversion int64 -> int32
			continue
		default:
			err = p.skipField(wireType)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// readOperatorSetID reads OperatorSetID message.
func (p *parser) readOperatorSetID(m *OperatorSetID) error {
	for p.pos < len(p.data) {
		fieldNum, wireType, err := p.readTag()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		switch fieldNum {
		case 1: // domain
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			m.Domain = string(data)
			continue
		case 2: // version
			m.Version, err = p.readVarint()
		default:
			err = p.skipField(wireType)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// readStringStringEntry reads StringStringEntry message.
func (p *parser) readStringStringEntry(m *StringStringEntry) error {
	for p.pos < len(p.data) {
		fieldNum, wireType, err := p.readTag()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		switch fieldNum {
		case 1: // key
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			m.Key = string(data)
			continue
		case 2: // value
			data, err2 := p.readBytes()
			if err2 != nil {
				return err2
			}
			m.Value = string(data)
			continue
		default:
			err = p.skipField(wireType)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// readTag reads a protobuf field tag.
func (p *parser) readTag() (fieldNum, wireType int, err error) {
	if p.pos >= len(p.data) {
		return 0, 0, io.EOF
	}
	tag, err := p.readVarint()
	if err != nil {
		return 0, 0, err
	}
	fieldNum = int(tag >> 3)
	wireType = int(tag & 0x7)
	return fieldNum, wireType, nil
}

// readVarint reads a varint-encoded int64.
func (p *parser) readVarint() (int64, error) {
	var result uint64
	var shift uint
	for {
		if p.pos >= len(p.data) {
			return 0, io.EOF
		}
		b := p.data[p.pos]
		p.pos++
		result |= uint64(b&0x7f) << shift
		if b&0x80 == 0 {
			break
		}
		shift += 7
		if shift >= 64 {
			return 0, errors.New("varint overflow")
		}
	}
	return int64(result), nil //nolint:gosec // G115: integer overflow conversion uint64 -> int64
}

// readInt32 reads a varint-encoded int32.
func (p *parser) readInt32() (int32, error) {
	v, err := p.readVarint()
	if err != nil {
		return 0, err
	}
	return int32(v), nil //nolint:gosec // G115: integer overflow conversion int64 -> int32
}

// readBytes reads a length-delimited byte slice.
func (p *parser) readBytes() ([]byte, error) {
	length, err := p.readVarint()
	if err != nil {
		return nil, err
	}
	if length < 0 {
		return nil, errors.New("negative length")
	}
	end := p.pos + int(length)
	if end > len(p.data) {
		return nil, io.ErrUnexpectedEOF
	}
	result := p.data[p.pos:end]
	p.pos = end
	return result, nil
}

// readFloat32 reads a 32-bit float.
func (p *parser) readFloat32() (float32, error) {
	if p.pos+4 > len(p.data) {
		return 0, io.ErrUnexpectedEOF
	}
	bits := binary.LittleEndian.Uint32(p.data[p.pos:])
	p.pos += 4
	return math.Float32frombits(bits), nil
}

// skipField skips a field based on wire type.
func (p *parser) skipField(wireType int) error {
	switch wireType {
	case wireVarint:
		_, err := p.readVarint()
		return err
	case wire64Bit:
		if p.pos+8 > len(p.data) {
			return io.ErrUnexpectedEOF
		}
		p.pos += 8
		return nil
	case wireBytes:
		_, err := p.readBytes()
		return err
	case wire32Bit:
		if p.pos+4 > len(p.data) {
			return io.ErrUnexpectedEOF
		}
		p.pos += 4
		return nil
	default:
		return fmt.Errorf("unknown wire type: %d", wireType)
	}
}
