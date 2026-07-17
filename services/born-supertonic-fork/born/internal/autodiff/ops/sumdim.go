package ops

import "github.com/born-ml/born/internal/tensor"

// SumDimOp represents a reduction sum operation along a dimension: output = sum(x, dim).
//
// Forward:
//
//	y = sum(x, dim, keepDim)
//
// Backward:
//
//	grad_x = broadcast(grad_y, x.shape)
//
// If keepDim=false, we need to unsqueeze grad_y first to match broadcasting requirements.
type SumDimOp struct {
	inputs  []*tensor.RawTensor // [x]
	output  *tensor.RawTensor   // sum(x, dim)
	dim     int                 // dimension to reduce
	keepDim bool                // whether to keep dimension
}

// NewSumDimOp creates a new SumDimOp.
func NewSumDimOp(x, output *tensor.RawTensor, dim int, keepDim bool) *SumDimOp {
	return &SumDimOp{
		inputs:  []*tensor.RawTensor{x},
		output:  output,
		dim:     dim,
		keepDim: keepDim,
	}
}

// Backward computes input gradients for sum reduction.
//
// The gradient flows by broadcasting grad_output to match input shape.
// Since sum just accumulates values, each input element contributes 1.0 to the output,
// so the gradient is simply broadcast back.
func (op *SumDimOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	x := op.inputs[0]
	grad := outputGrad

	// If keepDim=false, unsqueeze grad to restore the reduced dim.
	// Uses backend.Reshape — stays on GPU, no CPU readback.
	if !op.keepDim {
		grad = backend.Reshape(grad, unsqueezeDimShape(grad.Shape(), op.dim, x.Shape()))
	}

	// Broadcast gradient to input shape via backend.Expand — stays on GPU.
	gradX := broadcastTo(grad, x.Shape(), backend)

	return []*tensor.RawTensor{gradX}
}

// Inputs returns the input tensors [x].
func (op *SumDimOp) Inputs() []*tensor.RawTensor {
	return op.inputs
}

// Output returns the output tensor sum(x, dim).
func (op *SumDimOp) Output() *tensor.RawTensor {
	return op.output
}

// unsqueezeDimShape computes the shape with a size-1 dimension inserted at dim.
func unsqueezeDimShape(gradShape tensor.Shape, dim int, targetShape tensor.Shape) tensor.Shape {
	ndim := len(targetShape)
	if dim < 0 {
		dim = ndim + dim
	}

	newShape := make(tensor.Shape, 0, len(gradShape)+1)
	for i := 0; i < ndim; i++ {
		if i == dim {
			newShape = append(newShape, 1)
		} else {
			origIdx := i
			if i > dim {
				origIdx = i - 1
			}
			if origIdx < len(gradShape) {
				newShape = append(newShape, gradShape[origIdx])
			}
		}
	}
	if len(newShape) < len(gradShape)+1 {
		newShape = append(newShape, gradShape[len(newShape):]...)
	}
	return newShape
}

// broadcastTo broadcasts a tensor to match target shape via backend.Expand.
// Stays on GPU — no CPU readback.
func broadcastTo(t *tensor.RawTensor, targetShape tensor.Shape, backend tensor.Backend) *tensor.RawTensor {
	if t.Shape().Equal(targetShape) {
		return t.Clone()
	}
	return backend.Expand(t, targetShape)
}
