package ops

import (
	"math"

	"github.com/born-ml/born/internal/tensor"
)

// LogOp represents element-wise natural logarithm operation.
//
// Forward:
//
//	output = log(input)
//
// Backward:
//
//	∂L/∂input = ∂L/∂output * (1 / input)
//
// The gradient is the reciprocal of the input, scaled by the output gradient.
type LogOp struct {
	input  *tensor.RawTensor
	output *tensor.RawTensor
}

// NewLogOp creates a new log operation.
func NewLogOp(input, output *tensor.RawTensor) *LogOp {
	return &LogOp{
		input:  input,
		output: output,
	}
}

// Inputs returns the input tensors.
func (op *LogOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor.
func (op *LogOp) Output() *tensor.RawTensor {
	return op.output
}

// Backward computes the gradient with respect to input using only backend ops.
//
// Gradient formula:
//
//	∂L/∂input = ∂L/∂output / input
//
// Note: This assumes input > 0 (log is only defined for positive values).
func (op *LogOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	// grad_input = grad_output / input — single backend op, no data leaves the device.
	gradInput := backend.Div(outputGrad, op.input)

	return []*tensor.RawTensor{gradInput}
}

// LogWithEpsilonOp represents log with numerical stability epsilon.
//
// Forward:
//
//	output = log(input + epsilon)
//
// This is numerically more stable when input might be very close to zero.
type LogWithEpsilonOp struct {
	input   *tensor.RawTensor
	output  *tensor.RawTensor
	epsilon float64
}

// NewLogWithEpsilonOp creates a log operation with epsilon for stability.
func NewLogWithEpsilonOp(input, output *tensor.RawTensor, epsilon float64) *LogWithEpsilonOp {
	return &LogWithEpsilonOp{
		input:   input,
		output:  output,
		epsilon: epsilon,
	}
}

// Inputs returns the input tensors.
func (op *LogWithEpsilonOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor.
func (op *LogWithEpsilonOp) Output() *tensor.RawTensor {
	return op.output
}

// Backward computes gradient: ∂L/∂input = ∂L/∂output / (input + epsilon).
func (op *LogWithEpsilonOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	// epsilon must be typed to match the tensor's dtype — backend.AddScalar uses a strict
	// type assertion (scalar.(float32) or scalar.(float64)).
	var eps any
	switch op.input.DType() {
	case tensor.Float32:
		eps = float32(op.epsilon)
	default: // Float64
		eps = op.epsilon
	}

	// shifted = input + epsilon (stays on device)
	shifted := backend.AddScalar(op.input, eps)
	// grad_input = grad_output / (input + epsilon)
	gradInput := backend.Div(outputGrad, shifted)

	return []*tensor.RawTensor{gradInput}
}

// Exp computes element-wise exponential (helper for softmax).
//
// Forward: output = exp(input)
// Backward: ∂L/∂input = ∂L/∂output * exp(input) = ∂L/∂output * output
//
// Note: This is a helper function, not a full Operation.
// For autodiff support, use ExpOp (to be implemented if needed).
func Exp(input *tensor.RawTensor, device tensor.Device) *tensor.RawTensor {
	output, err := tensor.NewRaw(input.Shape(), input.DType(), device)
	if err != nil {
		panic(err)
	}

	switch input.DType() {
	case tensor.Float32:
		inputData := input.AsFloat32()
		outputData := output.AsFloat32()
		for i, val := range inputData {
			outputData[i] = float32(math.Exp(float64(val)))
		}

	case tensor.Float64:
		inputData := input.AsFloat64()
		outputData := output.AsFloat64()
		for i, val := range inputData {
			outputData[i] = math.Exp(val)
		}

	default:
		panic("Exp: only supports float32 and float64")
	}

	return output
}

// Log computes element-wise natural logarithm (helper function).
//
// Forward: output = log(input)
//
// Note: This is a helper function for use outside autodiff.
// For autodiff support, use backend.Log() which records LogOp.
func Log(input *tensor.RawTensor, device tensor.Device) *tensor.RawTensor {
	output, err := tensor.NewRaw(input.Shape(), input.DType(), device)
	if err != nil {
		panic(err)
	}

	switch input.DType() {
	case tensor.Float32:
		inputData := input.AsFloat32()
		outputData := output.AsFloat32()
		for i, val := range inputData {
			outputData[i] = float32(math.Log(float64(val)))
		}

	case tensor.Float64:
		inputData := input.AsFloat64()
		outputData := output.AsFloat64()
		for i, val := range inputData {
			outputData[i] = math.Log(val)
		}

	default:
		panic("Log: only supports float32 and float64")
	}

	return output
}
