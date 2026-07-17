package cpu

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// Cat concatenates tensors along the specified dimension.
//
// All tensors must have the same shape except along the concatenation dimension.
// Supports negative dim indexing (-1 = last dimension).
//
// Example:
//
//	a := tensor.Randn[float32]([]int{2, 3}, backend)
//	b := tensor.Randn[float32]([]int{2, 5}, backend)
//	c := backend.Cat([]*RawTensor{a, b}, 1) // Shape: [2, 8]
func (cpu *CPUBackend) Cat(tensors []*tensor.RawTensor, dim int) *tensor.RawTensor {
	if len(tensors) == 0 {
		panic("cat: at least one tensor required")
	}

	// Get first tensor properties
	shape := tensors[0].Shape()
	ndim := len(shape)
	dtype := tensors[0].DType()

	// Normalize negative dimension
	if dim < 0 {
		dim = ndim + dim
	}

	// Validate dimension
	if dim < 0 || dim >= ndim {
		panic(fmt.Sprintf("cat: dimension %d out of range for %dD tensor", dim, ndim))
	}

	// Validate shapes and calculate total size along concat dimension
	totalDim := 0
	for i, t := range tensors {
		tShape := t.Shape()
		if len(tShape) != ndim {
			panic(fmt.Sprintf("cat: tensor %d has %d dimensions, expected %d", i, len(tShape), ndim))
		}
		if t.DType() != dtype {
			panic(fmt.Sprintf("cat: tensor %d has dtype %s, expected %s", i, t.DType(), dtype))
		}

		// Check all dimensions except concat dim match
		for d := 0; d < ndim; d++ {
			if d == dim {
				totalDim += tShape[d]
			} else if tShape[d] != shape[d] {
				panic(fmt.Sprintf("cat: tensor %d dimension %d is %d, expected %d", i, d, tShape[d], shape[d]))
			}
		}
	}

	// Create output shape
	outShape := shape.Clone()
	outShape[dim] = totalDim

	// Create result tensor
	result, err := tensor.NewRaw(outShape, dtype, cpu.device)
	if err != nil {
		panic(fmt.Sprintf("cat: %v", err))
	}

	// Concatenate data
	switch dtype {
	case tensor.Float32:
		catFloat32(tensors, result, dim)
	case tensor.Float64:
		catFloat64(tensors, result, dim)
	case tensor.Int32:
		catInt32(tensors, result, dim)
	case tensor.Int64:
		catInt64(tensors, result, dim)
	case tensor.Uint8:
		catUint8(tensors, result, dim)
	case tensor.Bool:
		catBool(tensors, result, dim)
	default:
		panic(fmt.Sprintf("cat: unsupported dtype %s", dtype))
	}

	return result
}

// Chunk splits tensor into n equal parts along the specified dimension.
//
// The dimension size must be divisible by n.
// Supports negative dim indexing (-1 = last dimension).
//
// Example:
//
//	x := tensor.Randn[float32]([]int{2, 3, 6}, backend)
//	parts := backend.Chunk(x, 3, -1) // 3 tensors of shape [2, 3, 2]
func (cpu *CPUBackend) Chunk(x *tensor.RawTensor, n, dim int) []*tensor.RawTensor {
	if n <= 0 {
		panic(fmt.Sprintf("chunk: n must be positive, got %d", n))
	}

	shape := x.Shape()
	ndim := len(shape)

	// Normalize negative dimension
	if dim < 0 {
		dim = ndim + dim
	}

	// Validate dimension
	if dim < 0 || dim >= ndim {
		panic(fmt.Sprintf("chunk: dimension %d out of range for %dD tensor", dim, ndim))
	}

	// Check if dimension is divisible by n
	dimSize := shape[dim]
	if dimSize%n != 0 {
		panic(fmt.Sprintf("chunk: dimension %d size %d not divisible by %d", dim, dimSize, n))
	}

	chunkSize := dimSize / n

	// Create output shapes
	chunkShape := shape.Clone()
	chunkShape[dim] = chunkSize

	// Create result tensors
	results := make([]*tensor.RawTensor, n)
	for i := 0; i < n; i++ {
		chunk, err := tensor.NewRaw(chunkShape, x.DType(), cpu.device)
		if err != nil {
			panic(fmt.Sprintf("chunk: %v", err))
		}
		results[i] = chunk
	}

	// Split data
	switch x.DType() {
	case tensor.Float32:
		chunkFloat32(x, results, dim)
	case tensor.Float64:
		chunkFloat64(x, results, dim)
	case tensor.Int32:
		chunkInt32(x, results, dim)
	case tensor.Int64:
		chunkInt64(x, results, dim)
	case tensor.Uint8:
		chunkUint8(x, results, dim)
	case tensor.Bool:
		chunkBool(x, results, dim)
	default:
		panic(fmt.Sprintf("chunk: unsupported dtype %s", x.DType()))
	}

	return results
}

