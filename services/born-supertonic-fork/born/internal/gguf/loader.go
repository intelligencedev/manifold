package gguf

import (
	"fmt"
	"io"
	"os"
)

// LoadTensorData reads the raw data of a tensor from the file.
// Returns the tensor data as a byte slice.
func LoadTensorData(file *File, tensorName string) ([]byte, error) {
	// Find the tensor
	tensor := file.GetTensor(tensorName)
	if tensor == nil {
		return nil, fmt.Errorf("tensor not found: %s", tensorName)
	}

	// Open the file
	if file.FilePath == "" {
		return nil, fmt.Errorf("file path not set (file not loaded via ParseFile)")
	}

	f, err := os.Open(file.FilePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() {
		_ = f.Close() // Ignore close error.
	}()

	// Seek to tensor data
	offset := file.TensorDataOffset + int64(tensor.Offset) //nolint:gosec // G115: integer overflow conversion uint64 -> int64
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek to tensor data: %w", err)
	}

	// Read tensor data
	size := tensor.Size()
	data := make([]byte, size)
	if _, err := io.ReadFull(f, data); err != nil {
		return nil, fmt.Errorf("read tensor data: %w", err)
	}

	return data, nil
}

// LoadAllTensors reads all tensors from the file.
// Returns a map of tensor name to raw data.
func LoadAllTensors(file *File) (map[string][]byte, error) {
	if file.FilePath == "" {
		return nil, fmt.Errorf("file path not set (file not loaded via ParseFile)")
	}

	f, err := os.Open(file.FilePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() {
		_ = f.Close() // Ignore close error.
	}()

	tensors := make(map[string][]byte)
	for i := range file.TensorInfo {
		tensor := &file.TensorInfo[i]

		// Seek to tensor data
		offset := file.TensorDataOffset + int64(tensor.Offset) //nolint:gosec // G115: integer overflow conversion uint64 -> int64
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return nil, fmt.Errorf("seek to tensor %s: %w", tensor.Name, err)
		}

		// Read tensor data
		size := tensor.Size()
		data := make([]byte, size)
		if _, err := io.ReadFull(f, data); err != nil {
			return nil, fmt.Errorf("read tensor %s: %w", tensor.Name, err)
		}

		tensors[tensor.Name] = data
	}

	return tensors, nil
}

// TensorReader provides streaming access to tensor data for large models.
type TensorReader struct {
	file   *File
	reader io.ReadSeeker
}

// NewTensorReader creates a new tensor reader for the given file.
// The caller is responsible for closing the reader when done.
func NewTensorReader(file *File) (*TensorReader, error) {
	if file.FilePath == "" {
		return nil, fmt.Errorf("file path not set (file not loaded via ParseFile)")
	}

	f, err := os.Open(file.FilePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}

	return &TensorReader{
		file:   file,
		reader: f,
	}, nil
}

// ReadTensor reads the raw data of a tensor by name.
func (r *TensorReader) ReadTensor(name string) ([]byte, error) {
	// Find the tensor
	tensor := r.file.GetTensor(name)
	if tensor == nil {
		return nil, fmt.Errorf("tensor not found: %s", name)
	}

	// Seek to tensor data
	offset := r.file.TensorDataOffset + int64(tensor.Offset) //nolint:gosec // G115: integer overflow conversion uint64 -> int64
	if _, err := r.reader.Seek(offset, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek to tensor data: %w", err)
	}

	// Read tensor data
	size := tensor.Size()
	data := make([]byte, size)
	if _, err := io.ReadFull(r.reader, data); err != nil {
		return nil, fmt.Errorf("read tensor data: %w", err)
	}

	return data, nil
}

// ReadTensorInto reads the raw data of a tensor into the provided buffer.
// The buffer must be large enough to hold the tensor data.
func (r *TensorReader) ReadTensorInto(name string, dst []byte) error {
	// Find the tensor
	tensor := r.file.GetTensor(name)
	if tensor == nil {
		return fmt.Errorf("tensor not found: %s", name)
	}

	// Check buffer size
	size := tensor.Size()
	if uint64(len(dst)) < size {
		return fmt.Errorf("buffer too small: need %d bytes, got %d", size, len(dst))
	}

	// Seek to tensor data
	offset := r.file.TensorDataOffset + int64(tensor.Offset) //nolint:gosec // G115: integer overflow conversion uint64 -> int64
	if _, err := r.reader.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("seek to tensor data: %w", err)
	}

	// Read into buffer
	if _, err := io.ReadFull(r.reader, dst[:size]); err != nil {
		return fmt.Errorf("read tensor data: %w", err)
	}

	return nil
}

// Close closes the underlying file.
func (r *TensorReader) Close() error {
	if closer, ok := r.reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}
