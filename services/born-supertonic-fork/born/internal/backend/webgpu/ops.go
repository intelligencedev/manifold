//go:build windows || linux

package webgpu

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// Add performs element-wise addition on GPU.
// Supports float32 and int32 dtypes.
// In LazyMode (default), returns a lazy tensor that keeps data on GPU.
func (b *Backend) Add(a, other *tensor.RawTensor) *tensor.RawTensor {
	shaderName, shaderCode := selectBinaryShader(a.DType(), "add", addShader, addShaderInt32)

	var result *tensor.RawTensor
	var err error

	if b.LazyMode {
		result, err = b.runBinaryOpLazy(a, other, shaderName, shaderCode)
	} else {
		result, err = b.runBinaryOp(a, other, shaderName, shaderCode)
	}

	if err != nil {
		panic("webgpu: Add: " + err.Error())
	}
	return result
}

// Sub performs element-wise subtraction on GPU.
// Supports float32 and int32 dtypes.
// In LazyMode (default), returns a lazy tensor that keeps data on GPU.
func (b *Backend) Sub(a, other *tensor.RawTensor) *tensor.RawTensor {
	shaderName, shaderCode := selectBinaryShader(a.DType(), "sub", subShader, subShaderInt32)

	var result *tensor.RawTensor
	var err error

	if b.LazyMode {
		result, err = b.runBinaryOpLazy(a, other, shaderName, shaderCode)
	} else {
		result, err = b.runBinaryOp(a, other, shaderName, shaderCode)
	}

	if err != nil {
		panic("webgpu: Sub: " + err.Error())
	}
	return result
}

// Mul performs element-wise multiplication on GPU.
// Supports float32 and int32 dtypes.
// In LazyMode (default), returns a lazy tensor that keeps data on GPU.
func (b *Backend) Mul(a, other *tensor.RawTensor) *tensor.RawTensor {
	shaderName, shaderCode := selectBinaryShader(a.DType(), "mul", mulShader, mulShaderInt32)

	var result *tensor.RawTensor
	var err error

	if b.LazyMode {
		result, err = b.runBinaryOpLazy(a, other, shaderName, shaderCode)
	} else {
		result, err = b.runBinaryOp(a, other, shaderName, shaderCode)
	}

	if err != nil {
		panic("webgpu: Mul: " + err.Error())
	}
	return result
}

// Div performs element-wise division on GPU.
// Supports float32 and int32 dtypes.
// In LazyMode (default), returns a lazy tensor that keeps data on GPU.
func (b *Backend) Div(a, other *tensor.RawTensor) *tensor.RawTensor {
	shaderName, shaderCode := selectBinaryShader(a.DType(), "div", divShader, divShaderInt32)

	var result *tensor.RawTensor
	var err error

	if b.LazyMode {
		result, err = b.runBinaryOpLazy(a, other, shaderName, shaderCode)
	} else {
		result, err = b.runBinaryOp(a, other, shaderName, shaderCode)
	}

	if err != nil {
		panic("webgpu: Div: " + err.Error())
	}
	return result
}

// selectBinaryShader selects the appropriate shader based on dtype.
func selectBinaryShader(dtype tensor.DataType, baseName, float32Shader, int32Shader string) (string, string) {
	switch dtype {
	case tensor.Float32:
		return baseName, float32Shader
	case tensor.Int32:
		return baseName + "Int32", int32Shader
	default:
		panic("webgpu: unsupported dtype for binary operation: " + dtype.String())
	}
}

// MatMul performs matrix multiplication on GPU.
func (b *Backend) MatMul(a, other *tensor.RawTensor) *tensor.RawTensor {
	var result *tensor.RawTensor
	var err error

	if b.LazyMode {
		result, err = b.runMatMulLazy(a, other)
	} else {
		result, err = b.runMatMul(a, other)
	}

	if err != nil {
		panic("webgpu: MatMul: " + err.Error())
	}
	return result
}

// BatchMatMul performs batched matrix multiplication on GPU.
// Supports 3D tensors [batch, M, K] @ [batch, K, N] -> [batch, M, N]
// and 4D tensors [batch, heads, M, K] @ [batch, heads, K, N].
func (b *Backend) BatchMatMul(a, other *tensor.RawTensor) *tensor.RawTensor {
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runBatchMatMulLazy(a, other)
	} else {
		result, err = b.runBatchMatMul(a, other)
	}
	if err != nil {
		panic("webgpu: BatchMatMul: " + err.Error())
	}
	return result
}