// Unsqueeze adds a dimension of size 1 at the specified position.
//
// Supports negative dim indexing.
// This is a view operation (reshape).
//
// Example:
//
//	x := tensor.Randn[float32]([]int{2, 3}, backend)
//	y := backend.Unsqueeze(x, 1)  // Shape: [2, 1, 3]
func (cpu *CPUBackend) Unsqueeze(x *tensor.RawTensor, dim int) *tensor.RawTensor {
	shape := x.Shape()
	ndim := len(shape)

	// Normalize negative dimension (for unsqueeze, valid range is [0, ndim])
	if dim < 0 {
		dim = ndim + 1 + dim
	}

	// Validate dimension
	if dim < 0 || dim > ndim {
		panic(fmt.Sprintf("unsqueeze: dimension %d out of range for %dD tensor (valid: [0, %d])", dim, ndim, ndim))
	}

	// Create new shape with dimension inserted
	newShape := make(tensor.Shape, ndim+1)
	for i := 0; i < dim; i++ {
		newShape[i] = shape[i]
	}
	newShape[dim] = 1
	for i := dim; i < ndim; i++ {
		newShape[i+1] = shape[i]
	}

	// Reshape (view operation)
	return cpu.Reshape(x, newShape)
}

// Squeeze removes a dimension of size 1 at the specified position.
//
// Panics if the dimension size is not 1.
// Supports negative dim indexing.
// This is a view operation (reshape).
//
// Example:
//
//	x := tensor.Randn[float32]([]int{2, 1, 3}, backend)
//	y := backend.Squeeze(x, 1)  // Shape: [2, 3]
func (cpu *CPUBackend) Squeeze(x *tensor.RawTensor, dim int) *tensor.RawTensor {
	shape := x.Shape()
	ndim := len(shape)

	// Normalize negative dimension
	if dim < 0 {
		dim = ndim + dim
	}

	// Validate dimension
	if dim < 0 || dim >= ndim {
		panic(fmt.Sprintf("squeeze: dimension %d out of range for %dD tensor", dim, ndim))
	}

	// Check dimension is size 1
	if shape[dim] != 1 {
		panic(fmt.Sprintf("squeeze: dimension %d has size %d, must be 1", dim, shape[dim]))
	}

	// Create new shape without the squeezed dimension
	newShape := make(tensor.Shape, 0, ndim-1)
	for i := 0; i < ndim; i++ {
		if i != dim {
			newShape = append(newShape, shape[i])
		}
	}

	// Reshape (view operation)
	return cpu.Reshape(x, newShape)
}

// innerOuter returns the product of the dimensions after dim (the size of the
// contiguous inner block) and before dim (the number of outer blocks) for a
// row-major tensor. Chunk and Cat along dim move whole inner blocks, so the
// per-element coordinate math reduces to contiguous slab copies.
func innerOuter(shape tensor.Shape, dim int) (inner, outer int) {
	inner, outer = 1, 1
	for d := dim + 1; d < len(shape); d++ {
		inner *= shape[d]
	}
	for d := 0; d < dim; d++ {
		outer *= shape[d]
	}
	return inner, outer
}

