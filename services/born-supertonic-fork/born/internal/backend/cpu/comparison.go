package cpu

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// Comparison operations - return bool tensors.

// Greater returns a > b element-wise.
func (cpu *CPUBackend) Greater(a, b *tensor.RawTensor) *tensor.RawTensor {
	outShape, needsBroadcast, err := tensor.BroadcastShapes(a.Shape(), b.Shape())
	if err != nil {
		panic(fmt.Sprintf("greater: %v", err))
	}

	result, err := tensor.NewRaw(outShape, tensor.Bool, cpu.device)
	if err != nil {
		panic(fmt.Sprintf("greater: %v", err))
	}

	if !needsBroadcast && a.Shape().Equal(b.Shape()) {
		greaterVectorized(result, a, b)
	} else {
		greaterWithBroadcast(result, a, b, outShape)
	}

	return result
}

// Lower returns a < b element-wise.
func (cpu *CPUBackend) Lower(a, b *tensor.RawTensor) *tensor.RawTensor {
	outShape, needsBroadcast, err := tensor.BroadcastShapes(a.Shape(), b.Shape())
	if err != nil {
		panic(fmt.Sprintf("lower: %v", err))
	}

	result, err := tensor.NewRaw(outShape, tensor.Bool, cpu.device)
	if err != nil {
		panic(fmt.Sprintf("lower: %v", err))
	}

	if !needsBroadcast && a.Shape().Equal(b.Shape()) {
		lowerVectorized(result, a, b)
	} else {
		lowerWithBroadcast(result, a, b, outShape)
	}

	return result
}

// GreaterEqual returns a >= b element-wise.
func (cpu *CPUBackend) GreaterEqual(a, b *tensor.RawTensor) *tensor.RawTensor {
	outShape, needsBroadcast, err := tensor.BroadcastShapes(a.Shape(), b.Shape())
	if err != nil {
		panic(fmt.Sprintf("greaterEqual: %v", err))
	}

	result, err := tensor.NewRaw(outShape, tensor.Bool, cpu.device)
	if err != nil {
		panic(fmt.Sprintf("greaterEqual: %v", err))
	}

	if !needsBroadcast && a.Shape().Equal(b.Shape()) {
		greaterEqualVectorized(result, a, b)
	} else {
		greaterEqualWithBroadcast(result, a, b, outShape)
	}

	return result
}

// LowerEqual returns a <= b element-wise.
func (cpu *CPUBackend) LowerEqual(a, b *tensor.RawTensor) *tensor.RawTensor {
	outShape, needsBroadcast, err := tensor.BroadcastShapes(a.Shape(), b.Shape())
	if err != nil {
		panic(fmt.Sprintf("lowerEqual: %v", err))
	}

	result, err := tensor.NewRaw(outShape, tensor.Bool, cpu.device)
	if err != nil {
		panic(fmt.Sprintf("lowerEqual: %v", err))
	}

	if !needsBroadcast && a.Shape().Equal(b.Shape()) {
		lowerEqualVectorized(result, a, b)
	} else {
		lowerEqualWithBroadcast(result, a, b, outShape)
	}

	return result
}

// Equal returns a == b element-wise.
func (cpu *CPUBackend) Equal(a, b *tensor.RawTensor) *tensor.RawTensor {
	outShape, needsBroadcast, err := tensor.BroadcastShapes(a.Shape(), b.Shape())
	if err != nil {
		panic(fmt.Sprintf("equal: %v", err))
	}

	result, err := tensor.NewRaw(outShape, tensor.Bool, cpu.device)
	if err != nil {
		panic(fmt.Sprintf("equal: %v", err))
	}

	if !needsBroadcast && a.Shape().Equal(b.Shape()) {
		equalVectorized(result, a, b)
	} else {
		equalWithBroadcast(result, a, b, outShape)
	}

	return result
}

