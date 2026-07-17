package serialization

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// errInjected is the sentinel returned by the failing writer and closer stubs.
var errInjected = errors.New("injected failure")

// failWriter is an io.Writer that discards every write but returns errInjected
// on its nth Write call, counting from 1. A failOn of 0 never fails. It lets a
// test drive the writer to any single stage of the .born layout and assert the
// resulting error is surfaced with context.
type failWriter struct {
	failOn int
	calls  int
}

func (fw *failWriter) Write(p []byte) (int, error) {
	fw.calls++
	if fw.failOn != 0 && fw.calls == fw.failOn {
		return 0, errInjected
	}
	return len(p), nil
}

// countWriter discards every write and counts the calls, so a test can learn
// how many Write calls a successful run makes and target the final one.
type countWriter struct {
	calls int
}

func (cw *countWriter) Write(p []byte) (int, error) {
	cw.calls++
	return len(p), nil
}

// failCloser is an io.Closer whose Close reports errInjected.
type failCloser struct{}

func (failCloser) Close() error { return errInjected }

// failSeeker is an io.ReadSeeker whose Seek returns errInjected on its nth call,
// counting from 1. It drives the ReadSeeker reader constructor to each of its
// size-derivation seeks.
type failSeeker struct {
	failOn int
	calls  int
}

func (fs *failSeeker) Read([]byte) (int, error) { return 0, errInjected }

func (fs *failSeeker) Seek(int64, int) (int64, error) {
	fs.calls++
	if fs.calls == fs.failOn {
		return 0, errInjected
	}
	return 0, nil
}

// newTestStateDict builds a single-tensor state dict for writer tests.
func newTestStateDict(t *testing.T) map[string]*tensor.RawTensor {
	t.Helper()

	backend := tensor.NewMockBackend()
	raw, err := tensor.NewRaw(tensor.Shape{2, 2}, tensor.Float32, backend.Device())
	if err != nil {
		t.Fatalf("Failed to create tensor: %v", err)
	}
	copy(raw.AsFloat32(), []float32{1.0, 2.0, 3.0, 4.0})

	return map[string]*tensor.RawTensor{"weight": raw}
}

// TestWriteStateDict_WriteError verifies that a failure from the underlying
// writer surfaces from WriteStateDict, wrapped with the stage that failed. The
// v1 write path issues one Write per stage in a fixed order, so the nth Write
// call maps to a known stage regardless of payload sizes.
func TestWriteStateDict_WriteError(t *testing.T) {
	tests := []struct {
		name       string
		failOn     int
		wantSubstr string
	}{
		{"magic bytes", 1, "failed to write magic bytes"},
		{"version", 2, "failed to write version"},
		{"flags", 3, "failed to write flags"},
		{"header size", 4, "failed to write header size"},
		{"header JSON", 5, "failed to write header:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stateDict := newTestStateDict(t)
			fw := &failWriter{failOn: tt.failOn}

			err := newBornWriterToWriter(fw).WriteStateDict(stateDict, "TestModel", nil)
			if err == nil {
				t.Fatalf("Expected error at stage %q, got nil", tt.name)
			}
			if !errors.Is(err, errInjected) {
				t.Errorf("Expected error to wrap errInjected, got: %v", err)
			}
			if got := err.Error(); !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("Expected error to mention %q, got: %v", tt.wantSubstr, got)
			}
		})
	}
}

// TestWriteStateDict_TensorDataError verifies that a failure while writing the
// tensor data section surfaces named by tensor. The data write is the final
// Write of a successful run, so a count of that run pinpoints it.
func TestWriteStateDict_TensorDataError(t *testing.T) {
	cw := &countWriter{}
	if err := newBornWriterToWriter(cw).WriteStateDict(newTestStateDict(t), "TestModel", nil); err != nil {
		t.Fatalf("Reference write failed: %v", err)
	}

	fw := &failWriter{failOn: cw.calls}
	err := newBornWriterToWriter(fw).WriteStateDict(newTestStateDict(t), "TestModel", nil)
	if err == nil {
		t.Fatal("Expected error writing tensor data, got nil")
	}
	if !errors.Is(err, errInjected) {
		t.Errorf("Expected error to wrap errInjected, got: %v", err)
	}
	if got := err.Error(); !strings.Contains(got, "failed to write tensor weight") {
		t.Errorf("Expected error to name the tensor, got: %v", got)
	}
}

