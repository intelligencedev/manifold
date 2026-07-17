package cpu

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// Gather selects elements along dim using index tensor.
// Similar to torch.gather(input, dim, index).
//
// The index tensor must have dtype int32 and its shape must match input shape
// except at the gather dimension, where it can differ.
//
// Example:
//
//	input: [3, 4, 5] with values
//	index: [3, 4, 2] (int32 indices)
//	dim: 2
//	output: [3, 4, 2] where output[i,j,k] = input[i,j,index[i,j,k]]
//
//nolint:cyclop // Type-specific dispatch for gather operation (5 dtypes)
func (cpu *CPUBackend) Gather(x *tensor.RawTensor, dim int, index *tensor.RawTensor) *tensor.RawTensor {
	// Validate index dtype
	if index.DType() != tensor.Int32 {
		panic(fmt.Sprintf("gather: index tensor must have dtype int32, got %s", index.DType()))
	}

	// Normalize dimension
	ndim := len(x.Shape())
	if dim < 0 {
		dim += ndim
	}
	if dim < 0 || dim >= ndim {
		panic(fmt.Sprintf("gather: invalid dim %d for %dD tensor", dim, ndim))
	}

	// Validate index shape (must match input except at gather dim)
	indexShape := index.Shape()
	if len(indexShape) != ndim {
		panic(fmt.Sprintf("gather: index rank %d != input rank %d", len(indexShape), ndim))
	}
	for i := 0; i < ndim; i++ {
		if i != dim && indexShape[i] != x.Shape()[i] {
			panic(fmt.Sprintf("gather: index shape mismatch at dim %d: %d != %d",
				i, indexShape[i], x.Shape()[i]))
		}
	}

	// Create output tensor (same shape as index)
	result, err := tensor.NewRaw(indexShape, x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("gather: failed to create result tensor: %v", err))
	}

	// Perform gather
	indices := index.AsInt32()
	switch x.DType() {
	case tensor.Float32:
		gatherFloat32(result.AsFloat32(), x.AsFloat32(), indices, x.Shape(), indexShape, dim)
	case tensor.Float64:
		gatherFloat64(result.AsFloat64(), x.AsFloat64(), indices, x.Shape(), indexShape, dim)
	case tensor.Int32:
		gatherInt32(result.AsInt32(), x.AsInt32(), indices, x.Shape(), indexShape, dim)
	case tensor.Int64:
		gatherInt64(result.AsInt64(), x.AsInt64(), indices, x.Shape(), indexShape, dim)
	case tensor.Uint8:
		gatherUInt8(result.AsUint8(), x.AsUint8(), indices, x.Shape(), indexShape, dim)
	default:
		panic(fmt.Sprintf("gather: unsupported dtype %s", x.DType()))
	}

	return result
}

//nolint:dupl // Type-specific gather implementation for float32
func gatherFloat32(dst, src []float32, indices []int32, srcShape, dstShape tensor.Shape, dim int) {
	ndim := len(srcShape)
	dstStrides := dstShape.ComputeStrides()
	srcStrides := srcShape.ComputeStrides()

	for i := range dst {
		// Convert flat index to multi-dimensional index
		multiIdx := make([]int, ndim)
		remaining := i
		for d := 0; d < ndim; d++ {
			multiIdx[d] = remaining / dstStrides[d]
			remaining %= dstStrides[d]
		}

		// Get the index value at the gather dimension
		indexVal := int(indices[i])
		if indexVal < 0 || indexVal >= srcShape[dim] {
			panic(fmt.Sprintf("gather: index %d out of bounds [0, %d) at position %d",
				indexVal, srcShape[dim], i))
		}

		// Compute source flat index
		srcIdx := 0
		for d := 0; d < ndim; d++ {
			if d == dim {
				srcIdx += indexVal * srcStrides[d]
			} else {
				srcIdx += multiIdx[d] * srcStrides[d]
			}
		}

		dst[i] = src[srcIdx]
	}
}