// NotEqual returns a != b element-wise.
func (cpu *CPUBackend) NotEqual(a, b *tensor.RawTensor) *tensor.RawTensor {
	outShape, needsBroadcast, err := tensor.BroadcastShapes(a.Shape(), b.Shape())
	if err != nil {
		panic(fmt.Sprintf("notEqual: %v", err))
	}

	result, err := tensor.NewRaw(outShape, tensor.Bool, cpu.device)
	if err != nil {
		panic(fmt.Sprintf("notEqual: %v", err))
	}

	if !needsBroadcast && a.Shape().Equal(b.Shape()) {
		notEqualVectorized(result, a, b)
	} else {
		notEqualWithBroadcast(result, a, b, outShape)
	}

	return result
}

// ============================================================================
// Vectorized implementations (same shape)
// ============================================================================

func greaterVectorized(result, a, b *tensor.RawTensor) {
	dst := result.AsBool()
	switch a.DType() {
	case tensor.Float32:
		aData, bData := a.AsFloat32(), b.AsFloat32()
		for i := range dst {
			dst[i] = aData[i] > bData[i]
		}
	case tensor.Float64:
		aData, bData := a.AsFloat64(), b.AsFloat64()
		for i := range dst {
			dst[i] = aData[i] > bData[i]
		}
	case tensor.Int32:
		aData, bData := a.AsInt32(), b.AsInt32()
		for i := range dst {
			dst[i] = aData[i] > bData[i]
		}
	case tensor.Int64:
		aData, bData := a.AsInt64(), b.AsInt64()
		for i := range dst {
			dst[i] = aData[i] > bData[i]
		}
	}
}

func lowerVectorized(result, a, b *tensor.RawTensor) {
	dst := result.AsBool()
	switch a.DType() {
	case tensor.Float32:
		aData, bData := a.AsFloat32(), b.AsFloat32()
		for i := range dst {
			dst[i] = aData[i] < bData[i]
		}
	case tensor.Float64:
		aData, bData := a.AsFloat64(), b.AsFloat64()
		for i := range dst {
			dst[i] = aData[i] < bData[i]
		}
	case tensor.Int32:
		aData, bData := a.AsInt32(), b.AsInt32()
		for i := range dst {
			dst[i] = aData[i] < bData[i]
		}
	case tensor.Int64:
		aData, bData := a.AsInt64(), b.AsInt64()
		for i := range dst {
			dst[i] = aData[i] < bData[i]
		}
	}
}

func greaterEqualVectorized(result, a, b *tensor.RawTensor) {
	dst := result.AsBool()
	switch a.DType() {
	case tensor.Float32:
		aData, bData := a.AsFloat32(), b.AsFloat32()
		for i := range dst {
			dst[i] = aData[i] >= bData[i]
		}
	case tensor.Float64:
		aData, bData := a.AsFloat64(), b.AsFloat64()
		for i := range dst {
			dst[i] = aData[i] >= bData[i]
		}
	case tensor.Int32:
		aData, bData := a.AsInt32(), b.AsInt32()
		for i := range dst {
			dst[i] = aData[i] >= bData[i]
		}
	case tensor.Int64:
		aData, bData := a.AsInt64(), b.AsInt64()
		for i := range dst {
			dst[i] = aData[i] >= bData[i]
		}
	}
}

func lowerEqualVectorized(result, a, b *tensor.RawTensor) {
	dst := result.AsBool()
	switch a.DType() {
	case tensor.Float32:
		aData, bData := a.AsFloat32(), b.AsFloat32()
		for i := range dst {
			dst[i] = aData[i] <= bData[i]
		}
	case tensor.Float64:
		aData, bData := a.AsFloat64(), b.AsFloat64()
		for i := range dst {
			dst[i] = aData[i] <= bData[i]
		}
	case tensor.Int32:
		aData, bData := a.AsInt32(), b.AsInt32()
		for i := range dst {
			dst[i] = aData[i] <= bData[i]
		}
	case tensor.Int64:
		aData, bData := a.AsInt64(), b.AsInt64()
		for i := range dst {
			dst[i] = aData[i] <= bData[i]
		}
	}
}