// Conv2D performs 2D convolution on GPU.
// Input shape: [batch, in_channels, height, width].
// Kernel shape: [out_channels, in_channels, kH, kW].
func (b *Backend) Conv2D(input, kernel *tensor.RawTensor, stride, padding int) *tensor.RawTensor {
	result, err := b.runConv2D(input, kernel, stride, padding)
	if err != nil {
		panic("webgpu: Conv2D: " + err.Error())
	}
	return result
}

// MaxPool2D performs 2D max pooling on GPU.
// Input shape: [batch, channels, height, width].
func (b *Backend) MaxPool2D(input *tensor.RawTensor, kernelSize, stride int) *tensor.RawTensor {
	result, err := b.runMaxPool2D(input, kernelSize, stride)
	if err != nil {
		panic("webgpu: MaxPool2D: " + err.Error())
	}
	return result
}

// Reshape returns a tensor with new shape backed by the same data.
//
// In LazyMode, when the source tensor has unrealized GPU data, performs a
// GPU-to-GPU buffer copy (zero CPU allocation) to create the reshaped result.
// Falls back to CPU copy otherwise.
func (b *Backend) Reshape(t *tensor.RawTensor, newShape tensor.Shape) *tensor.RawTensor {
	if err := newShape.Validate(); err != nil {
		panic("webgpu: reshape: invalid shape: " + err.Error())
	}

	if t.NumElements() != newShape.NumElements() {
		panic("webgpu: reshape: incompatible number of elements")
	}

	// LazyMode GPU path: source lives on GPU — perform GPU-to-GPU copy (zero CPU).
	if b.LazyMode {
		if gpuData := t.GPUData(); gpuData != nil && !gpuData.IsRealized() {
			if result, err := b.runReshapeLazy(t, newShape); err == nil {
				return result
			}
			// On error (e.g., unsupported dtype), fall through to CPU path.
		}
	}

	// CPU path: allocate new tensor and copy data from host buffer.
	result, err := tensor.NewRaw(newShape, t.DType(), tensor.WebGPU)
	if err != nil {
		panic("webgpu: reshape: " + err.Error())
	}
	copy(result.Data(), t.Data())
	return result
}

// Transpose transposes the tensor by permuting its dimensions.
// GPU-accelerated for 2D (optimized) and ND tensors (up to 6D).
func (b *Backend) Transpose(t *tensor.RawTensor, axes ...int) *tensor.RawTensor {
	shape := t.Shape()
	ndim := len(shape)

	// Check for no-op case early
	if isNoOpTranspose(axes, ndim) {
		return t
	}

	// 2D transpose (matrix): use optimized 2D shader
	if ndim == 2 {
		validate2DTransposeAxes(axes)
		var result *tensor.RawTensor
		var err error
		if b.LazyMode {
			result, err = b.runTransposeLazy(t)
		} else {
			result, err = b.runTranspose(t)
		}
		if err != nil {
			panic("webgpu: Transpose: " + err.Error())
		}
		return result
	}

	// Multi-dimensional (3D, 4D, etc.): use GPU-accelerated ND transpose
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runTransposeNDLazy(t, axes)
	} else {
		result, err = b.runTransposeND(t, axes)
	}
	if err != nil {
		panic("webgpu: Transpose: " + err.Error())
	}
	return result
}

// isNoOpTranspose checks if transpose is a no-op.
func isNoOpTranspose(axes []int, ndim int) bool {
	if len(axes) == 0 {
		return false
	}
	if len(axes) != ndim {
		return false
	}
	for i, ax := range axes {
		if ax != i {
			return false
		}
	}
	return true
}

// validate2DTransposeAxes validates axes for 2D transpose.
func validate2DTransposeAxes(axes []int) {
	if len(axes) > 0 && (len(axes) != 2 || !isValid2DAxes(axes)) {
		panic("webgpu: Transpose: invalid axes for 2D tensor")
	}
}

// isValid2DAxes checks if axes are valid for 2D transpose.
func isValid2DAxes(axes []int) bool {
	return (axes[0] == 0 && axes[1] == 1) || (axes[0] == 1 && axes[1] == 0)
}

