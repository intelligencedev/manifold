package serialization

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// writeTestBorn encodes the shared test state dict to a v1 .born byte slice.
func writeTestBorn(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	if err := WriteTo(&buf, newTestStateDict(t), "TestModel", map[string]string{"dataset": "MNIST"}); err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}
	return buf.Bytes()
}

// TestReaderAccessors verifies the header accessors report what was written.
func TestReaderAccessors(t *testing.T) {
	reader, err := NewBornReaderFromBytes(writeTestBorn(t))
	if err != nil {
		t.Fatalf("Failed to open reader: %v", err)
	}
	defer reader.Close()

	if got := reader.Metadata()["dataset"]; got != "MNIST" {
		t.Errorf("Metadata()[dataset] = %q, want %q", got, "MNIST")
	}
	if got := reader.Header().ModelType; got != "TestModel" {
		t.Errorf("Header().ModelType = %q, want %q", got, "TestModel")
	}
	names := reader.TensorNames()
	if len(names) != 1 || names[0] != "weight" {
		t.Errorf("TensorNames() = %v, want [weight]", names)
	}
}

// TestReadFrom_RoundTrip verifies the io.Reader read path recovers the tensors
// and header WriteTo produced.
func TestReadFrom_RoundTrip(t *testing.T) {
	backend := tensor.NewMockBackend()

	var buf bytes.Buffer
	if err := WriteTo(&buf, newTestStateDict(t), "TestModel", nil); err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}

	loaded, header, err := ReadFrom(bytes.NewReader(buf.Bytes()), backend)
	if err != nil {
		t.Fatalf("ReadFrom failed: %v", err)
	}
	if header.ModelType != "TestModel" {
		t.Errorf("header.ModelType = %q, want %q", header.ModelType, "TestModel")
	}

	raw, ok := loaded["weight"]
	if !ok {
		t.Fatal("Tensor 'weight' not found")
	}
	want := []float32{1.0, 2.0, 3.0, 4.0}
	got := raw.AsFloat32()
	if len(got) != len(want) {
		t.Fatalf("Expected %d elements, got %d", len(want), len(got))
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("Element %d: expected %f, got %f", i, v, got[i])
		}
	}
}

// TestReadFrom_Errors verifies malformed streams are rejected with a message
// naming the stage that failed.
func TestReadFrom_Errors(t *testing.T) {
	backend := tensor.NewMockBackend()

	tests := []struct {
		name       string
		data       []byte
		wantSubstr string
	}{
		{"empty", nil, "failed to read magic bytes"},
		{"bad magic", []byte("XXXXXXXX"), "invalid magic bytes"},
		{"truncated version", append([]byte(MagicBytes), 0x01, 0x00), "failed to read version"},
		{"unsupported version", append([]byte(MagicBytes), 0x02, 0x00, 0x00, 0x00), "unsupported format version"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ReadFrom(bytes.NewReader(tt.data), backend)
			if err == nil {
				t.Fatalf("Expected error for %q, got nil", tt.name)
			}
			if got := err.Error(); !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("Expected error to mention %q, got: %v", tt.wantSubstr, got)
			}
		})
	}
}

// TestNewBornReaderFromBytes_Errors verifies the reader constructor surfaces the
// format sentinels for a corrupt header.
func TestNewBornReaderFromBytes_Errors(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr error  // sentinel to match with errors.Is, or nil
		wantSub string // substring to match when wantErr is nil
	}{
		{"empty", nil, nil, "failed to read magic bytes"},
		{"bad magic", []byte("XXXXXXXX"), ErrInvalidMagic, ""},
		{"unsupported version", append([]byte(MagicBytes), 0x09, 0x00, 0x00, 0x00), ErrUnsupportedVersion, ""},
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

// TestReader_TensorNotFound verifies the per-tensor lookups reject an absent
// name rather than returning zero-value data.
func TestReader_TensorNotFound(t *testing.T) {
	backend := tensor.NewMockBackend()

	reader, err := NewBornReaderFromBytes(writeTestBorn(t))
	if err != nil {
		t.Fatalf("Failed to open reader: %v", err)
	}
	defer reader.Close()

	if _, err := reader.TensorInfo("missing"); err == nil {
		t.Error("TensorInfo(missing) = nil error, want not-found")
	}
	if _, err := reader.ReadTensorData("missing"); err == nil {
		t.Error("ReadTensorData(missing) = nil error, want not-found")
	}
	if _, err := reader.LoadTensor("missing", backend); err == nil {
		t.Error("LoadTensor(missing) = nil error, want not-found")
	}
}

// TestReader_ClosedRejectsReads verifies every read path guards against use
// after Close.
func TestReader_ClosedRejectsReads(t *testing.T) {
	backend := tensor.NewMockBackend()

	reader, err := NewBornReaderFromBytes(writeTestBorn(t))
	if err != nil {
		t.Fatalf("Failed to open reader: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if _, err := reader.ReadTensorData("weight"); err == nil {
		t.Error("ReadTensorData after Close = nil error, want rejected")
	}
	if _, err := reader.LoadTensor("weight", backend); err == nil {
		t.Error("LoadTensor after Close = nil error, want rejected")
	}
	if _, err := reader.ReadStateDict(backend); err == nil {
		t.Error("ReadStateDict after Close = nil error, want rejected")
	}
}