func equalVectorized(result, a, b *tensor.RawTensor) {
	dst := result.AsBool()
	switch a.DType() {
	case tensor.Float32:
		aData, bData := a.AsFloat32(), b.AsFloat32()
		for i := range dst {
			dst[i] = aData[i] == bData[i]
		}
	case tensor.Float64:
		aData, bData := a.AsFloat64(), b.AsFloat64()
		for i := range dst {
			dst[i] = aData[i] == bData[i]
		}
	case tensor.Int32:
		aData, bData := a.AsInt32(), b.AsInt32()
		for i := range dst {
			dst[i] = aData[i] == bData[i]
		}
	case tensor.Int64:
		aData, bData := a.AsInt64(), b.AsInt64()
		for i := range dst {
			dst[i] = aData[i] == bData[i]
		}
	case tensor.Bool:
		aData, bData := a.AsBool(), b.AsBool()
		for i := range dst {
			dst[i] = aData[i] == bData[i]
		}
	}
}

func notEqualVectorized(result, a, b *tensor.RawTensor) {
	dst := result.AsBool()
	switch a.DType() {
	case tensor.Float32:
		aData, bData := a.AsFloat32(), b.AsFloat32()
		for i := range dst {
			dst[i] = aData[i] != bData[i]
		}
	case tensor.Float64:
		aData, bData := a.AsFloat64(), b.AsFloat64()
		for i := range dst {
			dst[i] = aData[i] != bData[i]
		}
	case tensor.Int32:
		aData, bData := a.AsInt32(), b.AsInt32()
		for i := range dst {
			dst[i] = aData[i] != bData[i]
		}
	case tensor.Int64:
		aData, bData := a.AsInt64(), b.AsInt64()
		for i := range dst {
			dst[i] = aData[i] != bData[i]
		}
	case tensor.Bool:
		aData, bData := a.AsBool(), b.AsBool()
		for i := range dst {
			dst[i] = aData[i] != bData[i]
		}
	}
}

// ============================================================================
// Broadcast implementations (different shapes)
// ============================================================================

func greaterWithBroadcast(result, a, b *tensor.RawTensor, outShape tensor.Shape) {
	dst := result.AsBool()
	aShape := a.Shape()
	bShape := b.Shape()

	outStrides := outShape.ComputeStrides()
	aStrides := computeBroadcastStridesForShape(aShape, outShape)
	bStrides := computeBroadcastStridesForShape(bShape, outShape)

	n := outShape.NumElements()
	switch a.DType() {
	case tensor.Float32:
		aData, bData := a.AsFloat32(), b.AsFloat32()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] > bData[bIdx]
		}
	case tensor.Float64:
		aData, bData := a.AsFloat64(), b.AsFloat64()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] > bData[bIdx]
		}
	case tensor.Int32:
		aData, bData := a.AsInt32(), b.AsInt32()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] > bData[bIdx]
		}
	case tensor.Int64:
		aData, bData := a.AsInt64(), b.AsInt64()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] > bData[bIdx]
		}
	}
}

func lowerWithBroadcast(result, a, b *tensor.RawTensor, outShape tensor.Shape) {
	dst := result.AsBool()
	aShape := a.Shape()
	bShape := b.Shape()

	outStrides := outShape.ComputeStrides()
	aStrides := computeBroadcastStridesForShape(aShape, outShape)
	bStrides := computeBroadcastStridesForShape(bShape, outShape)

	n := outShape.NumElements()
	switch a.DType() {
	case tensor.Float32:
		aData, bData := a.AsFloat32(), b.AsFloat32()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] < bData[bIdx]
		}
	case tensor.Float64:
		aData, bData := a.AsFloat64(), b.AsFloat64()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] < bData[bIdx]
		}
	case tensor.Int32:
		aData, bData := a.AsInt32(), b.AsInt32()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] < bData[bIdx]
		}
	case tensor.Int64:
		aData, bData := a.AsInt64(), b.AsInt64()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] < bData[bIdx]
		}
	}
}

