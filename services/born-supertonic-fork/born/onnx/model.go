//go:build !wasm

package onnx

import "github.com/born-ml/born/internal/tensor"

// Model represents a loaded ONNX model ready for inference.
//
// This interface hides the internal implementation and allows for:
//   - Easy mocking in tests
//   - Multiple implementations (e.g., optimized versions)
//   - Decoupling from internal package structure
//
// The model contains the computation graph, weights, and metadata
// from the original ONNX file. Use Forward or ForwardNamed to
// run inference.
type Model interface {
	// Forward runs inference with a single input tensor.
	// For models with multiple inputs, use ForwardNamed.
	//
	// Returns an error if the model does not have exactly one input
	// or one output. In such cases, use ForwardNamed instead.
	Forward(input *tensor.RawTensor) (*tensor.RawTensor, error)

	// ForwardNamed runs inference with named inputs.
	// Returns a map of output name to tensor.
	//
	// This method supports models with multiple inputs and outputs.
	// All input names from InputNames() must be provided.
	//
	// Example:
	//
	//	inputs := map[string]*tensor.RawTensor{
	//	    "input_ids": inputIDs,
	//	    "attention_mask": attentionMask,
	//	}
	//	outputs, err := model.ForwardNamed(inputs)
	//	if err != nil {
	//	    log.Fatal(err)
	//	}
	//	logits := outputs["logits"]
	ForwardNamed(inputs map[string]*tensor.RawTensor) (map[string]*tensor.RawTensor, error)

	// InputNames returns the names of model inputs.
	InputNames() []string

	// OutputNames returns the names of model outputs.
	OutputNames() []string

	// OpsetVersion returns the ONNX opset version used by the model.
	OpsetVersion() int64

	// Metadata returns model metadata as key-value pairs.
	//
	// Common metadata keys:
	//   - "producer_name": Framework that exported the model (e.g., "pytorch")
	//   - "producer_version": Version of the exporter
	//   - "domain": Domain of the model (usually "")
	//   - Custom keys from model.metadata_props
	Metadata() map[string]string
}
