package gguf

import (
	"fmt"
)

// ConvertedTensor holds converted tensor data.
type ConvertedTensor struct {
	Name         string
	Shape        []int
	Data         []float32
	OriginalType GGMLType // Original GGUF type (for debugging).
}

// TensorConverter converts GGUF tensors to Born tensor format.
type TensorConverter struct {
	file   *File
	reader *TensorReader
}

// NewTensorConverter creates a converter for the given GGUF file.
func NewTensorConverter(file *File) (*TensorConverter, error) {
	reader, err := NewTensorReader(file)
	if err != nil {
		return nil, fmt.Errorf("create tensor reader: %w", err)
	}

	return &TensorConverter{
		file:   file,
		reader: reader,
	}, nil
}

// Convert loads and dequantizes a single tensor.
// Returns raw float32 data and shape.
//
// GGUF dimensions are stored in reverse order compared to Born's convention:
// GGUF [dim0, dim1, dim2] → Born [dim2, dim1, dim0].
func (c *TensorConverter) Convert(name string) ([]float32, []int, error) {
	// Find tensor info.
	tensor := c.file.GetTensor(name)
	if tensor == nil {
		return nil, nil, fmt.Errorf("tensor not found: %s", name)
	}

	// Read raw quantized data.
	rawData, err := c.reader.ReadTensor(name)
	if err != nil {
		return nil, nil, fmt.Errorf("read tensor data: %w", err)
	}

	// Dequantize to float32.
	numElements := tensor.NumElements()
	data, err := Dequantize(rawData, tensor.Type, int(numElements)) //nolint:gosec // G115: integer overflow conversion uint64 -> int
	if err != nil {
		return nil, nil, fmt.Errorf("dequantize tensor: %w", err)
	}

	// Convert GGUF dimensions to Born shape (reverse order).
	shape := make([]int, len(tensor.Dimensions))
	for i, dim := range tensor.Dimensions {
		shape[len(tensor.Dimensions)-1-i] = int(dim) //nolint:gosec // G115: safe, GGUF dimensions are small shape values
	}

	return data, shape, nil
}

// ConvertAll loads and dequantizes all tensors.
func (c *TensorConverter) ConvertAll() (map[string]ConvertedTensor, error) {
	result := make(map[string]ConvertedTensor)

	for i := range c.file.TensorInfo {
		tensor := &c.file.TensorInfo[i]

		data, shape, err := c.Convert(tensor.Name)
		if err != nil {
			return nil, fmt.Errorf("convert tensor %s: %w", tensor.Name, err)
		}

		result[tensor.Name] = ConvertedTensor{
			Name:         tensor.Name,
			Shape:        shape,
			Data:         data,
			OriginalType: tensor.Type,
		}
	}

	return result, nil
}

// Close releases resources.
func (c *TensorConverter) Close() error {
	if c.reader != nil {
		return c.reader.Close()
	}
	return nil
}
