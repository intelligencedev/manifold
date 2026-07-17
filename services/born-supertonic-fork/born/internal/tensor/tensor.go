package tensor

import "fmt"

// Tensor is a generic tensor with type T and backend B.
// It provides type-safe operations over multi-dimensional arrays.
//
// Type Parameters:
//   - T: Data type (must satisfy DType constraint)
//   - B: Computation backend (must implement Backend interface)
//
// Example:
//
//	backend := cpu.New()
//	t := tensor.Zeros[float32](Shape{3, 4}, backend)
//	result := t.Add(t) // Type-safe addition
type Tensor[T DType, B Backend] struct {
	raw          *RawTensor
	backend      B
	grad         *Tensor[T, B] // Gradient tensor (for autodiff, Phase 1-4)
	requiresGrad bool          // Whether to compute gradients for this tensor
}

// New creates a Tensor from a RawTensor and backend.
func New[T DType, B Backend](raw *RawTensor, b B) *Tensor[T, B] {
	return &Tensor[T, B]{
		raw:     raw,
		backend: b,
		grad:    nil,
	}
}

// FromSlice creates a tensor from a Go slice.
// The slice is copied into the tensor's memory.
func FromSlice[T DType, B Backend](data []T, shape Shape, b B) (*Tensor[T, B], error) {
	if shape.NumElements() != len(data) {
		return nil, fmt.Errorf("shape %v requires %d elements, but got %d", shape, shape.NumElements(), len(data))
	}

	var dummy T
	dtype := inferDataType(dummy)

	raw, err := NewRaw(shape, dtype, b.Device())
	if err != nil {
		return nil, err
	}

	// Copy data into raw tensor
	t := New[T, B](raw, b)
	copy(t.Data(), data)

	return t, nil
}

// Shape returns the tensor's shape.
func (t *Tensor[T, B]) Shape() Shape {
	return t.raw.Shape()
}

// DType returns the tensor's data type.
func (t *Tensor[T, B]) DType() DataType {
	return t.raw.DType()
}

// Device returns the tensor's compute device.
func (t *Tensor[T, B]) Device() Device {
	return t.raw.Device()
}

// NumElements returns the total number of elements.
func (t *Tensor[T, B]) NumElements() int {
	return t.raw.NumElements()
}

// Raw returns the underlying RawTensor.
// Used by backend implementations for low-level operations.
func (t *Tensor[T, B]) Raw() *RawTensor {
	return t.raw
}

// Backend returns the computation backend.
func (t *Tensor[T, B]) Backend() B {
	return t.backend
}

// Grad returns the gradient tensor (if computed by autodiff).
func (t *Tensor[T, B]) Grad() *Tensor[T, B] {
	return t.grad
}

// SetGrad sets the gradient tensor.
// Used internally by autodiff (TASK-004).
func (t *Tensor[T, B]) SetGrad(grad *Tensor[T, B]) {
	t.grad = grad
}

// Detach returns a new tensor that shares the same data but doesn't track gradients.
//
// This is useful for:
//   - Stopping gradient flow at a specific point
//   - Creating targets in reinforcement learning (no backprop through target)
//   - HRM carry states between iterations (detach to prevent long gradient chains)
//   - Teacher-student training (stop gradients through teacher)
//
// The returned tensor shares the underlying data (zero-copy) but has no gradient
// tracking. Any operations on the detached tensor won't appear in the autodiff tape.
//
// Example:
//
//	// Training with detached target
//	prediction := model.Forward(input)
//	target := target_model.Forward(input).Detach()  // No gradients through target
//	loss := prediction.Sub(target).Pow(2).Mean()
//
//	// HRM carry state
//	newCarry := Carry{
//	    zH: zH.Detach(),  // Break gradient chain
//	    zL: zL.Detach(),
//	}
func (t *Tensor[T, B]) Detach() *Tensor[T, B] {
	// Clone RawTensor so the detached tensor has independent GPU buffer lifecycle.
	// Without Clone, ClearTape releasing the original's GPU data also kills the
	// detached tensor's buffer — causing use-after-free in carry state across steps.
	return &Tensor[T, B]{
		raw:          t.raw.Clone(),
		backend:      t.backend,
		grad:         nil,
		requiresGrad: false,
	}
}

