package ops

import (
	"github.com/born-ml/born/internal/tensor"
)

// ClampOp represents the clamp operation: y = clamp(x, min, max).
//
// Backward pass:
//   - d(clamp(x, min, max))/dx = 1 if min <= x <= max, else 0
//   - grad_input = grad_output * (1 if min <= input <= max, else 0)
type ClampOp struct {
	input    *tensor.RawTensor // x
	minBound any               // min value
	maxBound any               // max value
	output   *tensor.RawTensor // clamp(x, min, max)
}

// NewClampOp creates a new ClampOp.
func NewClampOp(input *tensor.RawTensor, minBound, maxBound any, output *tensor.RawTensor) *ClampOp {
	return &ClampOp{
		input:    input,
		minBound: minBound,
		maxBound: maxBound,
		output:   output,
	}
}

// Backward computes input gradient for clamp.
//
// Since d(clamp(x, min, max))/dx = 1 if min <= x <= max, else 0:
// grad_input = grad_output * (1 if min <= input <= max, else 0).
func (op *ClampOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	input := op.input

	switch input.DType() {
	case tensor.Int32:
		minBound := tensor.CheckScalarDType[int32](op.minBound)
		maxBound := tensor.CheckScalarDType[int32](op.maxBound)
		maskedGrad := clampBackwardGeneric(outputGrad, input, minBound, maxBound, backend)
		return []*tensor.RawTensor{maskedGrad}
	case tensor.Int64:
		minBound := tensor.CheckScalarDType[int64](op.minBound)
		maxBound := tensor.CheckScalarDType[int64](op.maxBound)
		maskedGrad := clampBackwardGeneric(outputGrad, input, minBound, maxBound, backend)
		return []*tensor.RawTensor{maskedGrad}

	case tensor.Float32:
		minBound := tensor.CheckScalarDType[float32](op.minBound)
		maxBound := tensor.CheckScalarDType[float32](op.maxBound)
		maskedGrad := clampBackwardGeneric(outputGrad, input, minBound, maxBound, backend)
		return []*tensor.RawTensor{maskedGrad}

	case tensor.Float64:
		minBound := tensor.CheckScalarDType[float64](op.minBound)
		maxBound := tensor.CheckScalarDType[float64](op.maxBound)
		maskedGrad := clampBackwardGeneric(outputGrad, input, minBound, maxBound, backend)
		return []*tensor.RawTensor{maskedGrad}
	default:
		panic("clamp: unsupported dtype (only int32/int64/float32/float64 supported)")
	}
}

func clampBackwardGeneric[T int32 | int64 | float32 | float64](outputGrad, input *tensor.RawTensor, minBound, maxBound T, backend tensor.Backend) *tensor.RawTensor {
	minValues := tensor.Full(input.Shape(), minBound, backend).Raw()
	maxValues := tensor.Full(input.Shape(), maxBound, backend).Raw()

	ones := tensor.Ones[T](input.Shape(), backend).Raw()
	zeros := tensor.Zeros[T](input.Shape(), backend).Raw()

	// Check if original input is within bounds: min <= input <= max
	minMask := backend.GreaterEqual(input, minValues) // bool where input >= min
	maxMask := backend.LowerEqual(input, maxValues)   // bool where input <= max
	combinedMask := backend.And(minMask, maxMask)     // bool where min <= input <= max

	// Convert bool mask to dtype T for multiplication
	mask := backend.Where(combinedMask, ones, zeros)
	maskedGrad := backend.Mul(outputGrad, mask) // grad_output * mask

	return maskedGrad
}

// Inputs returns the input tensor [x].
func (op *ClampOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor clamp(x, min, max).
func (op *ClampOp) Output() *tensor.RawTensor {
	return op.output
}
