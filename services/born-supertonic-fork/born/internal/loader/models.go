package loader

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/born-ml/born/internal/gguf"
	"github.com/born-ml/born/internal/tensor"
)

// ModelFormat represents the model weight format.
type ModelFormat int

// Supported model formats.
const (
	FormatUnknown ModelFormat = iota
	FormatSafeTensors
	FormatGGUF
)

// String returns the format name.
func (f ModelFormat) String() string {
	switch f {
	case FormatSafeTensors:
		return "SafeTensors"
	case FormatGGUF:
		return "GGUF"
	default:
		return "Unknown"
	}
}

// ModelReader provides a unified interface for loading model weights.
type ModelReader interface {
	// Close closes the underlying file.
	Close() error

	// Format returns the model format.
	Format() ModelFormat

	// Architecture returns the detected architecture (llama, mistral, deepseek).
	Architecture() string

	// Metadata returns model metadata.
	Metadata() map[string]interface{}

	// TensorNames returns all tensor names in the model.
	TensorNames() []string

	// LoadTensor loads a tensor by name with optional name mapping.
	LoadTensor(name string, backend tensor.Backend) (*tensor.RawTensor, error)

	// ReadTensorData reads raw tensor bytes (for custom conversion).
	ReadTensorData(name string) ([]byte, error)
}

// safeTensorsModel wraps SafeTensorsReader to implement ModelReader.
type safeTensorsModel struct {
	reader       *SafeTensorsReader
	architecture string
	mapper       WeightMapper
}

// Format returns FormatSafeTensors.
func (m *safeTensorsModel) Format() ModelFormat {
	return FormatSafeTensors
}

// Architecture returns the detected architecture.
func (m *safeTensorsModel) Architecture() string {
	return m.architecture
}

// Metadata returns model metadata.
func (m *safeTensorsModel) Metadata() map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m.reader.Metadata() {
		result[k] = v
	}
	return result
}

// TensorNames returns all tensor names.
func (m *safeTensorsModel) TensorNames() []string {
	return m.reader.TensorNames()
}

// LoadTensor loads a tensor with optional name mapping.
func (m *safeTensorsModel) LoadTensor(name string, backend tensor.Backend) (*tensor.RawTensor, error) {
	return m.reader.LoadTensor(name, backend)
}

// ReadTensorData reads raw tensor bytes.
func (m *safeTensorsModel) ReadTensorData(name string) ([]byte, error) {
	return m.reader.ReadTensorData(name)
}

// Close closes the reader.
func (m *safeTensorsModel) Close() error {
	return m.reader.Close()
}

// ggufModel wraps GGUFReader to implement ModelReader.
type ggufModel struct {
	reader       *GGUFReader
	architecture string
	mapper       WeightMapper
	// filePath is stored for on-demand dequantization via gguf.TensorConverter.
	filePath string
}

// Format returns FormatGGUF.
func (m *ggufModel) Format() ModelFormat {
	return FormatGGUF
}

// Architecture returns the detected architecture.
func (m *ggufModel) Architecture() string {
	return m.architecture
}

// Metadata returns model metadata.
func (m *ggufModel) Metadata() map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m.reader.Metadata() {
		result[k] = v
	}
	return result
}

// TensorNames returns all tensor names.
func (m *ggufModel) TensorNames() []string {
	return m.reader.TensorNames()
}