//nolint:dupl // Type-specific gather implementation for float64
func gatherFloat64(dst, src []float64, indices []int32, srcShape, dstShape tensor.Shape, dim int) {
	ndim := len(srcShape)
	dstStrides := dstShape.ComputeStrides()
	srcStrides := srcShape.ComputeStrides()

	for i := range dst {
		multiIdx := make([]int, ndim)
		remaining := i
		for d := 0; d < ndim; d++ {
			multiIdx[d] = remaining / dstStrides[d]
			remaining %= dstStrides[d]
		}

		indexVal := int(indices[i])
		if indexVal < 0 || indexVal >= srcShape[dim] {
			panic(fmt.Sprintf("gather: index %d out of bounds [0, %d) at position %d",
				indexVal, srcShape[dim], i))
		}

		srcIdx := 0
		for d := 0; d < ndim; d++ {
			if d == dim {
				srcIdx += indexVal * srcStrides[d]
			} else {
				srcIdx += multiIdx[d] * srcStrides[d]
			}
		}

		dst[i] = src[srcIdx]
	}
}

func gatherInt32(dst, src, indices []int32, srcShape, dstShape tensor.Shape, dim int) {
	ndim := len(srcShape)
	dstStrides := dstShape.ComputeStrides()
	srcStrides := srcShape.ComputeStrides()

	for i := range dst {
		multiIdx := make([]int, ndim)
		remaining := i
		for d := 0; d < ndim; d++ {
			multiIdx[d] = remaining / dstStrides[d]
			remaining %= dstStrides[d]
		}

		indexVal := int(indices[i])
		if indexVal < 0 || indexVal >= srcShape[dim] {
			panic(fmt.Sprintf("gather: index %d out of bounds [0, %d) at position %d",
				indexVal, srcShape[dim], i))
		}

		srcIdx := 0
		for d := 0; d < ndim; d++ {
			if d == dim {
				srcIdx += indexVal * srcStrides[d]
			} else {
				srcIdx += multiIdx[d] * srcStrides[d]
			}
		}

		dst[i] = src[srcIdx]
	}
}

//nolint:dupl // Type-specific gather implementation for int64
func gatherInt64(dst, src []int64, indices []int32, srcShape, dstShape tensor.Shape, dim int) {
	ndim := len(srcShape)
	dstStrides := dstShape.ComputeStrides()
	srcStrides := srcShape.ComputeStrides()

	for i := range dst {
		multiIdx := make([]int, ndim)
		remaining := i
		for d := 0; d < ndim; d++ {
			multiIdx[d] = remaining / dstStrides[d]
			remaining %= dstStrides[d]
		}

		indexVal := int(indices[i])
		if indexVal < 0 || indexVal >= srcShape[dim] {
			panic(fmt.Sprintf("gather: index %d out of bounds [0, %d) at position %d",
				indexVal, srcShape[dim], i))
		}

		srcIdx := 0
		for d := 0; d < ndim; d++ {
			if d == dim {
				srcIdx += indexVal * srcStrides[d]
			} else {
				srcIdx += multiIdx[d] * srcStrides[d]
			}
		}

		dst[i] = src[srcIdx]
	}
}

//nolint:dupl // Type-specific gather implementation for uint8
func gatherUInt8(dst, src []uint8, indices []int32, srcShape, dstShape tensor.Shape, dim int) {
	ndim := len(srcShape)
	dstStrides := dstShape.ComputeStrides()
	srcStrides := srcShape.ComputeStrides()

	for i := range dst {
		multiIdx := make([]int, ndim)
		remaining := i
		for d := 0; d < ndim; d++ {
			multiIdx[d] = remaining / dstStrides[d]
			remaining %= dstStrides[d]
		}

		indexVal := int(indices[i])
		if indexVal < 0 || indexVal >= srcShape[dim] {
			panic(fmt.Sprintf("gather: index %d out of bounds [0, %d) at position %d",
				indexVal, srcShape[dim], i))
		}

		srcIdx := 0
		for d := 0; d < ndim; d++ {
			if d == dim {
				srcIdx += indexVal * srcStrides[d]
			} else {
				srcIdx += multiIdx[d] * srcStrides[d]
			}
		}

		dst[i] = src[srcIdx]
	}
}

