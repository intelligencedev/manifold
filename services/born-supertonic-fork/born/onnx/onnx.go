//go:build !wasm

// Package onnx provides ONNX model import functionality for Born ML framework.
//
// This package enables loading and running inference on ONNX (Open Neural Network Exchange)
// models exported from PyTorch, TensorFlow, and other ML frameworks.
//
// # Supported Features
//
//   - ONNX format parsing (protobuf-based)
//   - 30+ standard ONNX operators
//   - Opset versions 1-21
//   - Float32 tensor operations
//   - Named input/output support
//
// # Example Usage
//
//	import (
//	    "github.com/born-ml/born/onnx"
//	    "github.com/born-ml/born/backend/cpu"
//	    "github.com/born-ml/born/tensor"
//	)
//
//	// Load ONNX model
//	backend := cpu.New()
//	model, err := onnx.Load("model.onnx", backend)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Create input tensor
//	input := tensor.FromSlice([]float32{1.0, 2.0, 3.0}, tensor.Shape{1, 3}, backend)
//
//	// Run inference
//	output, err := model.Forward(input.Raw())
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// # Supported Operators
//
// The following ONNX operators are supported:
//
//   - Arithmetic: Add, Sub, Mul, Div, Pow, Sqrt, Exp, Log, Sum, Erf
//   - Logical: Not, And, Or, Xor
//   - Comparison: Equal, Greater, GreaterOrEqual, Less, LessOrEqual
//   - Activation: Relu, LeakyRelu, PRelu, Gelu, Silu, Sigmoid, Tanh, Softmax, LogSoftmax
//   - Matrix: MatMul, Gemm, Transpose, Flatten
//   - Shape: Reshape, Squeeze, Unsqueeze, Concat, Split, Slice, Gather, Expand
//   - Normalization: LayerNormalization
//   - Reduction: ReduceMean, ReduceMax, ReduceMin
//   - Other: Identity, Dropout, Constant, Cast, ConstantOfShape, Shape, Size, Where, Clip
//
// Use [ListSupportedOps] to get the complete list of supported operators.
package onnx

import (
	internalonnx "github.com/born-ml/born/internal/onnx"
	"github.com/born-ml/born/internal/tensor"
)

// LoadOptions configures ONNX model loading behavior.
type LoadOptions = internalonnx.LoadOptions

// DefaultLoadOptions returns the default options for loading ONNX models.
//
// Default configuration:
//   - Strict mode: enabled (fails on unsupported operators)
//   - Optimization: enabled
func DefaultLoadOptions() LoadOptions {
	return internalonnx.DefaultLoadOptions()
}

// Load loads an ONNX model from a file path.
//
// The function parses the ONNX protobuf format, validates operators,
// and compiles the computation graph for efficient inference.
//
// Returns a Model interface that can be used for inference.
// The actual implementation is hidden in the internal package.
//
// Example:
//
//	backend := cpu.New()
//	model, err := onnx.Load("resnet18.onnx", backend)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Get model info
//	fmt.Println("Inputs:", model.InputNames())
//	fmt.Println("Outputs:", model.OutputNames())
//	fmt.Println("Opset:", model.OpsetVersion())
//
// For custom loading options, pass LoadOptions:
//
//	opts := onnx.DefaultLoadOptions()
//	opts.Strict = false // Allow unsupported ops (will skip them)
//	model, err := onnx.Load("model.onnx", backend, opts)
func Load(path string, backend tensor.Backend, opts ...LoadOptions) (Model, error) {
	return internalonnx.Load(path, backend, opts...)
}

// LoadFromBytes loads an ONNX model from raw bytes.
//
// This is useful when the model is embedded in the binary or loaded
// from a network source.
//
// Returns a Model interface that can be used for inference.
//
// Example:
//
//	modelBytes, _ := os.ReadFile("model.onnx")
//	model, err := onnx.LoadFromBytes(modelBytes, backend)
func LoadFromBytes(data []byte, backend tensor.Backend, opts ...LoadOptions) (Model, error) {
	return internalonnx.LoadFromBytes(data, backend, opts...)
}

// ModelInfo contains metadata about an ONNX model without loading weights.
//
// Use [GetModelInfo] to quickly inspect a model file before loading.
type ModelInfo = internalonnx.ModelInfo

// GetModelInfo extracts metadata from an ONNX file without loading the full model.
//
// This is useful for inspecting model structure, inputs/outputs, and
// operator requirements before committing to a full load.
//
// Example:
//
//	info, err := onnx.GetModelInfo("model.onnx")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	fmt.Printf("Producer: %s\n", info.ProducerName)
//	fmt.Printf("Opset: %d\n", info.OpsetVersion)
//	fmt.Printf("Inputs: %v\n", info.InputNames)
//	fmt.Printf("Outputs: %v\n", info.OutputNames)
//	fmt.Printf("Operators: %v\n", info.Operators)
func GetModelInfo(path string) (*ModelInfo, error) {
	return internalonnx.GetModelInfo(path)
}

// ListSupportedOps returns a list of all ONNX operators supported by Born.
//
// Example:
//
//	ops := onnx.ListSupportedOps()
//	for _, op := range ops {
//	    fmt.Println(op)
//	}
func ListSupportedOps() []string {
	return internalonnx.ListSupportedOps()
}
