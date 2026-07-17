package cpu

import (
	"github.com/born-ml/born/internal/tensor"
)

// addInplace performs inplace addition (a += b).
// Requires: a.Shape().Equal(b.Shape()) && a.IsUnique().
func addInplace(a, b *tensor.RawTensor) {
	switch a.DType() {
	case tensor.Float32:
		addInplaceFloat32(a.AsFloat32(), b.AsFloat32())
	case tensor.Float64:
		addInplaceFloat64(a.AsFloat64(), b.AsFloat64())
	case tensor.Int32:
		addInplaceInt32(a.AsInt32(), b.AsInt32())
	case tensor.Int64:
		addInplaceInt64(a.AsInt64(), b.AsInt64())
	default:
		panic("addInplace: unsupported dtype")
	}
}

// addVectorized performs vectorized addition: result = a + b.
// Requires: a.Shape().Equal(b.Shape()).
func addVectorized(result, a, b *tensor.RawTensor) {
	switch a.DType() {
	case tensor.Float32:
		addVectorizedFloat32(result.AsFloat32(), a.AsFloat32(), b.AsFloat32())
	case tensor.Float64:
		addVectorizedFloat64(result.AsFloat64(), a.AsFloat64(), b.AsFloat64())
	case tensor.Int32:
		addVectorizedInt32(result.AsInt32(), a.AsInt32(), b.AsInt32())
	case tensor.Int64:
		addVectorizedInt64(result.AsInt64(), a.AsInt64(), b.AsInt64())
	default:
		panic("addVectorized: unsupported dtype")
	}
}

// addWithBroadcast performs addition with broadcasting.
func addWithBroadcast(result, a, b *tensor.RawTensor, outShape tensor.Shape) {
	switch a.DType() {
	case tensor.Float32:
		addBroadcastFloat32(result.AsFloat32(), a.AsFloat32(), b.AsFloat32(), a.Shape(), b.Shape(), outShape)
	case tensor.Float64:
		addBroadcastFloat64(result.AsFloat64(), a.AsFloat64(), b.AsFloat64(), a.Shape(), b.Shape(), outShape)
	case tensor.Int32:
		addBroadcastInt32(result.AsInt32(), a.AsInt32(), b.AsInt32(), a.Shape(), b.Shape(), outShape)
	case tensor.Int64:
		addBroadcastInt64(result.AsInt64(), a.AsInt64(), b.AsInt64(), a.Shape(), b.Shape(), outShape)
	default:
		panic("addWithBroadcast: unsupported dtype")
	}
}

// Similar functions for sub, mul, div.
func subInplace(a, b *tensor.RawTensor) {
	switch a.DType() {
	case tensor.Float32:
		subInplaceFloat32(a.AsFloat32(), b.AsFloat32())
	case tensor.Float64:
		subInplaceFloat64(a.AsFloat64(), b.AsFloat64())
	case tensor.Int32:
		subInplaceInt32(a.AsInt32(), b.AsInt32())
	case tensor.Int64:
		subInplaceInt64(a.AsInt64(), b.AsInt64())
	default:
		panic("subInplace: unsupported dtype")
	}
}

func subVectorized(result, a, b *tensor.RawTensor) {
	switch a.DType() {
	case tensor.Float32:
		subVectorizedFloat32(result.AsFloat32(), a.AsFloat32(), b.AsFloat32())
	case tensor.Float64:
		subVectorizedFloat64(result.AsFloat64(), a.AsFloat64(), b.AsFloat64())
	case tensor.Int32:
		subVectorizedInt32(result.AsInt32(), a.AsInt32(), b.AsInt32())
	case tensor.Int64:
		subVectorizedInt64(result.AsInt64(), a.AsInt64(), b.AsInt64())
	default:
		panic("subVectorized: unsupported dtype")
	}
}

func subWithBroadcast(result, a, b *tensor.RawTensor, outShape tensor.Shape) {
	switch a.DType() {
	case tensor.Float32:
		subBroadcastFloat32(result.AsFloat32(), a.AsFloat32(), b.AsFloat32(), a.Shape(), b.Shape(), outShape)
	case tensor.Float64:
		subBroadcastFloat64(result.AsFloat64(), a.AsFloat64(), b.AsFloat64(), a.Shape(), b.Shape(), outShape)
	case tensor.Int32:
		subBroadcastInt32(result.AsInt32(), a.AsInt32(), b.AsInt32(), a.Shape(), b.Shape(), outShape)
	case tensor.Int64:
		subBroadcastInt64(result.AsInt64(), a.AsInt64(), b.AsInt64(), a.Shape(), b.Shape(), outShape)
	default:
		panic("subWithBroadcast: unsupported dtype")
	}
}

