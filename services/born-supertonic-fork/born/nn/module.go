// Copyright 2025 Born ML Framework. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package nn

import (
	"bytes"
	"io"

	"github.com/born-ml/born/internal/nn"
	"github.com/born-ml/born/internal/serialization"
	"github.com/born-ml/born/tensor"
)

// Module is the base interface for all neural network components.
//
// Every NN module must implement:
//   - Forward: Compute output from input
//   - Parameters: Return all trainable parameters
//   - StateDict: Export parameters for serialization
//   - LoadStateDict: Import parameters from serialization
//
// Modules can be composed to build complex architectures:
//
//	model := nn.NewSequential(
//	    nn.NewLinear(784, 128, backend),
//	    nn.NewReLU[Backend](),
//	    nn.NewLinear(128, 10, backend),
//	)
//
// Type parameter B must satisfy the tensor.Backend interface.
type Module[B tensor.Backend] interface {
	// Forward computes the output of the module given an input tensor.
	//
	// The input tensor should have the appropriate shape for this module.
	// For example, Linear expects [batch_size, in_features].
	//
	// Returns the output tensor with shape determined by the module type.
	Forward(input *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B]

	// Parameters returns all trainable parameters of this module.
	//
	// This includes weights, biases, and any nested module parameters.
	// Returns an empty slice for modules without trainable parameters
	// (e.g., activation functions).
	Parameters() []*Parameter[B]

	// StateDict returns a map of parameter names to raw tensors.
	//
	// This is used for serialization. The returned map contains all
	// trainable parameters with their names as keys.
	StateDict() map[string]*tensor.RawTensor

	// LoadStateDict loads parameters from a state dictionary.
	//
	// This is used for deserialization. The state dictionary should
	// contain parameter names as keys and RawTensors as values.
	//
	// Returns an error if a required parameter is missing or has wrong shape.
	LoadStateDict(stateDict map[string]*tensor.RawTensor) error
}

// Note: Internal implementations of Module automatically satisfy this interface
// because they have the same method signatures.

// Save saves a module to a .born file.
//
// This is a convenience function that exports the module's state dictionary
// and writes it to a file using the Born native format.
//
// Parameters:
//   - module: The module to save
//   - path: File path to write to
//   - modelType: Type name of the model (e.g., "Sequential", "Linear")
//   - metadata: Optional metadata (can be nil)
//
// Returns an error if saving fails.
//
// Example:
//
//	backend := cpu.New()
//	model := nn.NewLinear(784, 10, backend)
//	err := nn.Save(model, "model.born", "Linear", nil)
func Save[B tensor.Backend](module Module[B], path, modelType string, metadata map[string]string) error {
	return nn.Save(module, path, modelType, metadata)
}

// Load loads a module from a .born file.
//
// This is a convenience function that reads a state dictionary from a file
// and loads it into the provided module.
//
// Parameters:
//   - path: File path to read from
//   - backend: Backend to use for tensors
//   - module: The module to load into (will be modified)
//
// Returns the header and an error if loading fails.
//
// Example:
//
//	backend := cpu.New()
//	model := nn.NewLinear(784, 10, backend)
//	header, err := nn.Load("model.born", backend, model)
func Load[B tensor.Backend](path string, backend B, module Module[B]) (serialization.Header, error) {
	return nn.Load(path, backend, module)
}

// SaveTo serializes a module to an io.Writer in the Born native format.
//
// This is the streaming counterpart to Save: it exports the module's state
// dictionary and writes it to any io.Writer instead of a file, for streaming to
// HTTP responses, network connections, or in-memory buffers. The caller owns
// the writer's lifecycle.
//
// Parameters:
//   - w: The writer to encode into
//   - module: The module to serialize
//   - modelType: Type name of the model (e.g., "Sequential", "Linear")
//   - metadata: Optional metadata (can be nil)
//
// Returns an error if serialization fails.
func SaveTo[B tensor.Backend](w io.Writer, module Module[B], modelType string, metadata map[string]string) error {
	return serialization.WriteTo(w, module.StateDict(), modelType, metadata)
}

// SaveToBytes serializes a module to an in-memory .born byte slice.
//
// This is the in-memory counterpart to Save, for embedded assets or database
// blobs without touching disk. It is the save-side twin of LoadFromBytes and a
// thin wrapper over SaveTo.
//
// Parameters:
//   - module: The module to serialize
//   - modelType: Type name of the model (e.g., "Sequential", "Linear")
//   - metadata: Optional metadata (can be nil)
//
// Returns the encoded .born bytes and an error if serialization fails.
func SaveToBytes[B tensor.Backend](module Module[B], modelType string, metadata map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	if err := SaveTo(&buf, module, modelType, metadata); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// LoadFrom loads a module from an io.ReadSeeker in the Born native format.
//
// This is the streaming counterpart to Load: it reads a state dictionary from
// any seekable stream instead of a file and loads it into the provided module.
// Seeking is required because tensor payloads are read at their header offsets.
// The caller owns the reader's lifecycle.
//
// Parameters:
//   - r: The seekable stream to read from
//   - backend: Backend to use for tensors
//   - module: The module to load into (will be modified)
//
// Returns the header and an error if loading fails.
func LoadFrom[B tensor.Backend](r io.ReadSeeker, backend B, module Module[B]) (header serialization.Header, err error) {
	reader, err := serialization.NewBornReaderFromReadSeeker(r)
	if err != nil {
		return serialization.Header{}, err
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	stateDict, err := reader.ReadStateDict(backend)
	if err != nil {
		return serialization.Header{}, err
	}

	if loadErr := module.LoadStateDict(stateDict); loadErr != nil {
		return serialization.Header{}, loadErr
	}

	return reader.Header(), nil
}

// LoadFromBytes loads a module from an in-memory .born byte slice.
//
// This is the in-memory counterpart to Load, for embedded assets or database
// blobs without touching disk. It is a thin wrapper over LoadFrom.
//
// Parameters:
//   - data: The .born file contents as a byte slice
//   - backend: Backend to use for tensors
//   - module: The module to load into (will be modified)
//
// Returns the header and an error if loading fails.
func LoadFromBytes[B tensor.Backend](data []byte, backend B, module Module[B]) (serialization.Header, error) {
	return LoadFrom(bytes.NewReader(data), backend, module)
}
