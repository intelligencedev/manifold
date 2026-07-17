package optim

import (
	"fmt"
	"math"

	"github.com/born-ml/born/internal/nn"
	"github.com/born-ml/born/internal/tensor"
)

// Adam implements the Adam (Adaptive Moment Estimation) optimizer with optional
// decoupled weight decay (AdamW).
//
// Adam combines ideas from RMSprop and momentum:
//   - Maintains exponential moving averages of gradients (first moment)
//   - Maintains exponential moving averages of squared gradients (second moment)
//   - Applies bias correction to compensate for initialization at zero
//
// Update rule (AdamW when WeightDecay > 0):
//
//	m_t   = beta1 * m_{t-1} + (1-beta1) * gradient       // First moment
//	v_t   = beta2 * v_{t-1} + (1-beta2) * gradient²      // Second moment
//	m_hat = m_t / (1 - beta1^t)                           // Bias correction
//	v_hat = v_t / (1 - beta2^t)                           // Bias correction
//	param = param * (1 - lr * weightDecay)                // Decoupled weight decay
//	      - lr * m_hat / (sqrt(v_hat) + eps)              // Adaptive update
//
// All parameter updates use pure tensor operations — no AsFloat32() / CPU readback
// in the hot path. Bias-correction scalars are O(1) CPU values per step (they depend
// only on the scalar timestep, not on tensor data).
//
// If the backend implements CacheInvalidator, ClearInputBufferCache() is called at
// the end of Step() to invalidate stale weight-buffer cache entries.
//
// Reference: "Adam: A Method for Stochastic Optimization" (Kingma & Ba, 2014)
//
// Example:
//
//	optimizer := optim.NewAdam(model.Parameters(), optim.AdamConfig{
//	    LR:    0.001,
//	    Betas: [2]float32{0.9, 0.999},
//	    Eps:   1e-8,
//	})
//
//	for epoch := range epochs {
//	    loss := train_step(model, batch)
//	    grads := autodiff.Backward(loss, backend)
//	    optimizer.Step(grads)
//	    optimizer.ZeroGrad()
//	}
type Adam[B tensor.Backend] struct {
	params      []*nn.Parameter[B]
	lr          float32
	beta1       float32
	beta2       float32
	eps         float32
	weightDecay float32
	t           int                                             // Timestep for bias correction
	m           map[*nn.Parameter[B]]*tensor.Tensor[float32, B] // First moment estimates
	v           map[*nn.Parameter[B]]*tensor.Tensor[float32, B] // Second moment estimates
	backend     B
}

// AdamConfig holds configuration for Adam optimizer.
type AdamConfig struct {
	LR          float32    // Learning rate (default: 0.001)
	Betas       [2]float32 // Coefficients for computing running averages (default: [0.9, 0.999])
	Eps         float32    // Term for numerical stability (default: 1e-8)
	WeightDecay float32    // Decoupled weight-decay coefficient — AdamW style (default: 0.0)
}

// NewAdam creates a new Adam optimizer.
//
// Parameters:
//   - params: Model parameters to optimize
//   - config: Adam configuration (LR, Betas, Eps, WeightDecay)
//
// Returns a new Adam optimizer with default hyperparameters if not specified.
//
// Default hyperparameters:
//   - LR: 0.001
//   - Beta1: 0.9
//   - Beta2: 0.999
//   - Eps: 1e-8
func NewAdam[B tensor.Backend](params []*nn.Parameter[B], config AdamConfig, backend B) *Adam[B] {
	// Set defaults.
	if config.LR == 0 {
		config.LR = 0.001
	}
	if config.Betas[0] == 0 {
		config.Betas[0] = 0.9
	}
	if config.Betas[1] == 0 {
		config.Betas[1] = 0.999
	}
	if config.Eps == 0 {
		config.Eps = 1e-8
	}

	return &Adam[B]{
		params:      params,
		lr:          config.LR,
		beta1:       config.Betas[0],
		beta2:       config.Betas[1],
		eps:         config.Eps,
		weightDecay: config.WeightDecay,
		t:           0,
		m:           make(map[*nn.Parameter[B]]*tensor.Tensor[float32, B]),
		v:           make(map[*nn.Parameter[B]]*tensor.Tensor[float32, B]),
		backend:     backend,
	}
}

// Step performs a single optimization step using Adam algorithm.
//
// Applies Adam update to all parameters:
//  1. Update biased first moment estimate
//  2. Update biased second moment estimate
//  3. Compute bias-corrected moment estimates
//  4. Apply decoupled weight decay (if configured)
//  5. Update parameters
//
// Parameters with no gradient are skipped.
// If the backend implements CacheInvalidator, ClearInputBufferCache() is called
// at the end of Step() to invalidate stale weight-buffer cache entries.
func (a *Adam[B]) Step(grads map[*tensor.RawTensor]*tensor.RawTensor) {
	// Optimizer ops must NOT be recorded on the autodiff tape. If the tape is
	// still recording (Backward restores state), optimizer intermediates land on
	// tape → ClearTape releases their GPU buffers → moments lose GPU data.
	if ng, ok := any(a.backend).(interface{ NoGrad(func()) }); ok {
		ng.NoGrad(func() { a.stepInner(grads) })
		return
	}
	a.stepInner(grads)
}

