// Package tensor provides the core tensor types and operations for Born ML framework.
package tensor

import (
	"fmt"
	"math"
)

// Verify that MockBackend implements Backend.
var _ Backend = (*MockBackend)(nil)

// MockBackend is a simple backend for testing.
// It implements all operations naively for correctness verification.
type MockBackend struct{}

// NewMockBackend creates a new MockBackend.
func NewMockBackend() *MockBackend {
	return &MockBackend{}
}

// Name returns the backend name.
func (m *MockBackend) Name() string {
	return "mock"
}

// Device returns the device type.
func (m *MockBackend) Device() Device {
	return CPU
}

// Add performs element-wise addition with broadcasting.
func (m *MockBackend) Add(a, b *RawTensor) *RawTensor {
	return m.elementWise(a, b, func(x, y float64) float64 { return x + y })
}

// Sub performs element-wise subtraction with broadcasting.
func (m *MockBackend) Sub(a, b *RawTensor) *RawTensor {
	return m.elementWise(a, b, func(x, y float64) float64 { return x - y })
}

// Mul performs element-wise multiplication with broadcasting.
func (m *MockBackend) Mul(a, b *RawTensor) *RawTensor {
	return m.elementWise(a, b, func(x, y float64) float64 { return x * y })
}

// Div performs element-wise division with broadcasting.
func (m *MockBackend) Div(a, b *RawTensor) *RawTensor {
	return m.elementWise(a, b, func(x, y float64) float64 { return x / y })
}

// elementWise performs element-wise operations with broadcasting.
func (m *MockBackend) elementWise(a, b *RawTensor, op func(float64, float64) float64) *RawTensor {
	// Broadcast shapes
	outShape, _, err := BroadcastShapes(a.Shape(), b.Shape())
	if err != nil {
		panic(err)
	}

	// Create output tensor
	result, err := NewRaw(outShape, a.DType(), m.Device())
	if err != nil {
		panic(err)
	}

	// Perform operation (naive implementation)
	numElements := outShape.NumElements()

	// Convert to float64 for generic processing
	aData := m.toFloat64Slice(a)
	bData := m.toFloat64Slice(b)
	resultData := m.toFloat64Slice(result)

	for i := 0; i < numElements; i++ {
		aIdx := m.broadcastIndex(i, outShape, a.Shape())
		bIdx := m.broadcastIndex(i, outShape, b.Shape())

		resultData[i] = op(aData[aIdx], bData[bIdx])
	}

	m.fromFloat64Slice(resultData, result)
	return result
}

// MatMul performs matrix multiplication.
func (m *MockBackend) MatMul(a, b *RawTensor) *RawTensor {
	aShape := a.Shape()
	bShape := b.Shape()

	// Only support 2D for now
	if len(aShape) != 2 || len(bShape) != 2 {
		panic("MatMul only supports 2D tensors in mock backend")
	}

	if aShape[1] != bShape[0] {
		panic(fmt.Sprintf("incompatible shapes for MatMul: %v @ %v", aShape, bShape))
	}

	M, K := aShape[0], aShape[1]
	N := bShape[1]

	outShape := Shape{M, N}
	result, err := NewRaw(outShape, a.DType(), m.Device())
	if err != nil {
		panic(err)
	}

	aData := m.toFloat64Slice(a)
	bData := m.toFloat64Slice(b)
	resultData := m.toFloat64Slice(result)

	// Naive matrix multiplication
	for i := 0; i < M; i++ {
		for j := 0; j < N; j++ {
			sum := 0.0
			for k := 0; k < K; k++ {
				sum += aData[i*K+k] * bData[k*N+j]
			}
			resultData[i*N+j] = sum
		}
	}

	m.fromFloat64Slice(resultData, result)
	return result
}

// BatchMatMul performs batched matrix multiplication (naive implementation for testing).
func (m *MockBackend) BatchMatMul(a, b *RawTensor) *RawTensor {
	aShape := a.Shape()
	bShape := b.Shape()
	ndim := len(aShape)

	// Validate dimensions
	if ndim < 3 {
		panic(fmt.Sprintf("BatchMatMul: inputs must be at least 3D, got %dD", ndim))
	}
	if len(bShape) != ndim {
		panic(fmt.Sprintf("BatchMatMul: dimension mismatch, got %dD and %dD", ndim, len(bShape)))
	}

	// Validate batch dimensions match
	for i := 0; i < ndim-2; i++ {
		if aShape[i] != bShape[i] {
			panic(fmt.Sprintf("BatchMatMul: batch dimension mismatch at dim %d: %d vs %d", i, aShape[i], bShape[i]))
		}
	}

	// Extract matrix dimensions
	rows := aShape[ndim-2]
	k1 := aShape[ndim-1]
	k2 := bShape[ndim-2]
	cols := bShape[ndim-1]

	if k1 != k2 {
		panic(fmt.Sprintf("BatchMatMul: inner dimension mismatch: %d vs %d", k1, k2))
	}

	// Compute batch size (product of all batch dims)
	batchSize := 1
	for i := 0; i < ndim-2; i++ {
		batchSize *= aShape[i]
	}

	// Output shape = batch dims + [M, N]
	outShape := make(Shape, ndim)
	copy(outShape, aShape[:ndim-2])
	outShape[ndim-2] = rows
	outShape[ndim-1] = cols

	result, err := NewRaw(outShape, a.DType(), m.Device())
	if err != nil {
		panic(err)
	}

	aData := m.toFloat64Slice(a)
	bData := m.toFloat64Slice(b)
	resultData := m.toFloat64Slice(result)

	matrixSizeA := rows * k1
	matrixSizeB := k1 * cols
	matrixSizeC := rows * cols

	// Batched matrix multiplication
	for batch := 0; batch < batchSize; batch++ {
		aOffset := batch * matrixSizeA
		bOffset := batch * matrixSizeB
		cOffset := batch * matrixSizeC

		for i := 0; i < rows; i++ {
			for j := 0; j < cols; j++ {
				sum := 0.0
				for kIdx := 0; kIdx < k1; kIdx++ {
					sum += aData[aOffset+i*k1+kIdx] * bData[bOffset+kIdx*cols+j]
				}
				resultData[cOffset+i*cols+j] = sum
			}
		}
	}

	m.fromFloat64Slice(resultData, result)
	return result
}

