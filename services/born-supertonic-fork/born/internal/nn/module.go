// Package nn implements neural network modules for the Born ML Framework.
//
// This package provides building blocks for constructing neural networks:
//   - Module interface: Base interface for all NN components
//   - Parameter: Trainable parameters with gradient tracking
//   - Linear: Fully connected layer
//   - Activations: ReLU, Sigmoid, Tanh
//   - Loss functions: MSE, CrossEntropy
//   - Sequential: Container for stacking layers
//
// Design inspired by PyTorch's nn.Module but adapted for Go generics.
package nn

import (
	"fmt"

	"github.com/born-ml/born/internal/serialization"
	"github.com/born-ml/born/internal/tensor"
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
//	model := nn.Sequential[Backend](
//	    nn.NewLinear(784, 128, backend),
//	    nn.NewReLU(),
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
	//
	// Returns a map from parameter name to RawTensor.
	StateDict() map[string]*tensor.RawTensor

	// LoadStateDict loads parameters from a state dictionary.
	//
	// This is used for deserialization. The state dictionary should
	// contain parameter names as keys and RawTensors as values.
	//
	// Parameters:
	//   - stateDict: Map from parameter name to RawTensor
	//
	// Returns an error if:
	//   - A required parameter is missing
	//   - A parameter has the wrong shape or dtype
	LoadStateDict(stateDict map[string]*tensor.RawTensor) error
}

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
func Save[B tensor.Backend](module Module[B], path, modelType string, metadata map[string]string) (err error) {
	// Get state dictionary
	stateDict := module.StateDict()

	// Create writer
	writer, err := serialization.NewBornWriter(path)
	if err != nil {
		return fmt.Errorf("failed to create writer: %w", err)
	}
	defer func() {
		if closeErr := writer.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	// Write state dictionary
	if writeErr := writer.WriteStateDict(stateDict, modelType, metadata); writeErr != nil {
		return fmt.Errorf("failed to write state dict: %w", writeErr)
	}

	return nil
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
func Load[B tensor.Backend](path string, backend B, module Module[B]) (header serialization.Header, err error) {
	// Create reader
	reader, err := serialization.NewBornReader(path)
	if err != nil {
		return serialization.Header{}, fmt.Errorf("failed to create reader: %w", err)
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	// Read state dictionary
	stateDict, err := reader.ReadStateDict(backend)
	if err != nil {
		return serialization.Header{}, fmt.Errorf("failed to read state dict: %w", err)
	}

	// Load into module
	if loadErr := module.LoadStateDict(stateDict); loadErr != nil {
		return serialization.Header{}, fmt.Errorf("failed to load state dict: %w", loadErr)
	}

	return reader.Header(), nil
}
