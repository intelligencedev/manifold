// Package optim implements optimization algorithms for training neural networks.
//
// This package provides:
//   - Optimizer interface: Base interface for all optimizers
//   - SGD: Stochastic Gradient Descent with momentum
//   - Adam: Adaptive Moment Estimation
//
// Design inspired by PyTorch's torch.optim but adapted for Go with type safety.
//
// Example usage:
//
//	// Create optimizer
//	optimizer := optim.NewAdam(model.Parameters(), optim.AdamConfig{
//	    LR: 0.001,
//	})
//
//	// Training loop
//	for epoch := range epochs {
//	    loss := computeLoss(model, data)
//
//	    // Compute gradients
//	    backend.Tape().StartRecording()
//	    output := model.Forward(input)
//	    loss := lossFunc.Forward(output, targets)
//	    grads := autodiff.Backward(loss, backend)
//
//	    // Update parameters
//	    optimizer.Step(grads)
//	    optimizer.ZeroGrad()
//	}
package optim

import (
	"github.com/born-ml/born/internal/nn"
	"github.com/born-ml/born/internal/tensor"
)

// Optimizer is the base interface for all optimization algorithms.
//
// Optimizers update model parameters based on computed gradients to
// minimize the loss function during training.
//
// All optimizers must implement:
//   - Step: Apply gradient updates to parameters
//   - ZeroGrad: Clear gradients before next iteration
//   - GetLR: Get current learning rate (for monitoring/scheduling)
//   - StateDict: Export optimizer state for checkpoints
//   - LoadStateDict: Import optimizer state from checkpoints
type Optimizer interface {
	// Step applies gradient updates to all parameters.
	//
	// Takes a gradient map from Backward() and updates parameters in-place.
	// The gradient map should contain RawTensor -> gradient mapping.
	//
	// Example:
	//   grads := autodiff.Backward(loss, backend)
	//   optimizer.Step(grads)
	Step(grads map[*tensor.RawTensor]*tensor.RawTensor)

	// ZeroGrad clears all parameter gradients.
	//
	// This should be called before each backward pass to prevent
	// gradient accumulation from previous iterations.
	//
	// Example:
	//   optimizer.ZeroGrad()
	//   loss := model.Forward(...)
	//   grads := autodiff.Backward(loss, backend)
	ZeroGrad()

	// GetLR returns the current learning rate.
	//
	// Useful for monitoring and learning rate scheduling.
	GetLR() float32

	// StateDict returns the optimizer state for serialization.
	//
	// This includes optimizer-specific state like momentum buffers (SGD)
	// or moment estimates (Adam). Used for checkpoint saving.
	//
	// Returns a map from state name to RawTensor.
	// State names follow the pattern: "{state_type}.{param_index}"
	// For example: "velocity.0", "m.0", "v.0"
	StateDict() map[string]*tensor.RawTensor

	// LoadStateDict loads optimizer state from serialization.
	//
	// Restores optimizer-specific state from a checkpoint. The state
	// dictionary should match the structure returned by StateDict().
	//
	// Parameters:
	//   - stateDict: Map from state name to RawTensor
	//
	// Returns an error if the state dictionary is invalid.
	LoadStateDict(stateDict map[string]*tensor.RawTensor) error
}

// Config is the base configuration for all optimizers.
type Config struct {
	LR float32 // Learning rate
}

// CacheInvalidator is an optional interface that backends implement when they
// maintain an input-buffer cache keyed by *RawTensor identity.
//
// After an optimizer step the weight RawTensors stored in Parameters are
// replaced by new ones (via SetTensor). The old *RawTensor pointers no longer
// represent the current weights; any backend cache entry under those keys is
// stale. Calling ClearInputBufferCache() forces the next forward pass to
// re-upload the freshly computed weights from the new RawTensors.
//
// Optimizers call this via a type-assertion at the end of Step() — backends
// that do not maintain such a cache simply don't implement the interface.
type CacheInvalidator interface {
	ClearInputBufferCache()
}

// invalidateCacheIfNeeded type-asserts backend to CacheInvalidator and calls
// ClearInputBufferCache() if the backend supports it.
func invalidateCacheIfNeeded(backend any) {
	if ci, ok := backend.(CacheInvalidator); ok {
		ci.ClearInputBufferCache()
	}
}

// getGradient safely retrieves gradient for a parameter.
//
// Returns nil if no gradient is found (parameter wasn't part of computation graph).
func getGradient[B tensor.Backend](param *nn.Parameter[B], grads map[*tensor.RawTensor]*tensor.RawTensor) *tensor.RawTensor {
	if param == nil {
		return nil
	}
	return grads[param.Tensor().Raw()]
}