// Conv2D performs 2D convolution (naive implementation for testing).
func (m *MockBackend) Conv2D(input, kernel *RawTensor, stride, padding int) *RawTensor {
	inputShape := input.Shape()
	kernelShape := kernel.Shape()

	if len(inputShape) != 4 || len(kernelShape) != 4 {
		panic("Conv2D requires 4D tensors [N,C,H,W]")
	}

	N := inputShape[0]
	CIn := inputShape[1]
	H := inputShape[2]
	W := inputShape[3]
	COut := kernelShape[0]
	KH := kernelShape[2]
	KW := kernelShape[3]

	if CIn != kernelShape[1] {
		panic(fmt.Sprintf("Conv2D: input channels %d != kernel channels %d", CIn, kernelShape[1]))
	}

	HOut := (H+2*padding-KH)/stride + 1
	WOut := (W+2*padding-KW)/stride + 1

	output, err := NewRaw(Shape{N, COut, HOut, WOut}, input.DType(), m.Device())
	if err != nil {
		panic(err)
	}

	inputData := m.toFloat64Slice(input)
	kernelData := m.toFloat64Slice(kernel)
	outputData := m.toFloat64Slice(output)

	// Naive convolution (direct implementation)
	for n := 0; n < N; n++ {
		for cOut := 0; cOut < COut; cOut++ {
			for outH := 0; outH < HOut; outH++ {
				for outW := 0; outW < WOut; outW++ {
					sum := 0.0

					// Convolve over input patch
					for cIn := 0; cIn < CIn; cIn++ {
						for kh := 0; kh < KH; kh++ {
							for kw := 0; kw < KW; kw++ {
								h := outH*stride - padding + kh
								w := outW*stride - padding + kw

								// Check bounds (zero padding)
								if h >= 0 && h < H && w >= 0 && w < W {
									inputIdx := n*CIn*H*W + cIn*H*W + h*W + w
									kernelIdx := cOut*CIn*KH*KW + cIn*KH*KW + kh*KW + kw
									sum += inputData[inputIdx] * kernelData[kernelIdx]
								}
							}
						}
					}

					outputIdx := n*COut*HOut*WOut + cOut*HOut*WOut + outH*WOut + outW
					outputData[outputIdx] = sum
				}
			}
		}
	}

	m.fromFloat64Slice(outputData, output)
	return output
}

// MaxPool2D performs 2D max pooling (naive implementation for testing).
func (m *MockBackend) MaxPool2D(input *RawTensor, kernelSize, stride int) *RawTensor {
	inputShape := input.Shape()
	if len(inputShape) != 4 {
		panic(fmt.Sprintf("MaxPool2D: expected 4D input [N,C,H,W], got %dD", len(inputShape)))
	}

	N := inputShape[0]
	C := inputShape[1]
	H := inputShape[2]
	W := inputShape[3]

	// Compute output dimensions
	HOut := (H-kernelSize)/stride + 1
	WOut := (W-kernelSize)/stride + 1

	output, err := NewRaw(Shape{N, C, HOut, WOut}, input.DType(), m.Device())
	if err != nil {
		panic(err)
	}

	inputData := m.toFloat64Slice(input)
	outputData := m.toFloat64Slice(output)

	// Naive max pooling
	for n := 0; n < N; n++ {
		for c := 0; c < C; c++ {
			for outH := 0; outH < HOut; outH++ {
				for outW := 0; outW < WOut; outW++ {
					hStart := outH * stride
					wStart := outW * stride

					// Find max in pooling window
					maxVal := -1e308 // Negative infinity
					for kh := 0; kh < kernelSize; kh++ {
						for kw := 0; kw < kernelSize; kw++ {
							h := hStart + kh
							w := wStart + kw
							inputIdx := n*C*H*W + c*H*W + h*W + w
							if inputData[inputIdx] > maxVal {
								maxVal = inputData[inputIdx]
							}
						}
					}

					outputIdx := n*C*HOut*WOut + c*HOut*WOut + outH*WOut + outW
					outputData[outputIdx] = maxVal
				}
			}
		}
	}

	m.fromFloat64Slice(outputData, output)
	return output
}

// Reshape changes tensor shape.
func (m *MockBackend) Reshape(t *RawTensor, newShape Shape) *RawTensor {
	if err := newShape.Validate(); err != nil {
		panic(err)
	}

	if t.NumElements() != newShape.NumElements() {
		panic(fmt.Sprintf("cannot reshape tensor with %d elements to shape %v (%d elements)",
			t.NumElements(), newShape, newShape.NumElements()))
	}

	result, err := NewRaw(newShape, t.DType(), m.Device())
	if err != nil {
		panic(err)
	}

	// Copy data
	copy(result.Data(), t.Data())
	return result
}

// Transpose transposes tensor dimensions.
func (m *MockBackend) Transpose(t *RawTensor, axes ...int) *RawTensor {
	shape := t.Shape()

	// Default: reverse all dimensions
	if len(axes) == 0 {
		axes = make([]int, len(shape))
		for i := range axes {
			axes[i] = len(shape) - 1 - i
		}
	}

	// Validate axes
	if len(axes) != len(shape) {
		panic(fmt.Sprintf("axes length %d doesn't match tensor dimensions %d", len(axes), len(shape)))
	}

	// Compute new shape
	newShape := make(Shape, len(shape))
	for i, axis := range axes {
		if axis < 0 || axis >= len(shape) {
			panic(fmt.Sprintf("axis %d out of bounds for tensor with %d dimensions", axis, len(shape)))
		}
		newShape[i] = shape[axis]
	}

	result, err := NewRaw(newShape, t.DType(), m.Device())
	if err != nil {
		panic(err)
	}

	// Transpose data (naive implementation)
	tData := m.toFloat64Slice(t)
	resultData := m.toFloat64Slice(result)

	oldStrides := shape.ComputeStrides()
	newStrides := newShape.ComputeStrides()

	for i := 0; i < t.NumElements(); i++ {
		// Convert flat index to multi-dimensional indices
		indices := make([]int, len(shape))
		temp := i
		for j := 0; j < len(shape); j++ {
			indices[j] = temp / oldStrides[j]
			temp %= oldStrides[j]
		}

		// Permute indices
		permuted := make([]int, len(indices))
		for j, axis := range axes {
			permuted[j] = indices[axis]
		}

		// Convert permuted indices to flat index
		newIdx := 0
		for j, idx := range permuted {
			newIdx += idx * newStrides[j]
		}

		resultData[newIdx] = tData[i]
	}

	m.fromFloat64Slice(resultData, result)
	return result
}