// Where performs conditional element selection.
// Similar to torch.where(condition, x, y).
//
// Returns a tensor where each element is selected from x if condition is true,
// otherwise from y. All tensors must have compatible shapes (broadcasting supported).
//
// Example:
//
//	condition: [3, 4] (bool tensor)
//	x: [3, 4] (float32)
//	y: [3, 4] (float32)
//	output: [3, 4] where output[i,j] = condition[i,j] ? x[i,j] : y[i,j]
func (cpu *CPUBackend) Where(condition, x, y *tensor.RawTensor) *tensor.RawTensor {
	// Validate condition dtype (must be bool or uint8)
	if condition.DType() != tensor.Bool && condition.DType() != tensor.Uint8 {
		panic(fmt.Sprintf("where: condition must be bool or uint8, got %s", condition.DType()))
	}

	// Validate x and y have same dtype
	if x.DType() != y.DType() {
		panic(fmt.Sprintf("where: x and y must have same dtype, got %s and %s",
			x.DType(), y.DType()))
	}

	// Compute output shape (broadcast all three tensors)
	outShape1, _, err := tensor.BroadcastShapes(condition.Shape(), x.Shape())
	if err != nil {
		panic(fmt.Sprintf("where: failed to broadcast condition and x: %v", err))
	}
	outShape, _, err := tensor.BroadcastShapes(outShape1, y.Shape())
	if err != nil {
		panic(fmt.Sprintf("where: failed to broadcast with y: %v", err))
	}

	// Create output tensor
	result, err := tensor.NewRaw(outShape, x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("where: failed to create result tensor: %v", err))
	}

	// Perform conditional selection
	switch x.DType() {
	case tensor.Float32:
		whereFloat32(result.AsFloat32(), condition, x.AsFloat32(), y.AsFloat32(),
			outShape, condition.Shape(), x.Shape(), y.Shape())
	case tensor.Float64:
		whereFloat64(result.AsFloat64(), condition, x.AsFloat64(), y.AsFloat64(),
			outShape, condition.Shape(), x.Shape(), y.Shape())
	case tensor.Int32:
		whereInt32(result.AsInt32(), condition, x.AsInt32(), y.AsInt32(),
			outShape, condition.Shape(), x.Shape(), y.Shape())
	case tensor.Int64:
		whereInt64(result.AsInt64(), condition, x.AsInt64(), y.AsInt64(),
			outShape, condition.Shape(), x.Shape(), y.Shape())
	case tensor.Uint8:
		whereUInt8(result.AsUint8(), condition, x.AsUint8(), y.AsUint8(),
			outShape, condition.Shape(), x.Shape(), y.Shape())
	default:
		panic(fmt.Sprintf("where: unsupported dtype %s", x.DType()))
	}

	return result
}

// whereFloat32 performs where operation for float32 data.
func whereFloat32(dst []float32, condition *tensor.RawTensor, xData, yData []float32,
	outShape, condShape, xShape, yShape tensor.Shape) {
	outStrides := outShape.ComputeStrides()
	condStrides := condShape.ComputeStrides()
	xStrides := xShape.ComputeStrides()
	yStrides := yShape.ComputeStrides()

	// Get condition data (handle both bool and uint8)
	condData := getConditionAsUint8(condition)

	for i := range dst {
		// Convert flat index to multi-dimensional index
		multiIdx := make([]int, len(outShape))
		remaining := i
		for d := 0; d < len(outShape); d++ {
			multiIdx[d] = remaining / outStrides[d]
			remaining %= outStrides[d]
		}

		// Compute indices with broadcasting
		condIdx := broadcastIndex(multiIdx, condShape, condStrides)
		xIdx := broadcastIndex(multiIdx, xShape, xStrides)
		yIdx := broadcastIndex(multiIdx, yShape, yStrides)

		// Select based on condition
		if condData[condIdx] != 0 {
			dst[i] = xData[xIdx]
		} else {
			dst[i] = yData[yIdx]
		}
	}
}