// LoadTensor loads a tensor with optional name mapping.
// Quantized GGUF tensors (Q4_0, Q4_K, Q8_0, F16, etc.) are dequantized
// transparently to float32 using the gguf.TensorConverter.
func (m *ggufModel) LoadTensor(name string, backend tensor.Backend) (*tensor.RawTensor, error) {
	info, err := m.reader.TensorInfo(name)
	if err != nil {
		return nil, err
	}

	// Fast path: F32 tensors can be copied directly without dequantization.
	if info.DType == GGUFDTypeF32 {
		data, err := m.reader.ReadTensorData(name)
		if err != nil {
			return nil, err
		}

		// GGUF stores dimensions in reversed order relative to Born convention.
		shape := make(tensor.Shape, len(info.Dims))
		for i, dim := range info.Dims {
			shape[len(shape)-1-i] = int(dim) //nolint:gosec // G115: safe, GGUF dimensions are small shape values
		}

		raw, err := tensor.NewRaw(shape, tensor.Float32, backend.Device())
		if err != nil {
			return nil, fmt.Errorf("failed to create tensor: %w", err)
		}
		copy(raw.Data(), data)
		return raw, nil
	}

	// Slow path: quantized or F16 tensors require full dequantization.
	// Parse the GGUF file via internal/gguf to obtain a TensorConverter that
	// handles all quantization formats (Q4_0, Q4_K, Q5_K, Q6_K, Q8_0, F16, …).
	if m.filePath == "" {
		return nil, fmt.Errorf("tensor %s: quantized dtype %v requires file path for dequantization", name, info.DType)
	}

	ggufFile, err := gguf.ParseFile(m.filePath)
	if err != nil {
		return nil, fmt.Errorf("tensor %s: parse gguf for dequantization: %w", name, err)
	}

	converter, err := gguf.NewTensorConverter(ggufFile)
	if err != nil {
		return nil, fmt.Errorf("tensor %s: create tensor converter: %w", name, err)
	}
	defer func() { _ = converter.Close() }()

	float32Data, shape, err := converter.Convert(name)
	if err != nil {
		return nil, fmt.Errorf("tensor %s: dequantize: %w", name, err)
	}

	// Convert []int shape to tensor.Shape.
	tShape := make(tensor.Shape, len(shape))
	copy(tShape, shape)

	raw, err := tensor.NewRaw(tShape, tensor.Float32, backend.Device())
	if err != nil {
		return nil, fmt.Errorf("tensor %s: create raw tensor: %w", name, err)
	}
	copy(raw.AsFloat32(), float32Data)

	return raw, nil
}

// ReadTensorData reads raw tensor bytes.
func (m *ggufModel) ReadTensorData(name string) ([]byte, error) {
	return m.reader.ReadTensorData(name)
}

// Close closes the reader.
func (m *ggufModel) Close() error {
	return m.reader.Close()
}

// OpenModel opens a model file and auto-detects the format.
// Supports .safetensors and .gguf files.
//
// Example:
//
//	model, err := loader.OpenModel("path/to/model.safetensors")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer model.Close()
//
//	fmt.Printf("Format: %s\n", model.Format())
//	fmt.Printf("Architecture: %s\n", model.Architecture())
func OpenModel(path string) (ModelReader, error) {
	// Detect format from extension
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".safetensors":
		return openSafeTensors(path)
	case ".gguf":
		return openGGUF(path)
	default:
		return nil, fmt.Errorf("unsupported file format: %s (expected .safetensors or .gguf)", ext)
	}
}

// openSafeTensors opens a SafeTensors file.
func openSafeTensors(path string) (ModelReader, error) {
	reader, err := NewSafeTensorsReader(path)
	if err != nil {
		return nil, err
	}

	// Detect architecture
	names := reader.TensorNames()
	arch := DetectArchitecture(names)
	mapper := GetMapper(arch)

	return &safeTensorsModel{
		reader:       reader,
		architecture: arch,
		mapper:       mapper,
	}, nil
}

// openGGUF opens a GGUF file.
func openGGUF(path string) (ModelReader, error) {
	reader, err := NewGGUFReader(path)
	if err != nil {
		return nil, err
	}

	// Try to detect architecture from metadata
	var arch string
	if archMetadata, ok := reader.Metadata()["general.architecture"].(string); ok {
		arch = archMetadata
	} else {
		// Fallback: detect from tensor names
		names := reader.TensorNames()
		arch = DetectArchitecture(names)
	}
	if arch == "" {
		arch = ArchitectureLLaMA // Default if detection failed
	}

	mapper := GetMapper(arch)

	return &ggufModel{
		reader:       reader,
		architecture: arch,
		mapper:       mapper,
		filePath:     path,
	}, nil
}