// ReLU applies ReLU activation: max(0, x).
func (b *Backend) ReLU(x *tensor.RawTensor) *tensor.RawTensor {
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runUnaryOpLazy(x, "relu", reluShader)
	} else {
		result, err = b.runUnaryOp(x, "relu", reluShader)
	}
	if err != nil {
		panic("webgpu: ReLU: " + err.Error())
	}
	return result
}

// Sigmoid applies sigmoid activation: 1 / (1 + exp(-x)).
func (b *Backend) Sigmoid(x *tensor.RawTensor) *tensor.RawTensor {
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runUnaryOpLazy(x, "sigmoid", sigmoidShader)
	} else {
		result, err = b.runUnaryOp(x, "sigmoid", sigmoidShader)
	}
	if err != nil {
		panic("webgpu: Sigmoid: " + err.Error())
	}
	return result
}

// Tanh applies tanh activation.
func (b *Backend) Tanh(x *tensor.RawTensor) *tensor.RawTensor {
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runUnaryOpLazy(x, "tanh", tanhShader)
	} else {
		result, err = b.runUnaryOp(x, "tanh", tanhShader)
	}
	if err != nil {
		panic("webgpu: Tanh: " + err.Error())
	}
	return result
}

// SiLU applies SiLU (Swish) activation: x * sigmoid(x).
func (b *Backend) SiLU(x *tensor.RawTensor) *tensor.RawTensor {
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runUnaryOpLazy(x, "silu", siluShader)
	} else {
		result, err = b.runUnaryOp(x, "silu", siluShader)
	}
	if err != nil {
		panic("webgpu: SiLU: " + err.Error())
	}
	return result
}

// Exp computes element-wise exponential on GPU.
func (b *Backend) Exp(x *tensor.RawTensor) *tensor.RawTensor {
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runUnaryOpLazy(x, "exp", expShader)
	} else {
		result, err = b.runUnaryOp(x, "exp", expShader)
	}
	if err != nil {
		panic("webgpu: Exp: " + err.Error())
	}
	return result
}

// Sqrt computes element-wise square root on GPU.
func (b *Backend) Sqrt(x *tensor.RawTensor) *tensor.RawTensor {
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runUnaryOpLazy(x, "sqrt", sqrtShader)
	} else {
		result, err = b.runUnaryOp(x, "sqrt", sqrtShader)
	}
	if err != nil {
		panic("webgpu: Sqrt: " + err.Error())
	}
	return result
}

// Rsqrt computes element-wise reciprocal square root on GPU.
func (b *Backend) Rsqrt(x *tensor.RawTensor) *tensor.RawTensor {
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runUnaryOpLazy(x, "rsqrt", rsqrtShader)
	} else {
		result, err = b.runUnaryOp(x, "rsqrt", rsqrtShader)
	}
	if err != nil {
		panic("webgpu: Rsqrt: " + err.Error())
	}
	return result
}

// Cos computes element-wise cosine on GPU.
func (b *Backend) Cos(x *tensor.RawTensor) *tensor.RawTensor {
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runUnaryOpLazy(x, "cos", cosShader)
	} else {
		result, err = b.runUnaryOp(x, "cos", cosShader)
	}
	if err != nil {
		panic("webgpu: Cos: " + err.Error())
	}
	return result
}

// Sin computes element-wise sine on GPU.
func (b *Backend) Sin(x *tensor.RawTensor) *tensor.RawTensor {
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runUnaryOpLazy(x, "sin", sinShader)
	} else {
		result, err = b.runUnaryOp(x, "sin", sinShader)
	}
	if err != nil {
		panic("webgpu: Sin: " + err.Error())
	}
	return result
}

// Sign computes element-wise sign function on GPU.
//
// Only supports float32 dtype for now. Will raise a panic if called with unsupported dtype.
func (b *Backend) Sign(x *tensor.RawTensor) *tensor.RawTensor {
	if x.DType() != tensor.Float32 {
		panic(fmt.Sprintf("webgpu: Sign currently supports only Float32, got %s", x.DType()))
	}
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runUnaryOpLazy(x, "sign", signShader)
	} else {
		result, err = b.runUnaryOp(x, "sign", signShader)
	}
	if err != nil {
		panic("webgpu: Sign: " + err.Error())
	}
	return result
}