// Helper functions

func (m *MockBackend) toFloat64Slice(t *RawTensor) []float64 {
	switch t.DType() {
	case Float32:
		src := t.AsFloat32()
		dst := make([]float64, len(src))
		for i, v := range src {
			dst[i] = float64(v)
		}
		return dst
	case Float64:
		return t.AsFloat64()
	case Int32:
		src := t.AsInt32()
		dst := make([]float64, len(src))
		for i, v := range src {
			dst[i] = float64(v)
		}
		return dst
	case Int64:
		src := t.AsInt64()
		dst := make([]float64, len(src))
		for i, v := range src {
			dst[i] = float64(v)
		}
		return dst
	default:
		panic(fmt.Sprintf("unsupported dtype: %s", t.DType()))
	}
}

func (m *MockBackend) fromFloat64Slice(src []float64, t *RawTensor) {
	switch t.DType() {
	case Float32:
		dst := t.AsFloat32()
		for i, v := range src {
			dst[i] = float32(v)
		}
	case Float64:
		copy(t.AsFloat64(), src)
	case Int32:
		dst := t.AsInt32()
		for i, v := range src {
			dst[i] = int32(v)
		}
	case Int64:
		dst := t.AsInt64()
		for i, v := range src {
			dst[i] = int64(v)
		}
	}
}

func (m *MockBackend) broadcastIndex(flatIdx int, outShape, inShape Shape) int {
	// Convert flat index to multi-dimensional indices in output shape
	outStrides := outShape.ComputeStrides()
	indices := make([]int, len(outShape))

	temp := flatIdx
	for i := 0; i < len(outShape); i++ {
		indices[i] = temp / outStrides[i]
		temp %= outStrides[i]
	}

	// Map to input shape (accounting for broadcasting)
	inStrides := inShape.ComputeStrides()
	inIdx := 0

	offset := len(outShape) - len(inShape)
	for i := 0; i < len(inShape); i++ {
		outDimIdx := indices[offset+i]
		inDim := inShape[i]

		// If input dimension is 1, always use index 0 (broadcasting)
		if inDim == 1 {
			outDimIdx = 0
		}

		inIdx += outDimIdx * inStrides[i]
	}

	return inIdx
}

// Exp computes element-wise exponential.
func (m *MockBackend) Exp(x *RawTensor) *RawTensor {
	return m.unaryOp(x, math.Exp)
}

// Erf computes element-wise error function.
func (m *MockBackend) Erf(x *RawTensor) *RawTensor {
	return m.unaryOp(x, math.Erf)
}

// Sqrt computes element-wise square root.
func (m *MockBackend) Sqrt(x *RawTensor) *RawTensor {
	return m.unaryOp(x, func(v float64) float64 {
		if v < 0 {
			panic(fmt.Sprintf("sqrt: negative value %f", v))
		}
		return math.Sqrt(v)
	})
}

// Rsqrt computes element-wise reciprocal square root.
func (m *MockBackend) Rsqrt(x *RawTensor) *RawTensor {
	return m.unaryOp(x, func(v float64) float64 {
		if v <= 0 {
			panic(fmt.Sprintf("rsqrt: non-positive value %f", v))
		}
		return 1.0 / math.Sqrt(v)
	})
}

// Cos computes element-wise cosine.
func (m *MockBackend) Cos(x *RawTensor) *RawTensor {
	return m.unaryOp(x, math.Cos)
}

// Sin computes element-wise sine.
func (m *MockBackend) Sin(x *RawTensor) *RawTensor {
	return m.unaryOp(x, math.Sin)
}

// Sign computes element-wise sign function: -1 for negative, 0 for zero, 1 for positive.
func (m *MockBackend) Sign(x *RawTensor) *RawTensor {
	return m.unaryOp(x, func(v float64) float64 {
		switch {
		case math.IsNaN(v):
			return math.NaN()
		case v > 0:
			return 1.0
		case v < 0:
			return -1.0
		default:
			return 0.0
		}
	})
}

// Abs computes element-wise absolute value.
func (m *MockBackend) Abs(x *RawTensor) *RawTensor {
	return m.unaryOp(x, math.Abs)
}

// unaryOp applies a unary operation element-wise.
func (m *MockBackend) unaryOp(x *RawTensor, op func(float64) float64) *RawTensor {
	result, err := NewRaw(x.Shape(), x.DType(), m.Device())
	if err != nil {
		panic(err)
	}

	xData := m.toFloat64Slice(x)
	resultData := m.toFloat64Slice(result)

	for i := range xData {
		resultData[i] = op(xData[i])
	}

	m.fromFloat64Slice(resultData, result)
	return result
}

// SumDim sums tensor elements along the specified dimension (naive implementation).
func (m *MockBackend) SumDim(x *RawTensor, dim int, keepDim bool) *RawTensor {
	shape := x.Shape()
	ndim := len(shape)

	// Normalize negative dimension
	if dim < 0 {
		dim = ndim + dim
	}

	// Validate dimension
	if dim < 0 || dim >= ndim {
		panic(fmt.Sprintf("sumdim: dimension %d out of range for %dD tensor", dim, ndim))
	}

	// Calculate output shape
	var outShape Shape
	if keepDim {
		outShape = shape.Clone()
		outShape[dim] = 1
	} else {
		outShape = make(Shape, 0, ndim-1)
		for i := 0; i < ndim; i++ {
			if i != dim {
				outShape = append(outShape, shape[i])
			}
		}
	}

	result, err := NewRaw(outShape, x.DType(), m.Device())
	if err != nil {
		panic(err)
	}

	xData := m.toFloat64Slice(x)
	resultData := m.toFloat64Slice(result)

	// Initialize result to zero
	for i := range resultData {
		resultData[i] = 0
	}

	// Calculate strides
	strides := shape.ComputeStrides()
	outShapeWithDim := shape.Clone()
	outShapeWithDim[dim] = 1
	outStrides := outShapeWithDim.ComputeStrides()

	// Sum along dimension
	for i := 0; i < len(xData); i++ {
		// Compute output index
		outIdx := 0
		temp := i
		for d := 0; d < ndim; d++ {
			coord := temp / strides[d]
			temp %= strides[d]

			if d != dim {
				outIdx += coord * outStrides[d]
			}
		}

		resultData[outIdx] += xData[i]
	}

	m.fromFloat64Slice(resultData, result)
	return result
}

