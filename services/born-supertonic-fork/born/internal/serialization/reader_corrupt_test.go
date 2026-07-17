package serialization

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// v1Prefix builds the 20-byte fixed prefix of a v1 .born stream: magic, version,
// flags, and the header-size field. headerSize is stamped verbatim so a test can
// forge a size that disagrees with the header bytes that follow.
func v1Prefix(flags uint32, headerSize uint64) []byte {
	p := make([]byte, 20)
	copy(p[0:4], MagicBytes)
	binary.LittleEndian.PutUint32(p[4:8], uint32(FormatVersion))
	binary.LittleEndian.PutUint32(p[8:12], flags)
	binary.LittleEndian.PutUint64(p[12:20], headerSize)
	return p
}

// forgeV1 assembles a v1 .born stream from explicit parts so a test can damage a
// single stage. headerSize is the value stamped into the size field; it need not
// equal len(headerJSON) — that mismatch is what several corrupt rows forge. The
// alignment padding is sized from the header bytes actually written, and the
// tensor-data section follows it.
func forgeV1(flags uint32, headerSize uint64, headerJSON, data []byte) []byte {
	currentPos := int64(len(v1Prefix(0, 0))) + int64(len(headerJSON))
	padding := (HeaderAlignment - (currentPos % HeaderAlignment)) % HeaderAlignment

	out := make([]byte, 0, 20+len(headerJSON)+int(padding)+len(data))
	out = append(out, v1Prefix(flags, headerSize)...)
	out = append(out, headerJSON...)
	out = append(out, make([]byte, padding)...)
	out = append(out, data...)
	return out
}

// marshalHeader encodes a header to JSON or fails the test.
func marshalHeader(t *testing.T, h Header) []byte {
	t.Helper()

	b, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("Failed to marshal header: %v", err)
	}
	return b
}

// oneTensorHeader builds a v1 header describing a single tensor named "weight".
// Callers tweak DType, Shape or Size to forge a header the reader must reject.
func oneTensorHeader(dtype string, shape []int, size int64) Header {
	return Header{
		FormatVersion: FormatVersion,
		BornVersion:   bornVersion,
		ModelType:     "TestModel",
		Tensors:       []TensorMeta{{Name: "weight", DType: dtype, Shape: shape, Offset: 0, Size: size}},
		Metadata:      map[string]string{},
	}
}

// wellFormedV1 assembles a v1 stream whose size field matches its header bytes,
// so the stream parses cleanly up to whatever the header content or data section
// makes the reader reject.
func wellFormedV1(t *testing.T, h Header, data []byte) []byte {
	t.Helper()

	hj := marshalHeader(t, h)
	return forgeV1(0, uint64(len(hj)), hj, data)
}

// TestNewBornReaderFromBytes_CorruptHeader verifies the v1 header parser rejects
// a stream that is truncated at, or lies about, each header stage — named by the
// stage that failed, with the format sentinel surfaced where one exists.
func TestNewBornReaderFromBytes_CorruptHeader(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr error  // sentinel to match with errors.Is, or nil
		wantSub string // substring to match when wantErr is nil
	}{
		{"flags truncated", append([]byte(MagicBytes), 0x01, 0x00, 0x00, 0x00), nil, "failed to read flags"},
		{"header size truncated", append([]byte(MagicBytes), 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00), nil, "failed to read header size"},
		{"header too large", v1Prefix(0, MaxHeaderSize+1), ErrHeaderTooLarge, ""},
		{"header truncated", append(v1Prefix(0, 100), []byte("{}")...), nil, "failed to read header"},
		{"malformed header JSON", append(v1Prefix(0, 5), []byte("hello")...), nil, "failed to parse header JSON"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewBornReaderFromBytes(tt.data)
			if err == nil {
				t.Fatalf("Expected error for %q, got nil", tt.name)
			}
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("Expected error to wrap %v, got: %v", tt.wantErr, err)
				}
				return
			}
			if got := err.Error(); !strings.Contains(got, tt.wantSub) {
				t.Errorf("Expected error to mention %q, got: %v", tt.wantSub, got)
			}
		})
	}
}

// TestNewBornReaderFromBytes_ValidationRejects verifies the constructor runs the
// header through strict validation and refuses a file whose metadata would let a
// read stray beyond the data section or resolve a path-traversal name.
func TestNewBornReaderFromBytes_ValidationRejects(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"out of bounds tensor", wellFormedV1(t, oneTensorHeader(DTypeFloat32, []int{2, 2}, 16), make([]byte, 8))},
		{"path traversal name", wellFormedV1(t, func() Header {
			h := oneTensorHeader(DTypeFloat32, []int{2, 2}, 16)
			h.Tensors[0].Name = "../evil"
			return h
		}(), make([]byte, 16))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewBornReaderFromBytes(tt.data)
			if err == nil {
				t.Fatalf("Expected validation to reject %q, got nil", tt.name)
			}
			if got := err.Error(); !strings.Contains(got, "validation failed") {
				t.Errorf("Expected a validation failure, got: %v", got)
			}
		})
	}
}