// Abs computes element-wise absolute value on GPU.
//
// Only supports float32 dtype for now. Will raise a panic if called with unsupported dtype.
func (b *Backend) Abs(x *tensor.RawTensor) *tensor.RawTensor {
	if x.DType() != tensor.Float32 {
		panic(fmt.Sprintf("webgpu: Abs currently supports only Float32, got %s", x.DType()))
	}
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runUnaryOpLazy(x, "abs", absShader)
	} else {
		result, err = b.runUnaryOp(x, "abs", absShader)
	}
	if err != nil {
		panic("webgpu: Abs: " + err.Error())
	}
	return result
}

// Clamp restricts tensor values element-wise to [minBound, maxBound].
//
// Supports float32 and int32 dtypes. Will raise a panic if called with unsupported dtype.
func (b *Backend) Clamp(x *tensor.RawTensor, minBound, maxBound any) *tensor.RawTensor {
	if x.DType() != tensor.Float32 && x.DType() != tensor.Int32 {
		panic(fmt.Sprintf("webgpu: Clamp currently supports only Float32 and Int32, got %s", x.DType()))
	}
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runClampLazy(x, minBound, maxBound)
	} else {
		result, err = b.runClamp(x, minBound, maxBound)
	}
	if err != nil {
		panic("webgpu: Clamp: " + err.Error())
	}
	return result
}

// Erf computes element-wise error function on GPU.
func (b *Backend) Erf(x *tensor.RawTensor) *tensor.RawTensor {
	var result *tensor.RawTensor
	var err error
	if b.LazyMode {
		result, err = b.runUnaryOpLazy(x, "erf", erfShader)
	} else {
		result, err = b.runUnaryOp(x, "erf", erfShader)
	}
	if err != nil {
		panic("webgpu: Erf: " + err.Error())
	}
	return result
}

// SumDim sums along a dimension.
//
// In LazyMode, dispatches a GPU compute shader (sumDimGeneralShader) that keeps
// data on GPU — zero CPU allocation for the result. Falls back to CPU reduction
// when LazyMode is disabled or the source tensor is not GPU-resident.
func (b *Backend) SumDim(x *tensor.RawTensor, dim int, keepDim bool) *tensor.RawTensor {
	shape := x.Shape()
	ndim := len(shape)

	// Normalize negative dimension.
	if dim < 0 {
		dim = ndim + dim
	}

	if dim < 0 || dim >= ndim {
		panic("webgpu: SumDim: dimension out of range")
	}

	// LazyMode GPU path.
	if b.LazyMode && x.DType() == tensor.Float32 {
		result, err := b.runSumDimLazy(x, dim, keepDim)
		if err != nil {
			panic("webgpu: SumDim: " + err.Error())
		}
		return result
	}

	// CPU fallback path.
	var outShape tensor.Shape
	if keepDim {
		outShape = shape.Clone()
		outShape[dim] = 1
	} else {
		outShape = make(tensor.Shape, 0, ndim-1)
		for i := 0; i < ndim; i++ {
			if i != dim {
				outShape = append(outShape, shape[i])
			}
		}
	}

	result, err := tensor.NewRaw(outShape, x.DType(), tensor.WebGPU)
	if err != nil {
		panic("webgpu: SumDim: " + err.Error())
	}

	if x.DType() == tensor.Float32 {
		sumDimFloat32(x.AsFloat32(), result.AsFloat32(), shape, dim)
	} else {
		panic("webgpu: SumDim: only float32 supported")
	}

	return result
}

// sumDimFloat32 performs dimension reduction for float32 tensors.
func sumDimFloat32(data, result []float32, shape tensor.Shape, dim int) {
	for i := range result {
		result[i] = 0
	}

	strides := shape.ComputeStrides()
	numElements := shape.NumElements()

	outShape := shape.Clone()
	outShape[dim] = 1
	outStrides := outShape.ComputeStrides()

	for i := 0; i < numElements; i++ {
		outIdx := 0
		temp := i
		for d := 0; d < len(shape); d++ {
			coord := temp / strides[d]
			temp %= strides[d]

			if d != dim {
				outIdx += coord * outStrides[d]
			}
		}
		result[outIdx] += data[i]
	}
}

