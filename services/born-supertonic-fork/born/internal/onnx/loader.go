//go:build !wasm

package onnx

import (
	"fmt"

	"github.com/born-ml/born/internal/onnx/operators"
	"github.com/born-ml/born/internal/tensor"
)

// LoadOptions configures model loading behavior.
type LoadOptions struct {
	// StrictMode fails on unsupported operators (default: false = skip with warning).
	StrictMode bool

	// CustomOps provides custom operator handlers.
	CustomOps map[string]operators.OpHandler
}

// DefaultLoadOptions returns default loading options.
func DefaultLoadOptions() LoadOptions {
	return LoadOptions{
		StrictMode: false,
		CustomOps:  nil,
	}
}

// Load loads an ONNX model from file and prepares it for inference.
// The backend is used for tensor operations during inference.
//
// Example:
//
//	model, err := onnx.Load("resnet50.onnx", backend)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	output, err := model.Forward(input)
func Load(path string, backend tensor.Backend, opts ...LoadOptions) (*Model, error) {
	// Parse options
	opt := DefaultLoadOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}

	// Parse ONNX file
	proto, err := ParseFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ONNX file: %w", err)
	}

	return LoadFromProto(proto, backend, opt)
}

// LoadFromBytes loads an ONNX model from bytes.
func LoadFromBytes(data []byte, backend tensor.Backend, opts ...LoadOptions) (*Model, error) {
	// Parse options
	opt := DefaultLoadOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}

	// Parse ONNX data
	proto, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ONNX data: %w", err)
	}

	return LoadFromProto(proto, backend, opt)
}

// LoadFromProto loads a model from parsed ModelProto.
func LoadFromProto(proto *ModelProto, backend tensor.Backend, opt LoadOptions) (*Model, error) {
	// Create operator registry
	registry := operators.NewRegistry()

	// Add custom operators
	for opType, handler := range opt.CustomOps {
		registry.Register(opType, handler)
	}

	// Validate operators if strict mode
	if opt.StrictMode {
		if err := validateOperators(proto.Graph, registry); err != nil {
			return nil, err
		}
	}

	// Create model
	model := &Model{
		proto:    proto,
		registry: registry,
		backend:  backend,
	}

	// Compile model
	if err := model.compile(); err != nil {
		return nil, fmt.Errorf("failed to compile model: %w", err)
	}

	return model, nil
}

// validateOperators checks that all operators are supported.
func validateOperators(graph *GraphProto, registry *operators.Registry) error {
	if graph == nil {
		return fmt.Errorf("model has no graph")
	}
	if registry == nil {
		return fmt.Errorf("registry is nil")
	}

	unsupported := make([]string, 0)
	for i := range graph.Nodes {
		if _, ok := registry.Get(graph.Nodes[i].OpType); !ok {
			unsupported = append(unsupported, graph.Nodes[i].OpType)
		}
	}

	if len(unsupported) > 0 {
		return fmt.Errorf("unsupported operators: %v", unsupported)
	}

	return nil
}

// ModelInfo contains basic information about an ONNX model without fully loading it.
type ModelInfo struct {
	IRVersion       int64
	OpsetVersion    int64
	ProducerName    string
	ProducerVersion string
	InputNames      []string
	OutputNames     []string
	NodeCount       int
	WeightCount     int
}

// GetModelInfo extracts basic info from an ONNX file.
func GetModelInfo(path string) (*ModelInfo, error) {
	proto, err := ParseFile(path)
	if err != nil {
		return nil, err
	}

	info := &ModelInfo{
		IRVersion:       proto.IRVersion,
		ProducerName:    proto.ProducerName,
		ProducerVersion: proto.ProducerVersion,
	}

	// Get opset version
	for _, opset := range proto.OpsetImport {
		if opset.Domain == "" || opset.Domain == "ai.onnx" {
			info.OpsetVersion = opset.Version
			break
		}
	}

	if proto.Graph != nil {
		// Get inputs (excluding initializers)
		initNames := make(map[string]bool)
		for i := range proto.Graph.Initializers {
			initNames[proto.Graph.Initializers[i].Name] = true
		}
		for i := range proto.Graph.Inputs {
			if !initNames[proto.Graph.Inputs[i].Name] {
				info.InputNames = append(info.InputNames, proto.Graph.Inputs[i].Name)
			}
		}

		// Get outputs
		for _, output := range proto.Graph.Outputs {
			info.OutputNames = append(info.OutputNames, output.Name)
		}

		info.NodeCount = len(proto.Graph.Nodes)
		info.WeightCount = len(proto.Graph.Initializers)
	}

	return info, nil
}

// ListSupportedOps returns all supported ONNX operators.
func ListSupportedOps() []string {
	registry := operators.NewRegistry()
	return registry.SupportedOps()
}