// TestLoadTensor_CorruptTensor verifies the per-tensor load rejects a stored
// header whose dtype is unknown or whose shape is degenerate, rather than
// returning mislabeled or zero-shaped data. Both files pass strict header
// validation, so the rejection is LoadTensor's own.
func TestLoadTensor_CorruptTensor(t *testing.T) {
	backend := tensor.NewMockBackend()

	tests := []struct {
		name    string
		header  Header
		data    []byte
		wantSub string
	}{
		{"unsupported dtype", oneTensorHeader("float16", []int{2, 2}, 16), make([]byte, 16), "unsupported dtype"},
		{"invalid shape", oneTensorHeader(DTypeFloat32, []int{2, 0}, 0), nil, "invalid shape"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, err := NewBornReaderFromBytes(wellFormedV1(t, tt.header, tt.data))
			if err != nil {
				t.Fatalf("Failed to open reader: %v", err)
			}
			defer reader.Close()

			if _, err := reader.LoadTensor("weight", backend); err == nil {
				t.Fatalf("Expected LoadTensor to reject %q, got nil", tt.name)
			} else if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("Expected error to mention %q, got: %v", tt.wantSub, err)
			}
		})
	}
}

// TestLoadTensor_DataReadFailUnvalidated verifies that even with offset
// validation disabled a short data section is caught at read time: the header
// claims more tensor bytes than the stream carries, so the read fails rather
// than returning a truncated buffer.
func TestLoadTensor_DataReadFailUnvalidated(t *testing.T) {
	backend := tensor.NewMockBackend()

	data := wellFormedV1(t, oneTensorHeader(DTypeFloat32, []int{2, 2}, 16), make([]byte, 8))
	reader, err := NewBornReaderFromBytesWithOptions(data, ReaderOptions{ValidationLevel: ValidationNone})
	if err != nil {
		t.Fatalf("Failed to open reader: %v", err)
	}
	defer reader.Close()

	if _, err := reader.LoadTensor("weight", backend); err == nil {
		t.Fatal("Expected LoadTensor to fail on a short data section, got nil")
	} else if !strings.Contains(err.Error(), "failed to read tensor data") {
		t.Errorf("Expected a data read failure, got: %v", err)
	}
}

// TestReadFrom_CorruptStream verifies the streaming reader rejects a stream that
// is truncated at, or lies about, each stage past the magic and version bytes
// its sibling table already covers — header prefix, padding, and the per-tensor
// dtype, shape and data arms — each named by the stage that failed.
func TestReadFrom_CorruptStream(t *testing.T) {
	backend := tensor.NewMockBackend()

	tests := []struct {
		name    string
		data    []byte
		wantSub string
	}{
		{"flags truncated", append([]byte(MagicBytes), 0x01, 0x00, 0x00, 0x00), "failed to read flags"},
		{"header size truncated", append([]byte(MagicBytes), 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00), "failed to read header size"},
		{"header too large", v1Prefix(0, MaxHeaderSize+1), "invalid header size"},
		{"malformed header JSON", append(v1Prefix(0, 5), []byte("hello")...), "failed to parse header JSON"},
		{"padding truncated", v1TruncatedPadding(t, oneTensorHeader(DTypeFloat32, []int{2, 2}, 16)), "failed to read padding"},
		{"unsupported dtype", wellFormedV1(t, oneTensorHeader("float16", []int{2, 2}, 16), make([]byte, 16)), "unsupported dtype"},
		{"invalid shape", wellFormedV1(t, oneTensorHeader(DTypeFloat32, []int{2, 0}, 0), nil), "invalid shape"},
		{"truncated tensor data", wellFormedV1(t, oneTensorHeader(DTypeFloat32, []int{2, 2}, 16), make([]byte, 8)), "failed to read tensor weight"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ReadFrom(bytes.NewReader(tt.data), backend)
			if err == nil {
				t.Fatalf("Expected error for %q, got nil", tt.name)
			}
			if got := err.Error(); !strings.Contains(got, tt.wantSub) {
				t.Errorf("Expected error to mention %q, got: %v", tt.wantSub, got)
			}
		})
	}
}

// v1TruncatedPadding builds a well-formed v1 header whose stream ends one byte
// short of the alignment padding, so a reader fails while skipping padding.
func v1TruncatedPadding(t *testing.T, h Header) []byte {
	t.Helper()

	hj := marshalHeader(t, h)
	prefix := v1Prefix(0, uint64(len(hj)))
	currentPos := int64(len(prefix)) + int64(len(hj))
	padding := (HeaderAlignment - (currentPos % HeaderAlignment)) % HeaderAlignment
	if padding == 0 {
		t.Fatal("Header aligns exactly; no padding to truncate")
	}

	out := make([]byte, 0, len(prefix)+len(hj)+int(padding)-1)
	out = append(out, prefix...)
	out = append(out, hj...)
	out = append(out, make([]byte, padding-1)...)
	return out
}