// MeanDim computes mean along a dimension.
//
// In LazyMode, composes the lazy SumDim GPU path with MulScalar(1/dimSize),
// keeping all intermediate results on GPU — zero CPU allocation. Falls back to
// CPU arithmetic when LazyMode is disabled.
func (b *Backend) MeanDim(x *tensor.RawTensor, dim int, keepDim bool) *tensor.RawTensor {
	shape := x.Shape()
	ndim := len(shape)
	if dim < 0 {
		dim = ndim + dim
	}

	divisor := float32(shape[dim])

	// LazyMode GPU path: SumDim is already lazy; MulScalar is lazy too.
	// No CPU data touches either intermediate tensor.
	if b.LazyMode && x.DType() == tensor.Float32 {
		sumResult := b.SumDim(x, dim, keepDim)     // returns lazy GPU tensor
		return b.MulScalar(sumResult, 1.0/divisor) // returns lazy GPU tensor
	}

	// CPU fallback path.
	sumResult := b.SumDim(x, dim, keepDim)
	data := sumResult.AsFloat32()
	for i := range data {
		data[i] /= divisor
	}
	return sumResult
}

// Cat concatenates tensors along the specified dimension.
//
// In LazyMode with float32 inputs, dispatches GPU compute passes (catShader) to
// copy each input into the correct region of a pre-allocated output buffer — zero
// CPU allocation. Falls back to CPU concatenation otherwise.
func (b *Backend) Cat(tensors []*tensor.RawTensor, dim int) *tensor.RawTensor {
	if len(tensors) == 0 {
		panic("webgpu: Cat: at least one tensor required")
	}

	shape := tensors[0].Shape()
	ndim := len(shape)
	dtype := tensors[0].DType()

	if dim < 0 {
		dim = ndim + dim
	}

	if dim < 0 || dim >= ndim {
		panic("webgpu: Cat: dimension out of range")
	}

	// LazyMode GPU path (float32 only).
	if b.LazyMode && dtype == tensor.Float32 {
		result, err := b.runCatLazy(tensors, dim)
		if err != nil {
			panic("webgpu: Cat: " + err.Error())
		}
		return result
	}

	// CPU fallback path.
	totalDim := 0
	for _, t := range tensors {
		totalDim += t.Shape()[dim]
	}

	outShape := shape.Clone()
	outShape[dim] = totalDim

	result, err := tensor.NewRaw(outShape, dtype, tensor.WebGPU)
	if err != nil {
		panic("webgpu: Cat: " + err.Error())
	}

	if dtype == tensor.Float32 {
		catFloat32WebGPU(tensors, result, dim)
	} else {
		panic("webgpu: Cat: only float32 supported")
	}

	return result
}

func catFloat32WebGPU(tensors []*tensor.RawTensor, result *tensor.RawTensor, dim int) {
	outData := result.AsFloat32()
	outShape := result.Shape()
	outStrides := outShape.ComputeStrides()

	offset := 0
	for _, t := range tensors {
		data := t.AsFloat32()
		shape := t.Shape()
		strides := shape.ComputeStrides()
		numElements := shape.NumElements()

		for i := 0; i < numElements; i++ {
			outIdx := 0
			temp := i
			for d := 0; d < len(shape); d++ {
				coord := temp / strides[d]
				temp %= strides[d]

				if d == dim {
					coord += offset
				}
				outIdx += coord * outStrides[d]
			}
			outData[outIdx] = data[i]
		}
		offset += shape[dim]
	}
}