// catFloat32 concatenates float32 tensors along dim using contiguous slab copies
// (one copy per input tensor per outer block) instead of per-element scatter.
func catFloat32(tensors []*tensor.RawTensor, result *tensor.RawTensor, dim int) {
	outData := result.AsFloat32()
	outShape := result.Shape()
	inner, outer := innerOuter(outShape, dim)
	outDimStride := outShape[dim] * inner

	offset := 0
	for _, t := range tensors {
		data := t.AsFloat32()
		block := t.Shape()[dim] * inner
		for o := 0; o < outer; o++ {
			d := o*outDimStride + offset*inner
			copy(outData[d:d+block], data[o*block:o*block+block])
		}
		offset += t.Shape()[dim]
	}
}

// catFloat64 concatenates float64 tensors using contiguous slab copies.
func catFloat64(tensors []*tensor.RawTensor, result *tensor.RawTensor, dim int) {
	outData := result.AsFloat64()
	outShape := result.Shape()
	inner, outer := innerOuter(outShape, dim)
	outDimStride := outShape[dim] * inner

	offset := 0
	for _, t := range tensors {
		data := t.AsFloat64()
		block := t.Shape()[dim] * inner
		for o := 0; o < outer; o++ {
			d := o*outDimStride + offset*inner
			copy(outData[d:d+block], data[o*block:o*block+block])
		}
		offset += t.Shape()[dim]
	}
}

// catInt32 concatenates int32 tensors using contiguous slab copies.
func catInt32(tensors []*tensor.RawTensor, result *tensor.RawTensor, dim int) {
	outData := result.AsInt32()
	outShape := result.Shape()
	inner, outer := innerOuter(outShape, dim)
	outDimStride := outShape[dim] * inner

	offset := 0
	for _, t := range tensors {
		data := t.AsInt32()
		block := t.Shape()[dim] * inner
		for o := 0; o < outer; o++ {
			d := o*outDimStride + offset*inner
			copy(outData[d:d+block], data[o*block:o*block+block])
		}
		offset += t.Shape()[dim]
	}
}

// catInt64 concatenates int64 tensors using contiguous slab copies.
func catInt64(tensors []*tensor.RawTensor, result *tensor.RawTensor, dim int) {
	outData := result.AsInt64()
	outShape := result.Shape()
	inner, outer := innerOuter(outShape, dim)
	outDimStride := outShape[dim] * inner

	offset := 0
	for _, t := range tensors {
		data := t.AsInt64()
		block := t.Shape()[dim] * inner
		for o := 0; o < outer; o++ {
			d := o*outDimStride + offset*inner
			copy(outData[d:d+block], data[o*block:o*block+block])
		}
		offset += t.Shape()[dim]
	}
}

// catUint8 concatenates uint8 tensors using contiguous slab copies.
func catUint8(tensors []*tensor.RawTensor, result *tensor.RawTensor, dim int) {
	outData := result.AsUint8()
	outShape := result.Shape()
	inner, outer := innerOuter(outShape, dim)
	outDimStride := outShape[dim] * inner

	offset := 0
	for _, t := range tensors {
		data := t.AsUint8()
		block := t.Shape()[dim] * inner
		for o := 0; o < outer; o++ {
			d := o*outDimStride + offset*inner
			copy(outData[d:d+block], data[o*block:o*block+block])
		}
		offset += t.Shape()[dim]
	}
}

// catBool concatenates bool tensors using contiguous slab copies.
func catBool(tensors []*tensor.RawTensor, result *tensor.RawTensor, dim int) {
	outData := result.AsBool()
	outShape := result.Shape()
	inner, outer := innerOuter(outShape, dim)
	outDimStride := outShape[dim] * inner

	offset := 0
	for _, t := range tensors {
		data := t.AsBool()
		block := t.Shape()[dim] * inner
		for o := 0; o < outer; o++ {
			d := o*outDimStride + offset*inner
			copy(outData[d:d+block], data[o*block:o*block+block])
		}
		offset += t.Shape()[dim]
	}
}

