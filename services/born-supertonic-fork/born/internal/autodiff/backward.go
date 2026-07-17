package autodiff

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// BackwardCapable is an interface for backends that support backward pass.
// AutodiffBackend implements this interface.
type BackwardCapable interface {
	tensor.Backend
	// GetTape returns the gradient tape for backward computation.
	GetTape() *GradientTape
}

// GetTape returns the gradient tape (implements BackwardCapable interface).
func (b *AutodiffBackend[B]) GetTape() *GradientTape {
	return b.tape
}

// Backward computes gradients for a tensor using the AutodiffBackend's tape.
//
// This helper function extracts the tape from an AutodiffBackend
// and computes gradients for the given tensor.
//
// Parameters:
//   - t: The output tensor to compute gradients for
//   - backend: The backend (must be AutodiffBackend or implement BackwardCapable)
//
// Returns a map from RawTensor to its gradient.
//
// Example:
//
//	backend := autodiff.New(cpu.New())
//	backend.Tape().StartRecording()
//	x := tensor.Ones[float32](Shape{2}, backend)
//	y := x.Mul(x) // y = x²
//	gradients := autodiff.Backward(y, backend)
//	grad := gradients[x.Raw()] // Get gradient for x
func Backward[T tensor.DType, B BackwardCapable](t *tensor.Tensor[T, B], backend B) map[*tensor.RawTensor]*tensor.RawTensor {
	tape := backend.GetTape()

	if tape.NumOps() == 0 {
		panic("backward: no operations recorded (did you forget to call Tape().StartRecording()?)")
	}

	// Create output gradient: ones with same shape as output
	outputGrad, err := tensor.NewRaw(t.Shape(), t.DType(), backend.Device())
	if err != nil {
		panic(fmt.Sprintf("backward: failed to create output gradient: %v", err))
	}

	// Initialize output gradient to ones
	switch t.DType() {
	case tensor.Float32:
		data := outputGrad.AsFloat32()
		for i := range data {
			data[i] = 1.0
		}
	case tensor.Float64:
		data := outputGrad.AsFloat64()
		for i := range data {
			data[i] = 1.0
		}
	default:
		panic(fmt.Sprintf("backward: unsupported dtype %s (only float32/float64 supported)", t.DType()))
	}

	// Compute gradients using tape
	return tape.Backward(outputGrad, backend)
}

// ReleaseGradients releases GPU buffers for all gradient tensors in the map.
// Call after optimizer.Step(grads) — gradient tensors are no longer needed and
// their GPU buffers should be freed immediately rather than waiting for GC.
func ReleaseGradients(grads map[*tensor.RawTensor]*tensor.RawTensor) {
	for _, grad := range grads {
		grad.ReleaseGPU()
	}
}