// Chunk splits tensor into n equal parts along the specified dimension.
//
// In LazyMode with float32 inputs, dispatches GPU compute passes (chunkShader)
// to copy each slice into a freshly-allocated output buffer — zero CPU allocation.
// Falls back to CPU splitting otherwise.
func (b *Backend) Chunk(x *tensor.RawTensor, n, dim int) []*tensor.RawTensor {
	if n <= 0 {
		panic("webgpu: Chunk: n must be positive")
	}

	shape := x.Shape()
	ndim := len(shape)

	if dim < 0 {
		dim = ndim + dim
	}

	if dim < 0 || dim >= ndim {
		panic("webgpu: Chunk: dimension out of range")
	}

	dimSize := shape[dim]
	if dimSize%n != 0 {
		panic("webgpu: Chunk: dimension not divisible by n")
	}

	// LazyMode GPU path (float32 only).
	if b.LazyMode && x.DType() == tensor.Float32 {
		results, err := b.runChunkLazy(x, n, dim)
		if err != nil {
			panic("webgpu: Chunk: " + err.Error())
		}
		return results
	}

	// CPU fallback path.
	chunkSize := dimSize / n
	chunkShape := shape.Clone()
	chunkShape[dim] = chunkSize

	results := make([]*tensor.RawTensor, n)
	for i := 0; i < n; i++ {
		chunk, err := tensor.NewRaw(chunkShape, x.DType(), tensor.WebGPU)
		if err != nil {
			panic("webgpu: Chunk: " + err.Error())
		}
		results[i] = chunk
	}

	if x.DType() == tensor.Float32 {
		chunkFloat32WebGPU(x, results, dim, chunkSize)
	} else {
		panic("webgpu: Chunk: only float32 supported")
	}

	return results
}

func chunkFloat32WebGPU(x *tensor.RawTensor, results []*tensor.RawTensor, dim, chunkSize int) {
	data := x.AsFloat32()
	shape := x.Shape()
	strides := shape.ComputeStrides()
	numElements := shape.NumElements()

	for i := 0; i < numElements; i++ {
		temp := i
		coords := make([]int, len(shape))
		for d := 0; d < len(shape); d++ {
			coords[d] = temp / strides[d]
			temp %= strides[d]
		}

		chunkIdx := coords[dim] / chunkSize
		localCoord := coords[dim] % chunkSize

		outShape := results[chunkIdx].Shape()
		outStrides := outShape.ComputeStrides()
		outIdx := 0
		for d := 0; d < len(coords); d++ {
			if d == dim {
				outIdx += localCoord * outStrides[d]
			} else {
				outIdx += coords[d] * outStrides[d]
			}
		}

		results[chunkIdx].AsFloat32()[outIdx] = data[i]
	}
}

// Unsqueeze adds a dimension of size 1 at the specified position.
func (b *Backend) Unsqueeze(x *tensor.RawTensor, dim int) *tensor.RawTensor {
	shape := x.Shape()
	ndim := len(shape)

	if dim < 0 {
		dim = ndim + 1 + dim
	}

	if dim < 0 || dim > ndim {
		panic("webgpu: Unsqueeze: dimension out of range")
	}

	newShape := make(tensor.Shape, ndim+1)
	for i := 0; i < dim; i++ {
		newShape[i] = shape[i]
	}
	newShape[dim] = 1
	for i := dim; i < ndim; i++ {
		newShape[i+1] = shape[i]
	}

	return b.Reshape(x, newShape)
}

// SelectAdd performs a scatter-add along the specified dimension.
//
// Used primarily in Embedding backward to accumulate gradient rows into the weight
// gradient tensor. In LazyMode, dispatches a GPU compute shader that keeps the
// result on GPU without requiring f32 atomics (per-destination-row approach).
// Falls back to CPU for non-lazy mode.
func (b *Backend) SelectAdd(dest *tensor.RawTensor, dim int, indices, src *tensor.RawTensor) *tensor.RawTensor {
	if indices.DType() != tensor.Int32 {
		panic("webgpu: SelectAdd: indices must be int32")
	}

	destShape := dest.Shape()
	srcShape := src.Shape()
	ndim := len(destShape)

	if dim < 0 {
		dim += ndim
	}
	if dim < 0 || dim >= ndim {
		panic(fmt.Sprintf("webgpu: SelectAdd: dim %d out of range for %dD tensor", dim, ndim))
	}

	numIndices := indices.Shape()[0]

	if len(srcShape) != ndim {
		panic(fmt.Sprintf("webgpu: SelectAdd: src rank %d != dest rank %d", len(srcShape), ndim))
	}
	if srcShape[dim] != numIndices {
		panic(fmt.Sprintf("webgpu: SelectAdd: src dim %d (%d) != len(indices) (%d)", dim, srcShape[dim], numIndices))
	}

	// GPU path: selectAddShader handles dim=1 with 2-D tensors [numRows, innerSize].
	// For the common Embedding backward case this is always 2-D with dim=0.
	if b.LazyMode && ndim == 2 && dim == 0 && dest.DType() == tensor.Float32 {
		result, err := b.runSelectAddLazy(dest, indices, src)
		if err != nil {
			panic("webgpu: SelectAdd: " + err.Error())
		}
		return result
	}

	// CPU fallback for non-lazy mode or unsupported shapes/dtypes.
	return b.selectAddCPU(dest, dim, indices, src, destShape, srcShape, numIndices)
}

