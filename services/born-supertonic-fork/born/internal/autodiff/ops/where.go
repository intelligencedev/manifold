package ops

import (
	"github.com/born-ml/born/internal/tensor"
)

// WhereOp represents a conditional selection: output = where(cond, x, y).
//
// Forward: output[i] = x[i] if cond[i] else y[i]
//
// Backward:
//
//	grad_x = where(cond, grad_out, 0)
//	grad_y = where(cond, 0, grad_out)
//
// The condition tensor has no gradient (it's boolean).
type WhereOp struct {
	condition *tensor.RawTensor // bool tensor
	x         *tensor.RawTensor // "true" branch values
	y         *tensor.RawTensor // "false" branch values
	output    *tensor.RawTensor // result tensor
}

// NewWhereOp creates a new where operation.
func NewWhereOp(condition, x, y, output *tensor.RawTensor) *WhereOp {
	return &WhereOp{
		condition: condition,
		x:         x,
		y:         y,
		output:    output,
	}
}

// Inputs returns the input tensors (x and y).
// Note: condition is not included as it has no gradient (boolean).
func (op *WhereOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.x, op.y}
}

// Output returns the output tensor.
func (op *WhereOp) Output() *tensor.RawTensor {
	return op.output
}

// Backward computes gradients for x and y.
//
//	grad_x = where(cond, grad_out, 0)  -- gradient flows only where cond is true
//	grad_y = where(cond, 0, grad_out)  -- gradient flows only where cond is false
func (op *WhereOp) Backward(gradOutput *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	// Create zero tensor with same shape as gradOutput
	zeros, err := tensor.NewRaw(gradOutput.Shape(), gradOutput.DType(), backend.Device())
	if err != nil {
		panic("WhereOp.Backward: failed to create zeros tensor: " + err.Error())
	}
	// NewRaw creates zero-initialized tensor, so zeros is ready

	// grad_x = where(cond, grad_out, zeros)
	// Gradient flows to x only where condition was true
	gradX := backend.Where(op.condition, gradOutput, zeros)

	// grad_y = where(cond, zeros, grad_out)
	// Gradient flows to y only where condition was false
	gradY := backend.Where(op.condition, zeros, gradOutput)

	return []*tensor.RawTensor{gradX, gradY}
}
