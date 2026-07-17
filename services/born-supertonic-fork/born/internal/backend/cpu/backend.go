// Package cpu implements the CPU backend with SIMD optimizations and BLAS integration.
package cpu

import (
	"fmt"
	"math"

	"github.com/born-ml/born/internal/tensor"
)

// CPUBackend implements tensor operations on CPU with optional SIMD and BLAS optimizations.
type CPUBackend struct {
	device tensor.Device
}

// New creates a new CPU backend.
func New() *CPUBackend {
	return &CPUBackend{
		device: tensor.CPU,
	}
}

// Name returns the backend name.
func (cpu *CPUBackend) Name() string {
	return "CPU"
}

// Device returns the compute device.
func (cpu *CPUBackend) Device() tensor.Device {
	return cpu.device
}

// Add performs element-wise addition with NumPy-style broadcasting.
func (cpu *CPUBackend) Add(a, b *tensor.RawTensor) *tensor.RawTensor {
	outShape, needsBroadcast, err := tensor.BroadcastShapes(a.Shape(), b.Shape())
	if err != nil {
		panic(fmt.Sprintf("add: %v", err))
	}

	result, err := tensor.NewRaw(outShape, a.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("add: failed to create result tensor: %v", err))
	}

	// Check for inplace optimization
	if !needsBroadcast && a.Shape().Equal(b.Shape()) {
		// Fast path: same shape, check if we can do inplace
		if a.IsUnique() && a != b {
			// Inplace add into a (safe: operands don't alias)
			addInplace(a, b)
			return a
		}
		// Vectorized add
		addVectorized(result, a, b)
	} else {
		// Slow path: broadcasting required
		addWithBroadcast(result, a, b, outShape)
	}

	return result
}

// Sub performs element-wise subtraction with broadcasting.
func (cpu *CPUBackend) Sub(a, b *tensor.RawTensor) *tensor.RawTensor {
	outShape, needsBroadcast, err := tensor.BroadcastShapes(a.Shape(), b.Shape())
	if err != nil {
		panic(fmt.Sprintf("sub: %v", err))
	}

	result, err := tensor.NewRaw(outShape, a.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("sub: failed to create result tensor: %v", err))
	}

	if !needsBroadcast && a.Shape().Equal(b.Shape()) {
		if a.IsUnique() && a != b {
			subInplace(a, b)
			return a
		}
		subVectorized(result, a, b)
	} else {
		subWithBroadcast(result, a, b, outShape)
	}

	return result
}

// Mul performs element-wise multiplication with broadcasting.
func (cpu *CPUBackend) Mul(a, b *tensor.RawTensor) *tensor.RawTensor {
	outShape, needsBroadcast, err := tensor.BroadcastShapes(a.Shape(), b.Shape())
	if err != nil {
		panic(fmt.Sprintf("mul: %v", err))
	}

	result, err := tensor.NewRaw(outShape, a.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("mul: failed to create result tensor: %v", err))
	}

	if !needsBroadcast && a.Shape().Equal(b.Shape()) {
		if a.IsUnique() && a != b {
			mulInplace(a, b)
			return a
		}
		mulVectorized(result, a, b)
	} else {
		mulWithBroadcast(result, a, b, outShape)
	}

	return result
}

// Div performs element-wise division with broadcasting.
func (cpu *CPUBackend) Div(a, b *tensor.RawTensor) *tensor.RawTensor {
	outShape, needsBroadcast, err := tensor.BroadcastShapes(a.Shape(), b.Shape())
	if err != nil {
		panic(fmt.Sprintf("div: %v", err))
	}

	result, err := tensor.NewRaw(outShape, a.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("div: failed to create result tensor: %v", err))
	}

	if !needsBroadcast && a.Shape().Equal(b.Shape()) {
		if a.IsUnique() && a != b {
			divInplace(a, b)
			return a
		}
		divVectorized(result, a, b)
	} else {
		divWithBroadcast(result, a, b, outShape)
	}

	return result
}

// Reshape returns a tensor with the same data but different shape.
func (cpu *CPUBackend) Reshape(t *tensor.RawTensor, newShape tensor.Shape) *tensor.RawTensor {
	if err := newShape.Validate(); err != nil {
		panic(fmt.Sprintf("reshape: invalid shape: %v", err))
	}

	if t.NumElements() != newShape.NumElements() {
		panic(fmt.Sprintf("reshape: incompatible shapes: %v -> %v (different number of elements)",
			t.Shape(), newShape))
	}

	// Reshape is a view operation (zero-copy)
	reshaped := t.Clone()
	reshaped.Shape() // This will need to be modified to set shape

	// For now, create a new tensor with reshaped data
	// TODO: Optimize this to be a true view
	result, err := tensor.NewRaw(newShape, t.DType(), t.Device())
	if err != nil {
		panic(fmt.Sprintf("reshape: %v", err))
	}

	// Copy data
	copy(result.Data(), t.Data())
	return result
}

// Tanh applies hyperbolic tangent activation.
func (cpu *CPUBackend) Tanh(x *tensor.RawTensor) *tensor.RawTensor {
	result, err := tensor.NewRaw(x.Shape(), x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("tanh: %v", err))
	}

	switch x.DType() {
	case tensor.Float32:
		src := x.AsFloat32()
		dst := result.AsFloat32()
		for i, v := range src {
			dst[i] = float32(math.Tanh(float64(v)))
		}
	case tensor.Float64:
		src := x.AsFloat64()
		dst := result.AsFloat64()
		for i, v := range src {
			dst[i] = math.Tanh(v)
		}
	default:
		panic(fmt.Sprintf("tanh: unsupported dtype %s", x.DType()))
	}

	return result
}

// Transpose transposes the tensor by permuting its dimensions.
func (cpu *CPUBackend) Transpose(t *tensor.RawTensor, axes ...int) *tensor.RawTensor {
	shape := t.Shape()
	ndim := len(shape)

	// Default: reverse all dimensions
	if len(axes) == 0 {
		axes = make([]int, ndim)
		for i := range axes {
			axes[i] = ndim - 1 - i
		}
	}

	// Validate axes
	if len(axes) != ndim {
		panic(fmt.Sprintf("transpose: axes length %d != ndim %d", len(axes), ndim))
	}

	seen := make([]bool, ndim)
	for _, ax := range axes {
		if ax < 0 || ax >= ndim {
			panic(fmt.Sprintf("transpose: invalid axis %d for %dD tensor", ax, ndim))
		}
		if seen[ax] {
			panic(fmt.Sprintf("transpose: duplicate axis %d", ax))
		}
		seen[ax] = true
	}

	// Compute new shape
	newShape := make(tensor.Shape, ndim)
	for i, ax := range axes {
		newShape[i] = shape[ax]
	}

	// Create result tensor
	result, err := tensor.NewRaw(newShape, t.DType(), t.Device())
	if err != nil {
		panic(fmt.Sprintf("transpose: %v", err))
	}

	// Perform transpose
	transposeData(result, t, axes)

	return result
}