// selectAddCPU is the CPU fallback for SelectAdd.
func (b *Backend) selectAddCPU(
	dest *tensor.RawTensor, dim int, indices, src *tensor.RawTensor,
	destShape, srcShape tensor.Shape, numIndices int,
) *tensor.RawTensor {
	result, err := tensor.NewRaw(destShape, dest.DType(), tensor.WebGPU)
	if err != nil {
		panic("webgpu: SelectAdd: " + err.Error())
	}
	copy(result.Data(), dest.Data())

	idxData := indices.AsInt32()
	dstStrides := destShape.ComputeStrides()
	srcStrides := srcShape.ComputeStrides()
	innerSize := srcShape.NumElements() / srcShape[dim]
	srcDimStride := srcStrides[dim]
	dstDimStride := dstStrides[dim]

	switch dest.DType() {
	case tensor.Float32:
		dst := result.AsFloat32()
		srcData := src.AsFloat32()
		for i := 0; i < numIndices; i++ {
			idx := int(idxData[i])
			if idx < 0 || idx >= destShape[dim] {
				panic(fmt.Sprintf("webgpu: SelectAdd: index %d out of bounds [0, %d)", idx, destShape[dim]))
			}
			for j := 0; j < innerSize; j++ {
				nonDimFlat := webgpuSelectAddNonDimFlat(j, srcShape, dstStrides, dim)
				dst[idx*dstDimStride+nonDimFlat] += srcData[i*srcDimStride+nonDimFlat]
			}
		}
	default:
		panic(fmt.Sprintf("webgpu: SelectAdd: unsupported dtype %s", dest.DType()))
	}

	return result
}

// webgpuSelectAddNonDimFlat computes the flat-index contribution from dimensions
// other than dim, given a linear index j enumerating elements in those dimensions.
// Uses srcShape for coordinate decomposition and dstStrides for flat-index mapping
// (they are identical for non-scatter dims by the SelectAdd precondition).
func webgpuSelectAddNonDimFlat(j int, srcShape tensor.Shape, dstStrides []int, dim int) int {
	ndim := len(srcShape)
	flat := 0
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
		flat += coord * dstStrides[d]
	}
	return flat
}

// Squeeze removes a dimension of size 1 at the specified position.
func (b *Backend) Squeeze(x *tensor.RawTensor, dim int) *tensor.RawTensor {
	shape := x.Shape()
	ndim := len(shape)

	if dim < 0 {
		dim = ndim + dim
	}

	if dim < 0 || dim >= ndim {
		panic("webgpu: Squeeze: dimension out of range")
	}

	if shape[dim] != 1 {
		panic("webgpu: Squeeze: dimension size must be 1")
	}

	newShape := make(tensor.Shape, 0, ndim-1)
	for i := 0; i < ndim; i++ {
		if i != dim {
			newShape = append(newShape, shape[i])
		}
	}

	return b.Reshape(x, newShape)
}

// ScatterAdd performs a general scatter-add matching Gather backward semantics.
//
// For each element in src (same shape as indices), accumulates into result along dim
// at the position given by the corresponding index value. Follows Burn's float_scatter_add.
//
// In LazyMode, dispatches a GPU compute shader that keeps the result on GPU using a
// per-destination-element approach (no f32 atomics required). Falls back to CPU for
// non-lazy mode.
//
// Returns a new tensor with the same shape as dest. dest is not modified.
func (b *Backend) ScatterAdd(dest *tensor.RawTensor, dim int, indices, src *tensor.RawTensor) *tensor.RawTensor {
	dim = webgpuValidateScatterAdd(dest, dim, indices, src)

	// GPU path in LazyMode: scatterAddShader handles arbitrary N-D tensors (up to 6D).
	if b.LazyMode && dest.DType() == tensor.Float32 {
		result, err := b.runScatterAddLazy(dest, dim, indices, src)
		if err != nil {
			panic("webgpu: ScatterAdd: " + err.Error())
		}
		return result
	}

	// CPU fallback for non-lazy mode or unsupported dtypes.
	return b.scatterAddCPU(dest, dim, indices, src)
}

