package ops

import "github.com/born-ml/born/internal/tensor"

// MulScalarOp represents element-wise multiplication by a scalar: output = x * s.
//
// Backward: grad_x = outputGrad * s.
type MulScalarOp struct {
	input  *tensor.RawTensor
	output *tensor.RawTensor
	scalar any // float32 or float64, must match input dtype
}

// NewMulScalarOp creates a new MulScalarOp.
func NewMulScalarOp(input, output *tensor.RawTensor, scalar any) *MulScalarOp {
	return &MulScalarOp{input: input, output: output, scalar: scalar}
}

// Backward computes gradient: grad_x = outputGrad * scalar.
func (op *MulScalarOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	return []*tensor.RawTensor{backend.MulScalar(outputGrad, op.scalar)}
}

// Inputs returns the input tensor.
func (op *MulScalarOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor.
func (op *MulScalarOp) Output() *tensor.RawTensor {
	return op.output
}

// AddScalarOp represents element-wise addition of a scalar: output = x + s.
//
// Backward: grad_x = outputGrad (scalar is constant).
type AddScalarOp struct {
	input  *tensor.RawTensor
	output *tensor.RawTensor
}

// NewAddScalarOp creates a new AddScalarOp.
func NewAddScalarOp(input, output *tensor.RawTensor) *AddScalarOp {
	return &AddScalarOp{input: input, output: output}
}

// Backward computes gradient: grad_x = outputGrad (passthrough).
func (op *AddScalarOp) Backward(outputGrad *tensor.RawTensor, _ tensor.Backend) []*tensor.RawTensor {
	return []*tensor.RawTensor{outputGrad.Clone()}
}

// Inputs returns the input tensor.
func (op *AddScalarOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor.
func (op *AddScalarOp) Output() *tensor.RawTensor {
	return op.output
}

// SubScalarOp represents element-wise subtraction of a scalar: output = x - s.
//
// Backward: grad_x = outputGrad (scalar is constant).
type SubScalarOp struct {
	input  *tensor.RawTensor
	output *tensor.RawTensor
}

// NewSubScalarOp creates a new SubScalarOp.
func NewSubScalarOp(input, output *tensor.RawTensor) *SubScalarOp {
	return &SubScalarOp{input: input, output: output}
}

// Backward computes gradient: grad_x = outputGrad (passthrough).
func (op *SubScalarOp) Backward(outputGrad *tensor.RawTensor, _ tensor.Backend) []*tensor.RawTensor {
	return []*tensor.RawTensor{outputGrad.Clone()}
}

// Inputs returns the input tensor.
func (op *SubScalarOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor.
func (op *SubScalarOp) Output() *tensor.RawTensor {
	return op.output
}

// DivScalarOp represents element-wise division by a scalar: output = x / s.
//
// Backward: grad_x = outputGrad / s.
type DivScalarOp struct {
	input  *tensor.RawTensor
	output *tensor.RawTensor
	scalar any // float32 or float64, must match input dtype
}

// NewDivScalarOp creates a new DivScalarOp.
func NewDivScalarOp(input, output *tensor.RawTensor, scalar any) *DivScalarOp {
	return &DivScalarOp{input: input, output: output, scalar: scalar}
}

// Backward computes gradient: grad_x = outputGrad / scalar.
func (op *DivScalarOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	return []*tensor.RawTensor{backend.DivScalar(outputGrad, op.scalar)}
}

// Inputs returns the input tensor.
func (op *DivScalarOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor.
func (op *DivScalarOp) Output() *tensor.RawTensor {
	return op.output
}