// chunkFloat32 splits a float32 tensor into chunks along dim using contiguous
// slab copies (one copy per chunk per outer block) instead of per-element
// scatter with per-element coordinate allocation.
func chunkFloat32(x *tensor.RawTensor, results []*tensor.RawTensor, dim int) {
	data := x.AsFloat32()
	shape := x.Shape()
	inner, outer := innerOuter(shape, dim)
	srcDimStride := shape[dim] * inner
	for ci := range results {
		out := results[ci].AsFloat32()
		block := results[ci].Shape()[dim] * inner
		srcBase := ci * block
		for o := 0; o < outer; o++ {
			s := o*srcDimStride + srcBase
			copy(out[o*block:o*block+block], data[s:s+block])
		}
	}
}

// chunkFloat64 splits float64 tensor into chunks using contiguous slab copies.
func chunkFloat64(x *tensor.RawTensor, results []*tensor.RawTensor, dim int) {
	data := x.AsFloat64()
	shape := x.Shape()
	inner, outer := innerOuter(shape, dim)
	srcDimStride := shape[dim] * inner
	for ci := range results {
		out := results[ci].AsFloat64()
		block := results[ci].Shape()[dim] * inner
		srcBase := ci * block
		for o := 0; o < outer; o++ {
			s := o*srcDimStride + srcBase
			copy(out[o*block:o*block+block], data[s:s+block])
		}
	}
}

// chunkInt32 splits int32 tensor into chunks using contiguous slab copies.
func chunkInt32(x *tensor.RawTensor, results []*tensor.RawTensor, dim int) {
	data := x.AsInt32()
	shape := x.Shape()
	inner, outer := innerOuter(shape, dim)
	srcDimStride := shape[dim] * inner
	for ci := range results {
		out := results[ci].AsInt32()
		block := results[ci].Shape()[dim] * inner
		srcBase := ci * block
		for o := 0; o < outer; o++ {
			s := o*srcDimStride + srcBase
			copy(out[o*block:o*block+block], data[s:s+block])
		}
	}
}

// chunkInt64 splits int64 tensor into chunks using contiguous slab copies.
func chunkInt64(x *tensor.RawTensor, results []*tensor.RawTensor, dim int) {
	data := x.AsInt64()
	shape := x.Shape()
	inner, outer := innerOuter(shape, dim)
	srcDimStride := shape[dim] * inner
	for ci := range results {
		out := results[ci].AsInt64()
		block := results[ci].Shape()[dim] * inner
		srcBase := ci * block
		for o := 0; o < outer; o++ {
			s := o*srcDimStride + srcBase
			copy(out[o*block:o*block+block], data[s:s+block])
		}
	}
}

// chunkUint8 splits uint8 tensor into chunks using contiguous slab copies.
func chunkUint8(x *tensor.RawTensor, results []*tensor.RawTensor, dim int) {
	data := x.AsUint8()
	shape := x.Shape()
	inner, outer := innerOuter(shape, dim)
	srcDimStride := shape[dim] * inner
	for ci := range results {
		out := results[ci].AsUint8()
		block := results[ci].Shape()[dim] * inner
		srcBase := ci * block
		for o := 0; o < outer; o++ {
			s := o*srcDimStride + srcBase
			copy(out[o*block:o*block+block], data[s:s+block])
		}
	}
}

// chunkBool splits bool tensor into chunks using contiguous slab copies.
func chunkBool(x *tensor.RawTensor, results []*tensor.RawTensor, dim int) {
	data := x.AsBool()
	shape := x.Shape()
	inner, outer := innerOuter(shape, dim)
	srcDimStride := shape[dim] * inner
	for ci := range results {
		out := results[ci].AsBool()
		block := results[ci].Shape()[dim] * inner
		srcBase := ci * block
		for o := 0; o < outer; o++ {
			s := o*srcDimStride + srcBase
			copy(out[o*block:o*block+block], data[s:s+block])
		}
	}
}