// scatterAddCPU is the CPU fallback for ScatterAdd.
func (b *Backend) scatterAddCPU(dest *tensor.RawTensor, dim int, indices, src *tensor.RawTensor) *tensor.RawTensor {
	destShape := dest.Shape()
	srcShape := src.Shape()
	indexShape := indices.Shape()

	result, err := tensor.NewRaw(destShape, dest.DType(), tensor.WebGPU)
	if err != nil {
		panic("webgpu: ScatterAdd: " + err.Error())
	}
	copy(result.Data(), dest.Data())

	idxData := indices.AsInt32()
	numElements := src.NumElements()
	ndim := len(destShape)
	srcStrides := srcShape.ComputeStrides()
	dstStrides := destShape.ComputeStrides()
	indexStrides := indexShape.ComputeStrides()

	switch dest.DType() {
	case tensor.Float32:
		webgpuScatterAddFloat32(result.AsFloat32(), src.AsFloat32(), idxData,
			dim, numElements, ndim, destShape, srcStrides, dstStrides, indexStrides)
	default:
		panic(fmt.Sprintf("webgpu: ScatterAdd: unsupported dtype %s", dest.DType()))
	}

	return result
}

// webgpuValidateScatterAdd checks all preconditions and returns the normalized dim.
func webgpuValidateScatterAdd(dest *tensor.RawTensor, dim int, indices, src *tensor.RawTensor) int {
	if indices.DType() != tensor.Int32 {
		panic("webgpu: ScatterAdd: indices must be int32")
	}

	destShape := dest.Shape()
	srcShape := src.Shape()
	indexShape := indices.Shape()
	ndim := len(destShape)

	if dim < 0 {
		dim += ndim
	}
	if dim < 0 || dim >= ndim {
		panic(fmt.Sprintf("webgpu: ScatterAdd: dim %d out of range for %dD tensor", dim, ndim))
	}
	if len(indexShape) != len(srcShape) {
		panic(fmt.Sprintf("webgpu: ScatterAdd: indices rank %d != src rank %d", len(indexShape), len(srcShape)))
	}
	for d := range indexShape {
		if indexShape[d] != srcShape[d] {
			panic(fmt.Sprintf("webgpu: ScatterAdd: indices shape %v != src shape %v", indexShape, srcShape))
		}
	}
	if len(srcShape) != ndim {
		panic(fmt.Sprintf("webgpu: ScatterAdd: src rank %d != dest rank %d", len(srcShape), ndim))
	}
	for d := 0; d < ndim; d++ {
		if d == dim {
			continue
		}
		if srcShape[d] != destShape[d] {
			panic(fmt.Sprintf("webgpu: ScatterAdd: shape mismatch at dim %d: dest=%d src=%d", d, destShape[d], srcShape[d]))
		}
	}
	return dim
}

// webgpuScatterAddFloat32 performs the CPU-fallback scatter-add loop for float32.
func webgpuScatterAddFloat32(dst, srcData []float32, idxData []int32, dim, numElements, ndim int,
	destShape tensor.Shape, srcStrides, dstStrides, indexStrides []int) {
	for i := 0; i < numElements; i++ {
		rem := i
		coords := make([]int, ndim)
		for d := 0; d < ndim; d++ {
			coords[d] = rem / srcStrides[d]
			rem %= srcStrides[d]
		}
		indexIdx := 0
		for d := 0; d < ndim; d++ {
			indexIdx += coords[d] * indexStrides[d]
		}
		idx := int(idxData[indexIdx])
		if idx < 0 || idx >= destShape[dim] {
			panic(fmt.Sprintf("webgpu: ScatterAdd: index %d out of bounds [0, %d)", idx, destShape[dim]))
		}
		dstIdx := 0
		for d := 0; d < ndim; d++ {
			if d == dim {
				dstIdx += idx * dstStrides[d]
			} else {
				dstIdx += coords[d] * dstStrides[d]
			}
		}
		dst[dstIdx] += srcData[i]
	}
}