func (a *Adam[B]) stepInner(grads map[*tensor.RawTensor]*tensor.RawTensor) {
	// Increment timestep.
	a.t++

	// Bias-correction scalars: O(1) CPU computation — only the scalar timestep
	// is read, no tensor data leaves the device.
	biasCorrection1 := float32(1.0 - math.Pow(float64(a.beta1), float64(a.t)))
	biasCorrection2 := float32(1.0 - math.Pow(float64(a.beta2), float64(a.t)))

	for _, param := range a.params {
		// Get gradient for this parameter.
		grad := getGradient(param, grads)
		if grad == nil {
			// Parameter didn't participate in forward pass, skip.
			continue
		}

		// Wrap raw gradient in a typed Tensor — zero-copy view.
		gradTensor := tensor.New[float32, B](grad, a.backend)

		// Get or initialize first moment (m).
		m, mExists := a.m[param]
		if !mExists {
			m = tensor.Zeros[float32](param.Tensor().Shape(), a.backend)
			a.m[param] = m
		}

		// Get or initialize second moment (v).
		v, vExists := a.v[param]
		if !vExists {
			v = tensor.Zeros[float32](param.Tensor().Shape(), a.backend)
			a.v[param] = v
		}

		// Update moments and parameter — entirely on device.
		a.updateParameter(param, gradTensor, m, v, biasCorrection1, biasCorrection2)
	}

	// Invalidate the backend's input-buffer cache if it supports one.
	// After parameter tensors are swapped via SetTensor the old *RawTensor
	// keys in the cache are stale — the next forward pass must re-upload.
	invalidateCacheIfNeeded(a.backend)
}

// updateParameter performs Adam update for a single parameter using pure tensor ops.
//
// All computation runs on the backend device — no AsFloat32() / CPU readback occurs.
// Scalar bias-correction values are computed once per step in O(1) CPU work and
// applied via MulScalar / AddScalar which dispatch to the backend.
//
// m_t   = beta1 * m_{t-1} + (1-beta1) * grad
// v_t   = beta2 * v_{t-1} + (1-beta2) * grad²
// mHat  = m_t  / biasCorrection1
// vHat  = v_t  / biasCorrection2
// param = param * (1 - lr * weightDecay) - lr * mHat / (sqrt(vHat) + eps).
func (a *Adam[B]) updateParameter(
	param *nn.Parameter[B],
	grad *tensor.Tensor[float32, B],
	m, v *tensor.Tensor[float32, B],
	biasCorrection1, biasCorrection2 float32,
) {
	// Update biased first moment estimate.
	// newM = beta1 * m + (1 - beta1) * grad
	scaledM := m.MulScalar(a.beta1)
	scaledGradM := grad.MulScalar(float32(1.0) - a.beta1)
	newM := scaledM.Add(scaledGradM)
	scaledM.Raw().ReleaseGPU()     // intermediate: no longer needed
	scaledGradM.Raw().ReleaseGPU() // intermediate: no longer needed

	// Update biased second raw moment estimate.
	// newV = beta2 * v + (1 - beta2) * grad²
	gradSquared := grad.Mul(grad)
	scaledV := v.MulScalar(a.beta2)
	scaledGradSq := gradSquared.MulScalar(float32(1.0) - a.beta2)
	newV := scaledV.Add(scaledGradSq)
	gradSquared.Raw().ReleaseGPU()  // intermediate: consumed by scaledGradSq
	scaledV.Raw().ReleaseGPU()      // intermediate: no longer needed
	scaledGradSq.Raw().ReleaseGPU() // intermediate: no longer needed

	// Compute bias-corrected moment estimates.
	// mHat = newM / biasCorrection1  →  newM * (1 / biasCorrection1)
	// vHat = newV / biasCorrection2  →  newV * (1 / biasCorrection2)
	mHat := newM.MulScalar(float32(1.0) / biasCorrection1)
	vHat := newV.MulScalar(float32(1.0) / biasCorrection2)

	// denom = sqrt(vHat) + eps
	sqrtVHat := vHat.Sqrt()
	vHat.Raw().ReleaseGPU() // intermediate: consumed by sqrtVHat
	denom := sqrtVHat.AddScalar(a.eps)
	sqrtVHat.Raw().ReleaseGPU() // intermediate: no longer needed

	// adaptive update = lr * mHat / denom
	mHatDivDenom := mHat.Div(denom)
	mHat.Raw().ReleaseGPU()  // intermediate: no longer needed
	denom.Raw().ReleaseGPU() // intermediate: no longer needed
	adaptiveUpdate := mHatDivDenom.MulScalar(a.lr)
	mHatDivDenom.Raw().ReleaseGPU() // intermediate: no longer needed

	// Apply decoupled weight decay (AdamW-style) as a tensor op, then subtract update.
	current := param.Tensor()
	var decayed *tensor.Tensor[float32, B]
	if a.weightDecay != 0 {
		decayed = current.MulScalar(float32(1.0) - a.lr*a.weightDecay)
		current = decayed
	}

	updated := current.Sub(adaptiveUpdate)
	if decayed != nil {
		decayed.Raw().ReleaseGPU() // intermediate: no longer needed
	}
	adaptiveUpdate.Raw().ReleaseGPU() // intermediate: no longer needed

	// Release old moment and param GPU buffers immediately (GoMLX FinalizeAll pattern).
	// The buffers are queued via DeferReleaseGPUBuffer — they remain alive until
	// after queue.Submit, preventing use-after-release in the pending encoder batch.
	if m != nil {
		m.Raw().ReleaseGPU()
	}
	if v != nil {
		v.Raw().ReleaseGPU()
	}

	// Persist updated moments and parameter — no CPU readback.
	// Mark moments as persistent so ReclaimMemory does not release them.
	newM.Raw().SetGPUPersistent(true)
	newV.Raw().SetGPUPersistent(true)
	a.m[param] = newM
	a.v[param] = newV
	param.SetTensor(updated) // Releases old param GPU buffer internally
}

