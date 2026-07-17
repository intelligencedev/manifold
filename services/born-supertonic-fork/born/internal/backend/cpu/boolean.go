package cpu

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// Boolean operations - work on bool tensors.

// Or computes element-wise logical OR.
func (cpu *CPUBackend) Or(a, b *tensor.RawTensor) *tensor.RawTensor {
	if a.DType() != tensor.Bool || b.DType() != tensor.Bool {
		panic("or: both tensors must be bool dtype")
	}

	outShape, needsBroadcast, err := tensor.BroadcastShapes(a.Shape(), b.Shape())
	if err != nil {
		panic(fmt.Sprintf("or: %v", err))
	}

	result, err := tensor.NewRaw(outShape, tensor.Bool, cpu.device)
	if err != nil {
		panic(fmt.Sprintf("or: %v", err))
	}

	if !needsBroadcast && a.Shape().Equal(b.Shape()) {
		orVectorized(result, a, b)
	} else {
		orWithBroadcast(result, a, b, outShape)
	}

	return result
}

// And computes element-wise logical AND.
func (cpu *CPUBackend) And(a, b *tensor.RawTensor) *tensor.RawTensor {
	if a.DType() != tensor.Bool || b.DType() != tensor.Bool {
		panic("and: both tensors must be bool dtype")
	}

	outShape, needsBroadcast, err := tensor.BroadcastShapes(a.Shape(), b.Shape())
	if err != nil {
		panic(fmt.Sprintf("and: %v", err))
	}

	result, err := tensor.NewRaw(outShape, tensor.Bool, cpu.device)
	if err != nil {
		panic(fmt.Sprintf("and: %v", err))
	}

	if !needsBroadcast && a.Shape().Equal(b.Shape()) {
		andVectorized(result, a, b)
	} else {
		andWithBroadcast(result, a, b, outShape)
	}

	return result
}

// Not computes element-wise logical NOT.
func (cpu *CPUBackend) Not(x *tensor.RawTensor) *tensor.RawTensor {
	if x.DType() != tensor.Bool {
		panic("not: tensor must be bool dtype")
	}

	result, err := tensor.NewRaw(x.Shape(), tensor.Bool, cpu.device)
	if err != nil {
		panic(fmt.Sprintf("not: %v", err))
	}

	src := x.AsBool()
	dst := result.AsBool()

	for i := range dst {
		dst[i] = !src[i]
	}

	return result
}

// ============================================================================
// Vectorized implementations
// ============================================================================

func orVectorized(result, a, b *tensor.RawTensor) {
	aData := a.AsBool()
	bData := b.AsBool()
	dst := result.AsBool()

	for i := range dst {
		dst[i] = aData[i] || bData[i]
	}
}

func andVectorized(result, a, b *tensor.RawTensor) {
	aData := a.AsBool()
	bData := b.AsBool()
	dst := result.AsBool()

	for i := range dst {
		dst[i] = aData[i] && bData[i]
	}
}

// ============================================================================
// Broadcast implementations
// ============================================================================

func orWithBroadcast(result, a, b *tensor.RawTensor, outShape tensor.Shape) {
	aShape := a.Shape()
	bShape := b.Shape()

	dst := result.AsBool()
	aData := a.AsBool()
	bData := b.AsBool()

	outStrides := outShape.ComputeStrides()
	aStrides := computeBroadcastStridesForShape(aShape, outShape)
	bStrides := computeBroadcastStridesForShape(bShape, outShape)

	n := outShape.NumElements()
	for i := range n {
		aIdx := computeFlatIndex(i, outStrides, aStrides)
		bIdx := computeFlatIndex(i, outStrides, bStrides)
		dst[i] = aData[aIdx] || bData[bIdx]
	}
}

func andWithBroadcast(result, a, b *tensor.RawTensor, outShape tensor.Shape) {
	aShape := a.Shape()
	bShape := b.Shape()

	dst := result.AsBool()
	aData := a.AsBool()
	bData := b.AsBool()

	outStrides := outShape.ComputeStrides()
	aStrides := computeBroadcastStridesForShape(aShape, outShape)
	bStrides := computeBroadcastStridesForShape(bShape, outShape)

	n := outShape.NumElements()
	for i := range n {
		aIdx := computeFlatIndex(i, outStrides, aStrides)
		bIdx := computeFlatIndex(i, outStrides, bStrides)
		dst[i] = aData[aIdx] && bData[bIdx]
	}
}
