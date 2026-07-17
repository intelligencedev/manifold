package optim

import (
	"fmt"

	"github.com/born-ml/born/internal/nn"
	"github.com/born-ml/born/internal/tensor"
)

// SGD implements Stochastic Gradient Descent optimizer with optional momentum
// and L2 weight decay.
//
// Update rule without momentum:
//
//	param = param * (1 - lr * weightDecay) - lr * gradient
//
// Update rule with momentum:
//
//	velocity = momentum * velocity + gradient
//	param    = param * (1 - lr * weightDecay) - lr * velocity
//
// Weight decay is applied as a tensor-op (MulScalar) — no CPU loop.
// Momentum helps accelerate SGD in relevant directions and dampens oscillations.
//
// All parameter updates are performed as pure tensor operations — no data is
// transferred to CPU during Step(). This keeps the hot path entirely on the
// accelerator when using a GPU backend.
//
// Example:
//
//	optimizer := optim.NewSGD(model.Parameters(), optim.SGDConfig{
//	    LR:          0.01,
//	    Momentum:    0.9,
//	    WeightDecay: 1e-4,
//	})
//
//	for epoch := range epochs {
//	    loss := train_step(model, batch)
//	    grads := autodiff.Backward(loss, backend)
//	    optimizer.Step(grads)
//	    optimizer.ZeroGrad()
//	}
type SGD[B tensor.Backend] struct {
	params      []*nn.Parameter[B]
	lr          float32
	momentum    float32
	weightDecay float32
	velocities  map[*nn.Parameter[B]]*tensor.Tensor[float32, B]
	backend     B
}

// SGDConfig holds configuration for SGD optimizer.
type SGDConfig struct {
	LR          float32 // Learning rate (default: 0.01)
	Momentum    float32 // Momentum factor (default: 0.0, range: [0, 1))
	WeightDecay float32 // L2 weight-decay coefficient (default: 0.0)
}

// NewSGD creates a new SGD optimizer.
//
// Parameters:
//   - params: Model parameters to optimize
//   - config: SGD configuration (LR, Momentum, WeightDecay)
//
// Returns a new SGD optimizer.
//
// Example:
//
//	sgd := optim.NewSGD(model.Parameters(), optim.SGDConfig{
//	    LR:       0.01,
//	    Momentum: 0.9,
//	})
func NewSGD[B tensor.Backend](params []*nn.Parameter[B], config SGDConfig, backend B) *SGD[B] {
	// Set defaults.
	if config.LR == 0 {
		config.LR = 0.01
	}

	return &SGD[B]{
		params:      params,
		lr:          config.LR,
		momentum:    config.Momentum,
		weightDecay: config.WeightDecay,
		velocities:  make(map[*nn.Parameter[B]]*tensor.Tensor[float32, B]),
		backend:     backend,
	}
}

// Step performs a single optimization step.
//
// Applies gradient descent update to all parameters:
//   - Without momentum: param = param*(1-lr*wd) - lr*grad
//   - With momentum: velocity = momentum*velocity + grad, param = param*(1-lr*wd) - lr*velocity
//
// Parameters with no gradient (not in computational graph) are skipped.
// All tensor math runs on the backend device — no CPU readback occurs.
//
// If the backend implements CacheInvalidator, ClearInputBufferCache() is called
// at the end of Step() to invalidate stale weight-buffer cache entries.
func (s *SGD[B]) Step(grads map[*tensor.RawTensor]*tensor.RawTensor) {
	for _, param := range s.params {
		// Get gradient for this parameter.
		grad := getGradient(param, grads)
		if grad == nil {
			// Parameter didn't participate in forward pass, skip.
			continue
		}

		// Wrap the raw gradient in a typed Tensor — zero-copy view.
		gradTensor := tensor.New[float32, B](grad, s.backend)

		if s.momentum == 0 {
			s.updateParameter(param, gradTensor)
		} else {
			s.updateParameterWithMomentum(param, gradTensor)
		}
	}

	// Invalidate the backend's input-buffer cache if it supports one.
	// After parameter tensors are swapped via SetTensor the old *RawTensor
	// keys in the cache are stale — the next forward pass must re-upload.
	invalidateCacheIfNeeded(s.backend)
}

// updateParameter performs simple SGD update without momentum.
//
// param = param * (1 - lr * weightDecay) - lr * grad
//
// Pure tensor ops — no AsFloat32() / CPU readback.
// Intermediate buffers are released immediately via ReleaseGPU() (GoMLX pattern).
func (s *SGD[B]) updateParameter(param *nn.Parameter[B], grad *tensor.Tensor[float32, B]) {
	current := param.Tensor()

	// Apply L2 weight decay as a tensor op.
	var decayed *tensor.Tensor[float32, B]
	if s.weightDecay != 0 {
		decayed = current.MulScalar(float32(1.0) - s.lr*s.weightDecay)
		current = decayed
	}

	// scaled_grad = lr * grad  (intermediate)
	scaledGrad := grad.MulScalar(s.lr)
	updated := current.Sub(scaledGrad)
	scaledGrad.Raw().ReleaseGPU() // intermediate: no longer needed
	if decayed != nil {
		decayed.Raw().ReleaseGPU() // intermediate: no longer needed
	}

	param.SetTensor(updated) // Releases old param GPU buffer internally
}