// Persist marks this tensor's GPU data as persistent, so it survives
// ReclaimMemory calls between training steps. Use for any tensor that
// must live across step boundaries: carry state, running statistics,
// auxiliary accumulators. Returns the tensor for chaining.
//
// Without Persist, ReclaimMemory releases all non-persistent GPU tensors
// with refcount <= 1 — which includes carry state after ClearTape drops
// the tape's reference.
func (t *Tensor[T, B]) Persist() *Tensor[T, B] {
	t.raw.SetGPUPersistent(true)
	return t
}

// Unpersist removes the persistent flag, allowing ReclaimMemory to
// release this tensor's GPU data. Use when carry state is no longer needed.
func (t *Tensor[T, B]) Unpersist() *Tensor[T, B] {
	t.raw.SetGPUPersistent(false)
	return t
}

// Data returns a typed slice view of the tensor's data.
// The slice directly accesses the underlying memory (zero-copy).
//
// WARNING: Modifications to the returned slice will modify the tensor.
func (t *Tensor[T, B]) Data() []T {
	var dummy T
	switch any(dummy).(type) {
	case float32:
		return any(t.raw.AsFloat32()).([]T)
	case float64:
		return any(t.raw.AsFloat64()).([]T)
	case int32:
		return any(t.raw.AsInt32()).([]T)
	case int64:
		return any(t.raw.AsInt64()).([]T)
	case uint8:
		return any(t.raw.AsUint8()).([]T)
	case bool:
		return any(t.raw.AsBool()).([]T)
	default:
		panic("unsupported type")
	}
}

// Item returns the scalar value of a 0-D tensor.
// Panics if the tensor is not a scalar.
func (t *Tensor[T, B]) Item() T {
	if len(t.Shape()) != 0 || t.NumElements() != 1 {
		panic(fmt.Sprintf("Item() only works for scalar tensors, got shape %v", t.Shape()))
	}
	return t.Data()[0]
}

// At returns the element at the given indices.
// Panics if indices are out of bounds.
//
// Example:
//
//	t := tensor.Zeros[float32](Shape{3, 4}, backend)
//	value := t.At(1, 2) // Row 1, column 2
func (t *Tensor[T, B]) At(indices ...int) T {
	if len(indices) != len(t.Shape()) {
		panic(fmt.Sprintf("expected %d indices, got %d", len(t.Shape()), len(indices)))
	}

	// Calculate flat index using strides
	offset := 0
	strides := t.raw.Strides()
	for i, idx := range indices {
		if idx < 0 || idx >= t.Shape()[i] {
			panic(fmt.Sprintf("index %d out of bounds for dimension %d (size %d)", idx, i, t.Shape()[i]))
		}
		offset += idx * strides[i]
	}

	return t.Data()[offset]
}

// Set sets the element at the given indices.
// Panics if indices are out of bounds.
func (t *Tensor[T, B]) Set(value T, indices ...int) {
	if len(indices) != len(t.Shape()) {
		panic(fmt.Sprintf("expected %d indices, got %d", len(t.Shape()), len(indices)))
	}

	offset := 0
	strides := t.raw.Strides()
	for i, idx := range indices {
		if idx < 0 || idx >= t.Shape()[i] {
			panic(fmt.Sprintf("index %d out of bounds for dimension %d (size %d)", idx, i, t.Shape()[i]))
		}
		offset += idx * strides[i]
	}

	t.Data()[offset] = value
}

// String returns a human-readable representation of the tensor.
func (t *Tensor[T, B]) String() string {
	return fmt.Sprintf("Tensor[%s]%v on %s", t.raw.DType(), t.raw.Shape(), t.raw.Device())
}

// Clone creates a deep copy of the tensor.
func (t *Tensor[T, B]) Clone() *Tensor[T, B] {
	return &Tensor[T, B]{
		raw:          t.raw.Clone(),
		backend:      t.backend,
		grad:         nil,   // Don't clone gradient
		requiresGrad: false, // Don't clone gradient tracking
	}
}

// RequireGrad marks this tensor for gradient computation.
// When called, subsequent operations involving this tensor will be
// tracked in the computation graph (if using an AutodiffBackend).
//
// Returns the tensor itself for method chaining.
//
// Example:
//
//	x := tensor.Ones[float32](Shape{2, 2}, autodiffBackend).RequireGrad()
//	y := x.Mul(x) // Operations are tracked
//	y.Backward()  // Computes gradients
//	fmt.Println(x.Grad()) // dy/dx available
func (t *Tensor[T, B]) RequireGrad() *Tensor[T, B] {
	t.requiresGrad = true
	return t
}

// RequiresGrad returns true if this tensor requires gradient computation.
func (t *Tensor[T, B]) RequiresGrad() bool {
	return t.requiresGrad
}