// MeanDim computes the mean of tensor elements along the specified dimension.
func (m *MockBackend) MeanDim(x *RawTensor, dim int, keepDim bool) *RawTensor {
	// Sum along dimension
	sumResult := m.SumDim(x, dim, keepDim)

	// Normalize negative dimension
	shape := x.Shape()
	ndim := len(shape)
	if dim < 0 {
		dim = ndim + dim
	}

	// Divide by the size of the reduced dimension
	divisor := float64(shape[dim])
	sumData := m.toFloat64Slice(sumResult)
	for i := range sumData {
		sumData[i] /= divisor
	}

	m.fromFloat64Slice(sumData, sumResult)
	return sumResult
}

// Cat concatenates tensors along the specified dimension (naive implementation).
func (m *MockBackend) Cat(tensors []*RawTensor, dim int) *RawTensor {
	if len(tensors) == 0 {
		panic("cat: at least one tensor required")
	}

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

	// Validate shapes and calculate total size
	totalDim := 0
	for i, t := range tensors {
		tShape := t.Shape()
		if len(tShape) != ndim {
			panic(fmt.Sprintf("cat: tensor %d has %d dimensions, expected %d", i, len(tShape), ndim))
		}
		if t.DType() != dtype {
			panic(fmt.Sprintf("cat: tensor %d has dtype %s, expected %s", i, t.DType(), dtype))
		}

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

	result, err := NewRaw(outShape, dtype, m.Device())
	if err != nil {
		panic(err)
	}

	// Concatenate data
	resultData := m.toFloat64Slice(result)
	outStrides := outShape.ComputeStrides()

	offset := 0
	for _, t := range tensors {
		tData := m.toFloat64Slice(t)
		tShape := t.Shape()
		tStrides := tShape.ComputeStrides()

		for i := 0; i < len(tData); i++ {
			// Compute multi-dimensional index
			outIdx := 0
			temp := i
			for d := 0; d < ndim; d++ {
				coord := temp / tStrides[d]
				temp %= tStrides[d]

				if d == dim {
					coord += offset
				}
				outIdx += coord * outStrides[d]
			}

			resultData[outIdx] = tData[i]
		}

		offset += tShape[dim]
	}

	m.fromFloat64Slice(resultData, result)
	return result
}

// Chunk splits tensor into n equal parts along the specified dimension.
func (m *MockBackend) Chunk(x *RawTensor, n, dim int) []*RawTensor {
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

	dimSize := shape[dim]
	if dimSize%n != 0 {
		panic(fmt.Sprintf("chunk: dimension %d size %d not divisible by %d", dim, dimSize, n))
	}

	chunkSize := dimSize / n
	chunkShape := shape.Clone()
	chunkShape[dim] = chunkSize

	// Create result tensors
	results := make([]*RawTensor, n)
	for i := 0; i < n; i++ {
		chunk, err := NewRaw(chunkShape, x.DType(), m.Device())
		if err != nil {
			panic(err)
		}
		results[i] = chunk
	}

	// Split data
	xData := m.toFloat64Slice(x)
	strides := shape.ComputeStrides()

	for i := 0; i < len(xData); i++ {
		// Compute multi-dimensional index
		temp := i
		coords := make([]int, ndim)
		for d := 0; d < ndim; d++ {
			coords[d] = temp / strides[d]
			temp %= strides[d]
		}

		// Determine chunk
		chunkIdx := coords[dim] / chunkSize
		localCoord := coords[dim] % chunkSize

		// Compute output index
		outStrides := chunkShape.ComputeStrides()
		outIdx := 0
		for d := 0; d < ndim; d++ {
			if d == dim {
				outIdx += localCoord * outStrides[d]
			} else {
				outIdx += coords[d] * outStrides[d]
			}
		}

		chunkData := m.toFloat64Slice(results[chunkIdx])
		chunkData[outIdx] = xData[i]
		m.fromFloat64Slice(chunkData, results[chunkIdx])
	}

	return results
}

// Unsqueeze adds a dimension of size 1 at the specified position.
func (m *MockBackend) Unsqueeze(x *RawTensor, dim int) *RawTensor {
	shape := x.Shape()
	ndim := len(shape)

	// Normalize negative dimension
	if dim < 0 {
		dim = ndim + 1 + dim
	}

	// Validate dimension
	if dim < 0 || dim > ndim {
		panic(fmt.Sprintf("unsqueeze: dimension %d out of range for %dD tensor (valid: [0, %d])", dim, ndim, ndim))
	}

	// Create new shape
	newShape := make(Shape, ndim+1)
	for i := 0; i < dim; i++ {
		newShape[i] = shape[i]
	}
	newShape[dim] = 1
	for i := dim; i < ndim; i++ {
		newShape[i+1] = shape[i]
	}

	return m.Reshape(x, newShape)
}

// Squeeze removes a dimension of size 1 at the specified position.
func (m *MockBackend) Squeeze(x *RawTensor, dim int) *RawTensor {
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

	if shape[dim] != 1 {
		panic(fmt.Sprintf("squeeze: dimension %d has size %d, must be 1", dim, shape[dim]))
	}

	// Create new shape
	newShape := make(Shape, 0, ndim-1)
	for i := 0; i < ndim; i++ {
		if i != dim {
			newShape = append(newShape, shape[i])
		}
	}

	return m.Reshape(x, newShape)
}

// Gather selects elements along dim using index tensor (naive implementation).
func (m *MockBackend) Gather(x *RawTensor, dim int, index *RawTensor) *RawTensor {
	if index.DType() != Int32 {
		panic(fmt.Sprintf("gather: index must be int32, got %s", index.DType()))
	}

	shape := x.Shape()
	ndim := len(shape)

	// Normalize negative dimension
	if dim < 0 {
		dim = ndim + dim
	}

	// Validate dimension
	if dim < 0 || dim >= ndim {
		panic(fmt.Sprintf("gather: dimension %d out of range for %dD tensor", dim, ndim))
	}

	indexShape := index.Shape()
	if len(indexShape) != ndim {
		panic(fmt.Sprintf("gather: index rank %d != input rank %d", len(indexShape), ndim))
	}

	// Validate index shape
	for i := 0; i < ndim; i++ {
		if i != dim && indexShape[i] != shape[i] {
			panic(fmt.Sprintf("gather: index shape mismatch at dim %d: %d != %d", i, indexShape[i], shape[i]))
		}
	}

	// Create result
	result, err := NewRaw(indexShape, x.DType(), m.Device())
	if err != nil {
		panic(err)
	}

	xData := m.toFloat64Slice(x)
	resultData := m.toFloat64Slice(result)
	indices := index.AsInt32()

	strides := shape.ComputeStrides()
	indexStrides := indexShape.ComputeStrides()

	for i := 0; i < len(resultData); i++ {
		// Compute multi-dimensional index
		coords := make([]int, ndim)
		temp := i
		for d := 0; d < ndim; d++ {
			coords[d] = temp / indexStrides[d]
			temp %= indexStrides[d]
		}

		// Get index value
		idx := int(indices[i])
		if idx < 0 || idx >= shape[dim] {
			panic(fmt.Sprintf("gather: index %d out of bounds [0, %d)", idx, shape[dim]))
		}

		// Compute source index
		srcIdx := 0
		for d := 0; d < ndim; d++ {
			if d == dim {
				srcIdx += idx * strides[d]
			} else {
				srcIdx += coords[d] * strides[d]
			}
		}

		resultData[i] = xData[srcIdx]
	}

	m.fromFloat64Slice(resultData, result)
	return result
}

// Where performs conditional element selection (naive implementation).
func (m *MockBackend) Where(condition, x, y *RawTensor) *RawTensor {
	if condition.DType() != Bool && condition.DType() != Uint8 {
		panic(fmt.Sprintf("where: condition must be bool or uint8, got %s", condition.DType()))
	}

	if x.DType() != y.DType() {
		panic(fmt.Sprintf("where: x and y must have same dtype, got %s and %s", x.DType(), y.DType()))
	}

	// Broadcast all three shapes
	outShape1, _, err := BroadcastShapes(condition.Shape(), x.Shape())
	if err != nil {
		panic(fmt.Sprintf("where: failed to broadcast condition and x: %v", err))
	}
	outShape, _, err := BroadcastShapes(outShape1, y.Shape())
	if err != nil {
		panic(fmt.Sprintf("where: failed to broadcast with y: %v", err))
	}

	result, err := NewRaw(outShape, x.DType(), m.Device())
	if err != nil {
		panic(err)
	}

	// Get condition data as uint8
	var condData []uint8
	if condition.DType() == Bool {
		boolData := condition.AsBool()
		condData = make([]uint8, len(boolData))
		for i, b := range boolData {
			if b {
				condData[i] = 1
			}
		}
	} else {
		condData = condition.AsUint8()
	}

	xData := m.toFloat64Slice(x)
	yData := m.toFloat64Slice(y)
	resultData := m.toFloat64Slice(result)

	for i := 0; i < len(resultData); i++ {
		condIdx := m.broadcastIndex(i, outShape, condition.Shape())
		xIdx := m.broadcastIndex(i, outShape, x.Shape())
		yIdx := m.broadcastIndex(i, outShape, y.Shape())

		if condData[condIdx] != 0 {
			resultData[i] = xData[xIdx]
		} else {
			resultData[i] = yData[yIdx]
		}
	}

	m.fromFloat64Slice(resultData, result)
	return result
}

// Clamp restricts tensor values element-wise to [minBound, maxBound].
func (m *MockBackend) Clamp(x *RawTensor, minBound, maxBound any) *RawTensor {
	result, err := NewRaw(x.Shape(), x.DType(), m.Device())
	if err != nil {
		panic(err)
	}
	resultData := m.toFloat64Slice(result)

	minFloat := m.anyToFloat64(minBound)
	maxFloat := m.anyToFloat64(maxBound)

	if minFloat > maxFloat {
		for i := range resultData {
			resultData[i] = maxFloat
		}
		m.fromFloat64Slice(resultData, result)
		return result
	}

	xData := m.toFloat64Slice(x)
	for i, v := range xData {
		resultData[i] = min(max(v, minFloat), maxFloat)
	}

	m.fromFloat64Slice(resultData, result)
	return result
}

// Embedding performs embedding lookup (naive implementation).
// weight: [numEmbeddings, embeddingDim]
// indices: any shape of int32 indices
// output: [...indices.shape, embeddingDim]
func (m *MockBackend) Embedding(weight, indices *RawTensor) *RawTensor {
	if indices.DType() != Int32 {
		panic(fmt.Sprintf("embedding: indices must be int32, got %s", indices.DType()))
	}

	weightShape := weight.Shape()
	if len(weightShape) != 2 {
		panic(fmt.Sprintf("embedding: weight must be 2D, got shape %v", weightShape))
	}

	numEmbeddings := weightShape[0]
	embeddingDim := weightShape[1]

	// Output shape: [...indices.shape, embeddingDim]
	indicesShape := indices.Shape()
	outputShape := make(Shape, len(indicesShape)+1)
	copy(outputShape, indicesShape)
	outputShape[len(outputShape)-1] = embeddingDim

	result, err := NewRaw(outputShape, weight.DType(), m.Device())
	if err != nil {
		panic(err)
	}

	// Lookup embeddings
	indicesData := indices.AsInt32()
	weightData := m.toFloat64Slice(weight)
	resultData := m.toFloat64Slice(result)

	numIndices := indices.NumElements()
	for i := 0; i < numIndices; i++ {
		idx := int(indicesData[i])
		if idx < 0 || idx >= numEmbeddings {
			panic(fmt.Sprintf("embedding: index %d out of bounds [0, %d)", idx, numEmbeddings))
		}

		srcOffset := idx * embeddingDim
		dstOffset := i * embeddingDim
		copy(resultData[dstOffset:dstOffset+embeddingDim], weightData[srcOffset:srcOffset+embeddingDim])
	}

	m.fromFloat64Slice(resultData, result)
	return result
}

// Scalar operations (naive implementations).

// MulScalar multiplies tensor elements by a scalar (mock implementation).
func (m *MockBackend) MulScalar(x *RawTensor, scalar any) *RawTensor {
	result, err := NewRaw(x.Shape(), x.DType(), m.Device())
	if err != nil {
		panic(err)
	}

	xData := m.toFloat64Slice(x)
	resultData := m.toFloat64Slice(result)
	scalarVal := m.anyToFloat64(scalar)

	for i := range resultData {
		resultData[i] = xData[i] * scalarVal
	}

	m.fromFloat64Slice(resultData, result)
	return result
}

// AddScalar adds a scalar to tensor elements (mock implementation).
func (m *MockBackend) AddScalar(x *RawTensor, scalar any) *RawTensor {
	result, err := NewRaw(x.Shape(), x.DType(), m.Device())
	if err != nil {
		panic(err)
	}

	xData := m.toFloat64Slice(x)
	resultData := m.toFloat64Slice(result)
	scalarVal := m.anyToFloat64(scalar)

	for i := range resultData {
		resultData[i] = xData[i] + scalarVal
	}

	m.fromFloat64Slice(resultData, result)
	return result
}

// SubScalar subtracts a scalar from tensor elements (mock implementation).
func (m *MockBackend) SubScalar(x *RawTensor, scalar any) *RawTensor {
	result, err := NewRaw(x.Shape(), x.DType(), m.Device())
	if err != nil {
		panic(err)
	}

	xData := m.toFloat64Slice(x)
	resultData := m.toFloat64Slice(result)
	scalarVal := m.anyToFloat64(scalar)

	for i := range resultData {
		resultData[i] = xData[i] - scalarVal
	}

	m.fromFloat64Slice(resultData, result)
	return result
}

// DivScalar divides tensor elements by a scalar (mock implementation).
func (m *MockBackend) DivScalar(x *RawTensor, scalar any) *RawTensor {
	result, err := NewRaw(x.Shape(), x.DType(), m.Device())
	if err != nil {
		panic(err)
	}

	xData := m.toFloat64Slice(x)
	resultData := m.toFloat64Slice(result)
	scalarVal := m.anyToFloat64(scalar)

	for i := range resultData {
		resultData[i] = xData[i] / scalarVal
	}

	m.fromFloat64Slice(resultData, result)
	return result
}

// Math operations.

// Log computes natural logarithm element-wise (mock implementation).
func (m *MockBackend) Log(x *RawTensor) *RawTensor {
	return m.unaryOp(x, math.Log)
}

// Activation functions.

// ReLU applies ReLU activation: max(0, x).
func (m *MockBackend) ReLU(x *RawTensor) *RawTensor {
	return m.unaryOp(x, func(v float64) float64 {
		if v > 0 {
			return v
		}
		return 0
	})
}

// Sigmoid applies sigmoid activation: 1 / (1 + exp(-x)).
func (m *MockBackend) Sigmoid(x *RawTensor) *RawTensor {
	return m.unaryOp(x, func(v float64) float64 {
		return 1.0 / (1.0 + math.Exp(-v))
	})
}

// Tanh applies hyperbolic tangent activation.
func (m *MockBackend) Tanh(x *RawTensor) *RawTensor {
	return m.unaryOp(x, math.Tanh)
}

// SiLU applies SiLU (Swish) activation: x * sigmoid(x).
func (m *MockBackend) SiLU(x *RawTensor) *RawTensor {
	return m.unaryOp(x, func(v float64) float64 {
		return v / (1.0 + math.Exp(-v))
	})
}

// Softmax applies softmax activation along the specified dimension (mock stub).
func (m *MockBackend) Softmax(_ *RawTensor, _ int) *RawTensor {
	panic("mock: Softmax not implemented (use CPU backend)")
}

// Comparison operations (return bool tensor).

// Greater performs element-wise greater-than comparison (mock implementation).
func (m *MockBackend) Greater(a, b *RawTensor) *RawTensor {
	outShape, _, err := BroadcastShapes(a.Shape(), b.Shape())
	if err != nil {
		panic(err)
	}

	result, err := NewRaw(outShape, Bool, m.Device())
	if err != nil {
		panic(err)
	}

	aData := m.toFloat64Slice(a)
	bData := m.toFloat64Slice(b)
	resultBool := result.AsBool()

	for i := 0; i < len(resultBool); i++ {
		aIdx := m.broadcastIndex(i, outShape, a.Shape())
		bIdx := m.broadcastIndex(i, outShape, b.Shape())
		resultBool[i] = aData[aIdx] > bData[bIdx]
	}

	return result
}

// Lower performs element-wise less-than comparison (mock implementation).
func (m *MockBackend) Lower(a, b *RawTensor) *RawTensor {
	outShape, _, err := BroadcastShapes(a.Shape(), b.Shape())
	if err != nil {
		panic(err)
	}

	result, err := NewRaw(outShape, Bool, m.Device())
	if err != nil {
		panic(err)
	}

	aData := m.toFloat64Slice(a)
	bData := m.toFloat64Slice(b)
	resultBool := result.AsBool()

	for i := 0; i < len(resultBool); i++ {
		aIdx := m.broadcastIndex(i, outShape, a.Shape())
		bIdx := m.broadcastIndex(i, outShape, b.Shape())
		resultBool[i] = aData[aIdx] < bData[bIdx]
	}

	return result
}

// GreaterEqual performs element-wise greater-than-or-equal comparison (mock implementation).
func (m *MockBackend) GreaterEqual(a, b *RawTensor) *RawTensor {
	outShape, _, err := BroadcastShapes(a.Shape(), b.Shape())
	if err != nil {
		panic(err)
	}

	result, err := NewRaw(outShape, Bool, m.Device())
	if err != nil {
		panic(err)
	}

	aData := m.toFloat64Slice(a)
	bData := m.toFloat64Slice(b)
	resultBool := result.AsBool()

	for i := 0; i < len(resultBool); i++ {
		aIdx := m.broadcastIndex(i, outShape, a.Shape())
		bIdx := m.broadcastIndex(i, outShape, b.Shape())
		resultBool[i] = aData[aIdx] >= bData[bIdx]
	}

	return result
}

// LowerEqual performs element-wise less-than-or-equal comparison (mock implementation).
func (m *MockBackend) LowerEqual(a, b *RawTensor) *RawTensor {
	outShape, _, err := BroadcastShapes(a.Shape(), b.Shape())
	if err != nil {
		panic(err)
	}

	result, err := NewRaw(outShape, Bool, m.Device())
	if err != nil {
		panic(err)
	}

	aData := m.toFloat64Slice(a)
	bData := m.toFloat64Slice(b)
	resultBool := result.AsBool()

	for i := 0; i < len(resultBool); i++ {
		aIdx := m.broadcastIndex(i, outShape, a.Shape())
		bIdx := m.broadcastIndex(i, outShape, b.Shape())
		resultBool[i] = aData[aIdx] <= bData[bIdx]
	}

	return result
}

// Equal performs element-wise equality comparison (mock implementation).
func (m *MockBackend) Equal(a, b *RawTensor) *RawTensor {
	outShape, _, err := BroadcastShapes(a.Shape(), b.Shape())
	if err != nil {
		panic(err)
	}

	result, err := NewRaw(outShape, Bool, m.Device())
	if err != nil {
		panic(err)
	}

	aData := m.toFloat64Slice(a)
	bData := m.toFloat64Slice(b)
	resultBool := result.AsBool()

	for i := 0; i < len(resultBool); i++ {
		aIdx := m.broadcastIndex(i, outShape, a.Shape())
		bIdx := m.broadcastIndex(i, outShape, b.Shape())
		resultBool[i] = aData[aIdx] == bData[bIdx]
	}

	return result
}

// NotEqual performs element-wise inequality comparison (mock implementation).
func (m *MockBackend) NotEqual(a, b *RawTensor) *RawTensor {
	outShape, _, err := BroadcastShapes(a.Shape(), b.Shape())
	if err != nil {
		panic(err)
	}

	result, err := NewRaw(outShape, Bool, m.Device())
	if err != nil {
		panic(err)
	}

	aData := m.toFloat64Slice(a)
	bData := m.toFloat64Slice(b)
	resultBool := result.AsBool()

	for i := 0; i < len(resultBool); i++ {
		aIdx := m.broadcastIndex(i, outShape, a.Shape())
		bIdx := m.broadcastIndex(i, outShape, b.Shape())
		resultBool[i] = aData[aIdx] != bData[bIdx]
	}

	return result
}

// Boolean operations.

// Or performs element-wise logical OR operation (mock implementation).
func (m *MockBackend) Or(a, b *RawTensor) *RawTensor {
	outShape, _, err := BroadcastShapes(a.Shape(), b.Shape())
	if err != nil {
		panic(err)
	}

	result, err := NewRaw(outShape, Bool, m.Device())
	if err != nil {
		panic(err)
	}

	aData := a.AsBool()
	bData := b.AsBool()
	resultBool := result.AsBool()

	for i := 0; i < len(resultBool); i++ {
		aIdx := m.broadcastIndex(i, outShape, a.Shape())
		bIdx := m.broadcastIndex(i, outShape, b.Shape())
		resultBool[i] = aData[aIdx] || bData[bIdx]
	}

	return result
}

// And performs element-wise logical AND operation (mock implementation).
func (m *MockBackend) And(a, b *RawTensor) *RawTensor {
	outShape, _, err := BroadcastShapes(a.Shape(), b.Shape())
	if err != nil {
		panic(err)
	}

	result, err := NewRaw(outShape, Bool, m.Device())
	if err != nil {
		panic(err)
	}

	aData := a.AsBool()
	bData := b.AsBool()
	resultBool := result.AsBool()

	for i := 0; i < len(resultBool); i++ {
		aIdx := m.broadcastIndex(i, outShape, a.Shape())
		bIdx := m.broadcastIndex(i, outShape, b.Shape())
		resultBool[i] = aData[aIdx] && bData[bIdx]
	}

	return result
}

// Not performs element-wise logical NOT operation (mock implementation).
func (m *MockBackend) Not(x *RawTensor) *RawTensor {
	result, err := NewRaw(x.Shape(), Bool, m.Device())
	if err != nil {
		panic(err)
	}

	xData := x.AsBool()
	resultBool := result.AsBool()

	for i := range resultBool {
		resultBool[i] = !xData[i]
	}

	return result
}

// Reduction operations.

// Sum computes the total sum of all tensor elements (mock implementation).
func (m *MockBackend) Sum(x *RawTensor) *RawTensor {
	result, err := NewRaw(Shape{}, x.DType(), m.Device())
	if err != nil {
		panic(err)
	}

	xData := m.toFloat64Slice(x)
	var sum float64
	for _, v := range xData {
		sum += v
	}

	resultData := m.toFloat64Slice(result)
	resultData[0] = sum
	m.fromFloat64Slice(resultData, result)

	return result
}

// Argmax returns indices of maximum values along the specified dimension (mock stub).
func (m *MockBackend) Argmax(_ *RawTensor, _ int) *RawTensor {
	panic("mock: Argmax not implemented (use CPU backend)")
}

// Shape operations.

// Expand broadcasts the tensor to a new shape (mock stub).
func (m *MockBackend) Expand(_ *RawTensor, _ Shape) *RawTensor {
	panic("mock: Expand not implemented (use CPU backend)")
}

// Type conversion.

// Cast converts the tensor to a different data type (mock implementation).
func (m *MockBackend) Cast(x *RawTensor, dtype DataType) *RawTensor {
	if x.DType() == dtype {
		return x
	}

	result, err := NewRaw(x.Shape(), dtype, m.Device())
	if err != nil {
		panic(err)
	}

	xData := m.toFloat64Slice(x)
	resultData := m.toFloat64Slice(result)
	copy(resultData, xData)
	m.fromFloat64Slice(resultData, result)

	return result
}

// Helper to convert any scalar to float64.
func (m *MockBackend) anyToFloat64(v any) float64 {
	switch val := v.(type) {
	case float32:
		return float64(val)
	case float64:
		return val
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case uint8:
		return float64(val)
	case bool:
		if val {
			return 1.0
		}
		return 0.0
	default:
		panic(fmt.Sprintf("anyToFloat64: unsupported type %T", v))
	}
}

// SelectAdd performs a scatter-add along the specified dimension (naive implementation).
//
// For each i: result[indices[i], ...] += src[i, ...] (at the given dim).
// Returns a new tensor; dest is not modified.
func (m *MockBackend) SelectAdd(dest *RawTensor, dim int, indices, src *RawTensor) *RawTensor {
	if indices.DType() != Int32 {
		panic(fmt.Sprintf("selectadd: indices must be int32, got %s", indices.DType()))
	}

	destShape := dest.Shape()
	srcShape := src.Shape()
	ndim := len(destShape)

	if dim < 0 {
		dim += ndim
	}
	if dim < 0 || dim >= ndim {
		panic(fmt.Sprintf("selectadd: dim %d out of range for %dD tensor", dim, ndim))
	}
	if len(indices.Shape()) != 1 {
		panic(fmt.Sprintf("selectadd: indices must be 1-D, got shape %v", indices.Shape()))
	}

	numIndices := indices.Shape()[0]

	if len(srcShape) != ndim {
		panic(fmt.Sprintf("selectadd: src rank %d != dest rank %d", len(srcShape), ndim))
	}
	if srcShape[dim] != numIndices {
		panic(fmt.Sprintf("selectadd: src dim %d (%d) != len(indices) (%d)", dim, srcShape[dim], numIndices))
	}
	for d := 0; d < ndim; d++ {
		if d == dim {
			continue
		}
		if srcShape[d] != destShape[d] {
			panic(fmt.Sprintf("selectadd: shape mismatch at dim %d: dest=%d src=%d", d, destShape[d], srcShape[d]))
		}
	}

	// Clone dest.
	result, err := NewRaw(destShape, dest.DType(), m.Device())
	if err != nil {
		panic(err)
	}
	copy(result.Data(), dest.Data())

	idxData := indices.AsInt32()
	destData := m.toFloat64Slice(dest)
	srcData := m.toFloat64Slice(src)
	resultData := m.toFloat64Slice(result)
	// Initialize from dest (already copied via result.Data(), but toFloat64Slice
	// returns a fresh slice — so copy from destData).
	copy(resultData, destData)

	dstStrides := destShape.ComputeStrides()
	srcStrides := srcShape.ComputeStrides()
	innerSize := srcShape.NumElements() / srcShape[dim]
	srcDimStride := srcStrides[dim]
	dstDimStride := dstStrides[dim]

	for i := 0; i < numIndices; i++ {
		idx := int(idxData[i])
		if idx < 0 || idx >= destShape[dim] {
			panic(fmt.Sprintf("selectadd: index %d out of bounds [0, %d)", idx, destShape[dim]))
		}

		for j := 0; j < innerSize; j++ {
			// Compute flat-index offset for non-scatter dimensions.
			nonDimFlat := 0
			rem := j
			for d := 0; d < ndim; d++ {
				if d == dim {
					continue
				}
				nonDimStride := 1
				for dd := d + 1; dd < ndim; dd++ {
					if dd != dim {
						nonDimStride *= srcShape[dd]
					}
				}
				coord := rem / nonDimStride
				rem %= nonDimStride
				nonDimFlat += coord * dstStrides[d]
			}

			srcFlat := i*srcDimStride + nonDimFlat
			dstFlat := idx*dstDimStride + nonDimFlat
			resultData[dstFlat] += srcData[srcFlat]
		}
	}

	m.fromFloat64Slice(resultData, result)
	return result
}

// ScatterAdd performs a general scatter-add matching Gather backward semantics.
//
// For each element in src (same shape as indices), accumulates into result along dim
// at the position given by the corresponding index value. Follows Burn's float_scatter_add.
//
// Returns a new tensor with the same shape as dest. dest is not modified.
func (m *MockBackend) ScatterAdd(dest *RawTensor, dim int, indices, src *RawTensor) *RawTensor {
	if indices.DType() != Int32 {
		panic(fmt.Sprintf("scatteradd: indices must be int32, got %s", indices.DType()))
	}

	destShape := dest.Shape()
	srcShape := src.Shape()
	indexShape := indices.Shape()
	ndim := len(destShape)

	if dim < 0 {
		dim += ndim
	}
	if dim < 0 || dim >= ndim {
		panic(fmt.Sprintf("scatteradd: dim %d out of range for %dD tensor", dim, ndim))
	}

	if len(indexShape) != len(srcShape) {
		panic(fmt.Sprintf("scatteradd: indices rank %d != src rank %d", len(indexShape), len(srcShape)))
	}
	for d := range indexShape {
		if indexShape[d] != srcShape[d] {
			panic(fmt.Sprintf("scatteradd: indices shape %v != src shape %v", indexShape, srcShape))
		}
	}

	if len(srcShape) != ndim {
		panic(fmt.Sprintf("scatteradd: src rank %d != dest rank %d", len(srcShape), ndim))
	}
	for d := 0; d < ndim; d++ {
		if d == dim {
			continue
		}
		if srcShape[d] != destShape[d] {
			panic(fmt.Sprintf("scatteradd: shape mismatch at dim %d: dest=%d src=%d", d, destShape[d], srcShape[d]))
		}
	}

	// Clone dest.
	result, err := NewRaw(destShape, dest.DType(), m.Device())
	if err != nil {
		panic(err)
	}
	copy(result.Data(), dest.Data())

	idxData := indices.AsInt32()
	destData := m.toFloat64Slice(dest)
	srcData := m.toFloat64Slice(src)
	resultData := m.toFloat64Slice(result)
	copy(resultData, destData)

	srcStrides := srcShape.ComputeStrides()
	dstStrides := destShape.ComputeStrides()
	indexStrides := indexShape.ComputeStrides()
	numElements := src.NumElements()

	for i := 0; i < numElements; i++ {
		// Decompose flat index into multi-dimensional coordinates.
		rem := i
		coords := make([]int, ndim)
		for d := 0; d < ndim; d++ {
			coords[d] = rem / srcStrides[d]
			rem %= srcStrides[d]
		}

		// Compute index into the indices tensor.
		indexIdx := 0
		for d := 0; d < ndim; d++ {
			indexIdx += coords[d] * indexStrides[d]
		}
		idx := int(idxData[indexIdx])
		if idx < 0 || idx >= destShape[dim] {
			panic(fmt.Sprintf("scatteradd: index %d out of bounds [0, %d)", idx, destShape[dim]))
		}

		// Compute destination flat index.
		dstIdx := 0
		for d := 0; d < ndim; d++ {
			if d == dim {
				dstIdx += idx * dstStrides[d]
			} else {
				dstIdx += coords[d] * dstStrides[d]
			}
		}
		resultData[dstIdx] += srcData[i]
	}

	m.fromFloat64Slice(resultData, result)
	return result
}

// Conv2DInputBackward computes gradient w.r.t. input for Conv2D.
// Stub implementation for MockBackend (test-only).
func (m *MockBackend) Conv2DInputBackward(_, _, _ *RawTensor, _, _ int) *RawTensor {
	panic("MockBackend.Conv2DInputBackward: not implemented (test-only backend)")
}

// Conv2DKernelBackward computes gradient w.r.t. kernel for Conv2D.
// Stub implementation for MockBackend (test-only).
func (m *MockBackend) Conv2DKernelBackward(_, _, _ *RawTensor, _, _ int) *RawTensor {
	panic("MockBackend.Conv2DKernelBackward: not implemented (test-only backend)")
}

// MaxPool2DBackward computes gradient w.r.t. input for MaxPool2D.
// Stub implementation for MockBackend (test-only).
func (m *MockBackend) MaxPool2DBackward(_, _ *RawTensor, _ []int, _, _ int) *RawTensor {
	panic("MockBackend.MaxPool2DBackward: not implemented (test-only backend)")
}