// updateParameterWithMomentum performs SGD update with momentum.
//
// velocity = momentum * velocity + grad
// param    = param * (1 - lr * weightDecay) - lr * velocity
//
// Pure tensor ops — no AsFloat32() / CPU readback.
// Intermediate buffers are released immediately via ReleaseGPU() (GoMLX pattern).
func (s *SGD[B]) updateParameterWithMomentum(param *nn.Parameter[B], grad *tensor.Tensor[float32, B]) {
	// Get or initialize velocity buffer (zeros, same shape as parameter).
	velocity, exists := s.velocities[param]
	if !exists {
		velocity = tensor.Zeros[float32](param.Tensor().Shape(), s.backend)
		s.velocities[param] = velocity
	}

	// newVelocity = momentum * velocity + grad
	scaledVel := velocity.MulScalar(s.momentum)
	newVelocity := scaledVel.Add(grad)
	scaledVel.Raw().ReleaseGPU() // intermediate: no longer needed

	current := param.Tensor()

	// Apply L2 weight decay as a tensor op.
	var decayed *tensor.Tensor[float32, B]
	if s.weightDecay != 0 {
		decayed = current.MulScalar(float32(1.0) - s.lr*s.weightDecay)
		current = decayed
	}

	// param = current - lr * newVelocity
	scaledNewVel := newVelocity.MulScalar(s.lr)
	updated := current.Sub(scaledNewVel)
	scaledNewVel.Raw().ReleaseGPU() // intermediate: no longer needed
	if decayed != nil {
		decayed.Raw().ReleaseGPU() // intermediate: no longer needed
	}

	// Release old velocity GPU buffer immediately (GoMLX FinalizeAll pattern).
	// Queued via DeferReleaseGPUBuffer — stays alive until after queue.Submit.
	if velocity != nil {
		velocity.Raw().ReleaseGPU()
	}

	// Persist both the new velocity and the updated parameter.
	s.velocities[param] = newVelocity
	param.SetTensor(updated) // Releases old param GPU buffer internally
}

// ZeroGrad clears gradients for all parameters.
func (s *SGD[B]) ZeroGrad() {
	for _, param := range s.params {
		param.ZeroGrad()
	}
}

// GetLR returns the current learning rate.
func (s *SGD[B]) GetLR() float32 {
	return s.lr
}

// SetLR updates the learning rate.
//
// Useful for learning rate scheduling during training.
func (s *SGD[B]) SetLR(lr float32) {
	s.lr = lr
}

// StateDict returns the optimizer state for serialization.
//
// For SGD with momentum, this exports velocity buffers for each parameter.
// Without momentum, returns an empty map.
//
// State keys: "velocity.{param_index}" -> velocity tensor.
func (s *SGD[B]) StateDict() map[string]*tensor.RawTensor {
	stateDict := make(map[string]*tensor.RawTensor)

	// Only save velocities if momentum is enabled.
	if s.momentum == 0 {
		return stateDict
	}

	// Export velocity buffers with index.
	for i, param := range s.params {
		velocity, exists := s.velocities[param]
		if !exists {
			continue // No velocity yet (hasn't been used in training).
		}

		key := fmt.Sprintf("velocity.%d", i)
		stateDict[key] = velocity.Raw()
	}

	return stateDict
}

// LoadStateDict loads optimizer state from serialization.
//
// Restores velocity buffers for SGD with momentum. If momentum is 0,
// ignores the provided state (no velocities needed).
//
// Parameters:
//   - stateDict: Map from state name to RawTensor
//
// Returns an error if velocity shapes don't match parameter shapes.
func (s *SGD[B]) LoadStateDict(stateDict map[string]*tensor.RawTensor) error {
	// If no momentum, nothing to load.
	if s.momentum == 0 {
		return nil
	}

	// Clear existing velocities.
	s.velocities = make(map[*nn.Parameter[B]]*tensor.Tensor[float32, B])

	// Load velocity buffers for each parameter.
	for i, param := range s.params {
		key := fmt.Sprintf("velocity.%d", i)
		velocityRaw, exists := stateDict[key]
		if !exists {
			// No velocity for this parameter — will be initialized on first step.
			continue
		}

		// Validate shape.
		if !velocityRaw.Shape().Equal(param.Tensor().Shape()) {
			return fmt.Errorf("velocity shape mismatch for parameter %d: expected %v, got %v",
				i, param.Tensor().Shape(), velocityRaw.Shape())
		}

		// Convert to typed tensor.
		velocity := tensor.New[float32, B](velocityRaw, s.backend)
		s.velocities[param] = velocity
	}

	return nil
}