func mulInplace(a, b *tensor.RawTensor) {
	switch a.DType() {
	case tensor.Float32:
		mulInplaceFloat32(a.AsFloat32(), b.AsFloat32())
	case tensor.Float64:
		mulInplaceFloat64(a.AsFloat64(), b.AsFloat64())
	case tensor.Int32:
		mulInplaceInt32(a.AsInt32(), b.AsInt32())
	case tensor.Int64:
		mulInplaceInt64(a.AsInt64(), b.AsInt64())
	default:
		panic("mulInplace: unsupported dtype")
	}
}

func mulVectorized(result, a, b *tensor.RawTensor) {
	switch a.DType() {
	case tensor.Float32:
		mulVectorizedFloat32(result.AsFloat32(), a.AsFloat32(), b.AsFloat32())
	case tensor.Float64:
		mulVectorizedFloat64(result.AsFloat64(), a.AsFloat64(), b.AsFloat64())
	case tensor.Int32:
		mulVectorizedInt32(result.AsInt32(), a.AsInt32(), b.AsInt32())
	case tensor.Int64:
		mulVectorizedInt64(result.AsInt64(), a.AsInt64(), b.AsInt64())
	default:
		panic("mulVectorized: unsupported dtype")
	}
}

func mulWithBroadcast(result, a, b *tensor.RawTensor, outShape tensor.Shape) {
	switch a.DType() {
	case tensor.Float32:
		mulBroadcastFloat32(result.AsFloat32(), a.AsFloat32(), b.AsFloat32(), a.Shape(), b.Shape(), outShape)
	case tensor.Float64:
		mulBroadcastFloat64(result.AsFloat64(), a.AsFloat64(), b.AsFloat64(), a.Shape(), b.Shape(), outShape)
	case tensor.Int32:
		mulBroadcastInt32(result.AsInt32(), a.AsInt32(), b.AsInt32(), a.Shape(), b.Shape(), outShape)
	case tensor.Int64:
		mulBroadcastInt64(result.AsInt64(), a.AsInt64(), b.AsInt64(), a.Shape(), b.Shape(), outShape)
	default:
		panic("mulWithBroadcast: unsupported dtype")
	}
}

func divInplace(a, b *tensor.RawTensor) {
	switch a.DType() {
	case tensor.Float32:
		divInplaceFloat32(a.AsFloat32(), b.AsFloat32())
	case tensor.Float64:
		divInplaceFloat64(a.AsFloat64(), b.AsFloat64())
	case tensor.Int32:
		divInplaceInt32(a.AsInt32(), b.AsInt32())
	case tensor.Int64:
		divInplaceInt64(a.AsInt64(), b.AsInt64())
	default:
		panic("divInplace: unsupported dtype")
	}
}

func divVectorized(result, a, b *tensor.RawTensor) {
	switch a.DType() {
	case tensor.Float32:
		divVectorizedFloat32(result.AsFloat32(), a.AsFloat32(), b.AsFloat32())
	case tensor.Float64:
		divVectorizedFloat64(result.AsFloat64(), a.AsFloat64(), b.AsFloat64())
	case tensor.Int32:
		divVectorizedInt32(result.AsInt32(), a.AsInt32(), b.AsInt32())
	case tensor.Int64:
		divVectorizedInt64(result.AsInt64(), a.AsInt64(), b.AsInt64())
	default:
		panic("divVectorized: unsupported dtype")
	}
}

func divWithBroadcast(result, a, b *tensor.RawTensor, outShape tensor.Shape) {
	switch a.DType() {
	case tensor.Float32:
		divBroadcastFloat32(result.AsFloat32(), a.AsFloat32(), b.AsFloat32(), a.Shape(), b.Shape(), outShape)
	case tensor.Float64:
		divBroadcastFloat64(result.AsFloat64(), a.AsFloat64(), b.AsFloat64(), a.Shape(), b.Shape(), outShape)
	case tensor.Int32:
		divBroadcastInt32(result.AsInt32(), a.AsInt32(), b.AsInt32(), a.Shape(), b.Shape(), outShape)
	case tensor.Int64:
		divBroadcastInt64(result.AsInt64(), a.AsInt64(), b.AsInt64(), a.Shape(), b.Shape(), outShape)
	default:
		panic("divWithBroadcast: unsupported dtype")
	}
}

func transposeData(result, src *tensor.RawTensor, axes []int) {
	switch src.DType() {
	case tensor.Float32:
		transposeFloat32(result.AsFloat32(), src.AsFloat32(), src.Shape(), axes)
	case tensor.Float64:
		transposeFloat64(result.AsFloat64(), src.AsFloat64(), src.Shape(), axes)
	case tensor.Int32:
		transposeInt32(result.AsInt32(), src.AsInt32(), src.Shape(), axes)
	case tensor.Int64:
		transposeInt64(result.AsInt64(), src.AsInt64(), src.Shape(), axes)
	default:
		panic("transpose: unsupported dtype")
	}
}