// TestBornWriter_CloseSurfacesCloserError verifies that Close reports the
// closer's error, that a second Close is a no-op, and that writing after Close
// is rejected.
func TestBornWriter_CloseSurfacesCloserError(t *testing.T) {
	w := newBornWriterToWriter(&bytes.Buffer{})
	w.closer = failCloser{}

	if err := w.Close(); !errors.Is(err, errInjected) {
		t.Errorf("Expected Close to surface errInjected, got: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("Expected second Close to be a no-op, got: %v", err)
	}
	if err := w.WriteStateDict(newTestStateDict(t), "TestModel", nil); err == nil {
		t.Error("Expected write after Close to be rejected, got nil")
	}
}

// TestBornWriter_CloseNoCloser verifies that a writer with no closer, such as
// one built directly over an in-memory buffer, closes without error.
func TestBornWriter_CloseNoCloser(t *testing.T) {
	if err := newBornWriterToWriter(&bytes.Buffer{}).Close(); err != nil {
		t.Errorf("Expected no-op Close, got: %v", err)
	}
}

// TestBornWriter_BufferRoundTrip verifies the filesystem-free path the io.Writer
// inversion unlocks: write a state dict into an in-memory buffer and read it
// back through the byte reader, asserting names, shapes, dtypes and bytes.
func TestBornWriter_BufferRoundTrip(t *testing.T) {
	backend := tensor.NewMockBackend()
	stateDict := newTestStateDict(t)

	var buf bytes.Buffer
	if err := newBornWriterToWriter(&buf).WriteStateDict(stateDict, "TestModel", map[string]string{"k": "v"}); err != nil {
		t.Fatalf("Failed to write state dict: %v", err)
	}

	reader, err := NewBornReaderFromBytes(buf.Bytes())
	if err != nil {
		t.Fatalf("Failed to open buffer: %v", err)
	}
	defer reader.Close()

	loaded, err := reader.ReadStateDict(backend)
	if err != nil {
		t.Fatalf("Failed to read state dict: %v", err)
	}

	raw, ok := loaded["weight"]
	if !ok {
		t.Fatal("Tensor 'weight' not found")
	}
	if got := raw.Shape(); !got.Equal(tensor.Shape{2, 2}) {
		t.Errorf("Expected shape [2 2], got %v", got)
	}
	if got := raw.DType(); got != tensor.Float32 {
		t.Errorf("Expected dtype Float32, got %v", got)
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

// TestNewBornReaderFromReadSeeker_SeekError verifies that a Seek failure while
// deriving the stream size surfaces from the constructor, wrapped with the seek
// that failed.
func TestNewBornReaderFromReadSeeker_SeekError(t *testing.T) {
	tests := []struct {
		name       string
		failOn     int
		wantSubstr string
	}{
		{"size determination", 1, "failed to determine size"},
		{"seek to start", 2, "failed to seek to start"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewBornReaderFromReadSeeker(&failSeeker{failOn: tt.failOn})
			if err == nil {
				t.Fatalf("Expected error at %q, got nil", tt.name)
			}
			if !errors.Is(err, errInjected) {
				t.Errorf("Expected error to wrap errInjected, got: %v", err)
			}
			if got := err.Error(); !strings.Contains(got, tt.wantSubstr) {
				t.Errorf("Expected error to mention %q, got: %v", tt.wantSubstr, got)
			}
		})
	}
}

// TestWriteTo_ReadSeekerRoundTrip exercises the public streaming pair: WriteTo
// encodes a state dict into an in-memory buffer and NewBornReaderFromReadSeeker
// reads it back through a seekable byte reader, deriving the size from the
// stream. These are the symmetric counterparts the io.Writer/io.ReadSeeker
// inversion put in place.
func TestWriteTo_ReadSeekerRoundTrip(t *testing.T) {
	backend := tensor.NewMockBackend()
	stateDict := newTestStateDict(t)

	var buf bytes.Buffer
	if err := WriteTo(&buf, stateDict, "TestModel", map[string]string{"k": "v"}); err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}

	reader, err := NewBornReaderFromReadSeeker(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("NewBornReaderFromReadSeeker failed: %v", err)
	}
	defer reader.Close()

	loaded, err := reader.ReadStateDict(backend)
	if err != nil {
		t.Fatalf("Failed to read state dict: %v", err)
	}

	raw, ok := loaded["weight"]
	if !ok {
		t.Fatal("Tensor 'weight' not found")
	}
	if got := raw.Shape(); !got.Equal(tensor.Shape{2, 2}) {
		t.Errorf("Expected shape [2 2], got %v", got)
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