// whereFloat64 performs where operation for float64 data.
func whereFloat64(dst []float64, condition *tensor.RawTensor, xData, yData []float64,
	outShape, condShape, xShape, yShape tensor.Shape) {
	outStrides := outShape.ComputeStrides()
	condStrides := condShape.ComputeStrides()
	xStrides := xShape.ComputeStrides()
	yStrides := yShape.ComputeStrides()

	condData := getConditionAsUint8(condition)

	for i := range dst {
		multiIdx := make([]int, len(outShape))
		remaining := i
		for d := 0; d < len(outShape); d++ {
			multiIdx[d] = remaining / outStrides[d]
			remaining %= outStrides[d]
		}

		condIdx := broadcastIndex(multiIdx, condShape, condStrides)
		xIdx := broadcastIndex(multiIdx, xShape, xStrides)
		yIdx := broadcastIndex(multiIdx, yShape, yStrides)

		if condData[condIdx] != 0 {
			dst[i] = xData[xIdx]
		} else {
			dst[i] = yData[yIdx]
		}
	}
}

// whereInt32 performs where operation for int32 data.
func whereInt32(dst []int32, condition *tensor.RawTensor, xData, yData []int32,
	outShape, condShape, xShape, yShape tensor.Shape) {
	outStrides := outShape.ComputeStrides()
	condStrides := condShape.ComputeStrides()
	xStrides := xShape.ComputeStrides()
	yStrides := yShape.ComputeStrides()

	condData := getConditionAsUint8(condition)

	for i := range dst {
		multiIdx := make([]int, len(outShape))
		remaining := i
		for d := 0; d < len(outShape); d++ {
			multiIdx[d] = remaining / outStrides[d]
			remaining %= outStrides[d]
		}

		condIdx := broadcastIndex(multiIdx, condShape, condStrides)
		xIdx := broadcastIndex(multiIdx, xShape, xStrides)
		yIdx := broadcastIndex(multiIdx, yShape, yStrides)

		if condData[condIdx] != 0 {
			dst[i] = xData[xIdx]
		} else {
			dst[i] = yData[yIdx]
		}
	}
}

// whereInt64 performs where operation for int64 data.
func whereInt64(dst []int64, condition *tensor.RawTensor, xData, yData []int64,
	outShape, condShape, xShape, yShape tensor.Shape) {
	outStrides := outShape.ComputeStrides()
	condStrides := condShape.ComputeStrides()
	xStrides := xShape.ComputeStrides()
	yStrides := yShape.ComputeStrides()

	condData := getConditionAsUint8(condition)

	for i := range dst {
		multiIdx := make([]int, len(outShape))
		remaining := i
		for d := 0; d < len(outShape); d++ {
			multiIdx[d] = remaining / outStrides[d]
			remaining %= outStrides[d]
		}

		condIdx := broadcastIndex(multiIdx, condShape, condStrides)
		xIdx := broadcastIndex(multiIdx, xShape, xStrides)
		yIdx := broadcastIndex(multiIdx, yShape, yStrides)

		if condData[condIdx] != 0 {
			dst[i] = xData[xIdx]
		} else {
			dst[i] = yData[yIdx]
		}
	}
}

// whereUInt8 performs where operation for uint8 data.
func whereUInt8(dst []uint8, condition *tensor.RawTensor, xData, yData []uint8,
	outShape, condShape, xShape, yShape tensor.Shape) {
	outStrides := outShape.ComputeStrides()
	condStrides := condShape.ComputeStrides()
	xStrides := xShape.ComputeStrides()
	yStrides := yShape.ComputeStrides()

	condData := getConditionAsUint8(condition)

	for i := range dst {
		multiIdx := make([]int, len(outShape))
		remaining := i
		for d := 0; d < len(outShape); d++ {
			multiIdx[d] = remaining / outStrides[d]
			remaining %= outStrides[d]
		}

		condIdx := broadcastIndex(multiIdx, condShape, condStrides)
		xIdx := broadcastIndex(multiIdx, xShape, xStrides)
		yIdx := broadcastIndex(multiIdx, yShape, yStrides)

		if condData[condIdx] != 0 {
			dst[i] = xData[xIdx]
		} else {
			dst[i] = yData[yIdx]
		}
	}
}