// ZeroGrad clears gradients for all parameters.
func (a *Adam[B]) ZeroGrad() {
	for _, param := range a.params {
		param.ZeroGrad()
	}
}

// DiagMoments returns moment tensor GPU state for diagnostics.
func (a *Adam[B]) DiagMoments() [][2]*tensor.RawTensor {
	result := make([][2]*tensor.RawTensor, len(a.params))
	for i, p := range a.params {
		if m, ok := a.m[p]; ok {
			result[i][0] = m.Raw()
		}
		if v, ok := a.v[p]; ok {
			result[i][1] = v.Raw()
		}
	}
	return result
}

// GetLR returns the current learning rate.
func (a *Adam[B]) GetLR() float32 {
	return a.lr
}

// SetLR updates the learning rate.
//
// Useful for learning rate scheduling during training.
func (a *Adam[B]) SetLR(lr float32) {
	a.lr = lr
}

// GetTimestep returns the current timestep.
//
// Useful for monitoring optimizer state.
func (a *Adam[B]) GetTimestep() int {
	return a.t
}

// StateDict returns the optimizer state for serialization.
//
// For Adam, this exports:
//   - First moment estimates (m) for each parameter: "m.{param_index}"
//   - Second moment estimates (v) for each parameter: "v.{param_index}"
//
// Returns a map from state name to RawTensor.
func (a *Adam[B]) StateDict() map[string]*tensor.RawTensor {
	stateDict := make(map[string]*tensor.RawTensor)

	// Export first moment (m) and second moment (v) for each parameter.
	for i, param := range a.params {
		// First moment.
		if m, exists := a.m[param]; exists {
			key := fmt.Sprintf("m.%d", i)
			stateDict[key] = m.Raw()
		}

		// Second moment.
		if v, exists := a.v[param]; exists {
			key := fmt.Sprintf("v.%d", i)
			stateDict[key] = v.Raw()
		}
	}

	return stateDict
}

// LoadStateDict loads optimizer state from serialization.
//
// Restores first and second moment estimates for Adam optimizer.
//
// Parameters:
//   - stateDict: Map from state name to RawTensor
//
// Returns an error if moment shapes don't match parameter shapes.
func (a *Adam[B]) LoadStateDict(stateDict map[string]*tensor.RawTensor) error {
	// Clear existing state.
	a.m = make(map[*nn.Parameter[B]]*tensor.Tensor[float32, B])
	a.v = make(map[*nn.Parameter[B]]*tensor.Tensor[float32, B])

	// Load first and second moments for each parameter.
	for i, param := range a.params {
		// Load first moment (m).
		mKey := fmt.Sprintf("m.%d", i)
		if mRaw, exists := stateDict[mKey]; exists {
			// Validate shape.
			if !mRaw.Shape().Equal(param.Tensor().Shape()) {
				return fmt.Errorf("m shape mismatch for parameter %d: expected %v, got %v",
					i, param.Tensor().Shape(), mRaw.Shape())
			}
			a.m[param] = tensor.New[float32, B](mRaw, a.backend)
		}

		// Load second moment (v).
		vKey := fmt.Sprintf("v.%d", i)
		if vRaw, exists := stateDict[vKey]; exists {
			// Validate shape.
			if !vRaw.Shape().Equal(param.Tensor().Shape()) {
				return fmt.Errorf("v shape mismatch for parameter %d: expected %v, got %v",
					i, param.Tensor().Shape(), vRaw.Shape())
			}
			a.v[param] = tensor.New[float32, B](vRaw, a.backend)
		}
	}

	return nil
}