func greaterEqualWithBroadcast(result, a, b *tensor.RawTensor, outShape tensor.Shape) {
	dst := result.AsBool()
	aShape := a.Shape()
	bShape := b.Shape()

	outStrides := outShape.ComputeStrides()
	aStrides := computeBroadcastStridesForShape(aShape, outShape)
	bStrides := computeBroadcastStridesForShape(bShape, outShape)

	n := outShape.NumElements()
	switch a.DType() {
	case tensor.Float32:
		aData, bData := a.AsFloat32(), b.AsFloat32()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] >= bData[bIdx]
		}
	case tensor.Float64:
		aData, bData := a.AsFloat64(), b.AsFloat64()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] >= bData[bIdx]
		}
	case tensor.Int32:
		aData, bData := a.AsInt32(), b.AsInt32()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] >= bData[bIdx]
		}
	case tensor.Int64:
		aData, bData := a.AsInt64(), b.AsInt64()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] >= bData[bIdx]
		}
	}
}

func lowerEqualWithBroadcast(result, a, b *tensor.RawTensor, outShape tensor.Shape) {
	dst := result.AsBool()
	aShape := a.Shape()
	bShape := b.Shape()

	outStrides := outShape.ComputeStrides()
	aStrides := computeBroadcastStridesForShape(aShape, outShape)
	bStrides := computeBroadcastStridesForShape(bShape, outShape)

	n := outShape.NumElements()
	switch a.DType() {
	case tensor.Float32:
		aData, bData := a.AsFloat32(), b.AsFloat32()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] <= bData[bIdx]
		}
	case tensor.Float64:
		aData, bData := a.AsFloat64(), b.AsFloat64()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] <= bData[bIdx]
		}
	case tensor.Int32:
		aData, bData := a.AsInt32(), b.AsInt32()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] <= bData[bIdx]
		}
	case tensor.Int64:
		aData, bData := a.AsInt64(), b.AsInt64()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] <= bData[bIdx]
		}
	}
}

func equalWithBroadcast(result, a, b *tensor.RawTensor, outShape tensor.Shape) {
	dst := result.AsBool()
	aShape := a.Shape()
	bShape := b.Shape()

	outStrides := outShape.ComputeStrides()
	aStrides := computeBroadcastStridesForShape(aShape, outShape)
	bStrides := computeBroadcastStridesForShape(bShape, outShape)

	n := outShape.NumElements()
	switch a.DType() {
	case tensor.Float32:
		aData, bData := a.AsFloat32(), b.AsFloat32()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] == bData[bIdx]
		}
	case tensor.Float64:
		aData, bData := a.AsFloat64(), b.AsFloat64()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] == bData[bIdx]
		}
	case tensor.Int32:
		aData, bData := a.AsInt32(), b.AsInt32()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] == bData[bIdx]
		}
	case tensor.Int64:
		aData, bData := a.AsInt64(), b.AsInt64()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] == bData[bIdx]
		}
	case tensor.Bool:
		aData, bData := a.AsBool(), b.AsBool()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] == bData[bIdx]
		}
	}
}

func notEqualWithBroadcast(result, a, b *tensor.RawTensor, outShape tensor.Shape) {
	dst := result.AsBool()
	aShape := a.Shape()
	bShape := b.Shape()

	outStrides := outShape.ComputeStrides()
	aStrides := computeBroadcastStridesForShape(aShape, outShape)
	bStrides := computeBroadcastStridesForShape(bShape, outShape)

	n := outShape.NumElements()
	switch a.DType() {
	case tensor.Float32:
		aData, bData := a.AsFloat32(), b.AsFloat32()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] != bData[bIdx]
		}
	case tensor.Float64:
		aData, bData := a.AsFloat64(), b.AsFloat64()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] != bData[bIdx]
		}
	case tensor.Int32:
		aData, bData := a.AsInt32(), b.AsInt32()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] != bData[bIdx]
		}
	case tensor.Int64:
		aData, bData := a.AsInt64(), b.AsInt64()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] != bData[bIdx]
		}
	case tensor.Bool:
		aData, bData := a.AsBool(), b.AsBool()
		for i := range n {
			aIdx := computeFlatIndex(i, outStrides, aStrides)
			bIdx := computeFlatIndex(i, outStrides, bStrides)
			dst[i] = aData[aIdx] != bData[bIdx]
		}
	}
}