// broadcastIndex computes the flat index for a broadcasted tensor.
func broadcastIndex(multiIdx []int, shape tensor.Shape, strides []int) int {
	idx := 0
	offset := len(multiIdx) - len(shape)
	for i, size := range shape {
		dimIdx := multiIdx[offset+i]
		// Broadcast dimension (size 1) always uses index 0
		if size == 1 {
			dimIdx = 0
		}
		idx += dimIdx * strides[i]
	}
	return idx
}

// getConditionAsUint8 converts condition tensor to uint8 data (bool -> uint8).
func getConditionAsUint8(condition *tensor.RawTensor) []uint8 {
	if condition.DType() == tensor.Bool {
		boolData := condition.AsBool()
		uint8Data := make([]uint8, len(boolData))
		for i, b := range boolData {
			if b {
				uint8Data[i] = 1
			}
		}
		return uint8Data
	}
	return condition.AsUint8()
}

// Embedding performs embedding lookup.
// weight: [numEmbeddings, embeddingDim]
// indices: any shape of int32 indices
// output: [...indices.shape, embeddingDim]
//
// Similar to PyTorch's F.embedding or nn.Embedding.
func (cpu *CPUBackend) Embedding(weight, indices *tensor.RawTensor) *tensor.RawTensor {
	// Validate indices dtype
	if indices.DType() != tensor.Int32 {
		panic(fmt.Sprintf("embedding: indices must be int32, got %s", indices.DType()))
	}

	// Validate weight shape (must be 2D)
	weightShape := weight.Shape()
	if len(weightShape) != 2 {
		panic(fmt.Sprintf("embedding: weight must be 2D, got shape %v", weightShape))
	}

	numEmbeddings := weightShape[0]
	embeddingDim := weightShape[1]

	// Output shape: [...indices.shape, embeddingDim]
	indicesShape := indices.Shape()
	outputShape := make(tensor.Shape, len(indicesShape)+1)
	copy(outputShape, indicesShape)
	outputShape[len(outputShape)-1] = embeddingDim

	// Create output tensor
	result, err := tensor.NewRaw(outputShape, weight.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("embedding: failed to create result tensor: %v", err))
	}

	// Perform embedding lookup
	indicesData := indices.AsInt32()
	numIndices := indices.NumElements()

	switch weight.DType() {
	case tensor.Float32:
		embeddingFloat32(result.AsFloat32(), weight.AsFloat32(), indicesData, numIndices, numEmbeddings, embeddingDim)
	case tensor.Float64:
		embeddingFloat64(result.AsFloat64(), weight.AsFloat64(), indicesData, numIndices, numEmbeddings, embeddingDim)
	default:
		panic(fmt.Sprintf("embedding: unsupported weight dtype %s", weight.DType()))
	}

	return result
}

func embeddingFloat32(dst, weight []float32, indices []int32, numIndices, numEmbeddings, embeddingDim int) {
	for i := 0; i < numIndices; i++ {
		idx := int(indices[i])
		if idx < 0 || idx >= numEmbeddings {
			panic(fmt.Sprintf("embedding: index %d out of bounds [0, %d)", idx, numEmbeddings))
		}

		srcOffset := idx * embeddingDim
		dstOffset := i * embeddingDim
		copy(dst[dstOffset:dstOffset+embeddingDim], weight[srcOffset:srcOffset+embeddingDim])
	}
}

func embeddingFloat64(dst, weight []float64, indices []int32, numIndices, numEmbeddings, embeddingDim int) {
	for i := 0; i < numIndices; i++ {
		idx := int(indices[i])
		if idx < 0 || idx >= numEmbeddings {
			panic(fmt.Sprintf("embedding: index %d out of bounds [0, %d)", idx, numEmbeddings))
		}

		srcOffset := idx * embeddingDim
		dstOffset := i * embeddingDim
		copy(dst[dstOffset:dstOffset+embeddingDim], weight[srcOffset:srcOffset+embeddingDim])
	}
}
