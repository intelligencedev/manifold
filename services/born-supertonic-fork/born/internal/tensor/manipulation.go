package tensor

// Cat concatenates tensors along the specified dimension.
//
// All tensors must have the same shape except along the concatenation dimension.
// Supports negative dim indexing (-1 = last dimension).
//
// Example:
//
//	a := tensor.Randn[float32](Shape{2, 3}, backend)
//	b := tensor.Randn[float32](Shape{2, 5}, backend)
//	c := tensor.Cat([]*Tensor[float32, B]{a, b}, 1) // Shape: [2, 8]
func Cat[T DType, B Backend](tensors []*Tensor[T, B], dim int) *Tensor[T, B] {
	if len(tensors) == 0 {
		panic("cat: at least one tensor required")
	}

	if len(tensors) == 1 {
		// Single tensor - return clone
		return tensors[0].Clone()
	}

	// Extract raw tensors and backend
	rawTensors := make([]*RawTensor, len(tensors))
	backend := tensors[0].backend
	for i, t := range tensors {
		rawTensors[i] = t.raw
	}

	result := backend.Cat(rawTensors, dim)
	return New[T, B](result, backend)
}

// Chunk splits the tensor into n equal parts along the specified dimension.
//
// The dimension size must be divisible by n.
// Supports negative dim indexing (-1 = last dimension).
//
// Example:
//
//	x := tensor.Randn[float32](Shape{2, 3, 6}, backend)
//	parts := x.Chunk(3, -1) // 3 tensors of shape [2, 3, 2]
func (t *Tensor[T, B]) Chunk(n, dim int) []*Tensor[T, B] {
	rawParts := t.backend.Chunk(t.raw, n, dim)
	parts := make([]*Tensor[T, B], len(rawParts))
	for i, raw := range rawParts {
		parts[i] = New[T, B](raw, t.backend)
	}
	return parts
}

// Unsqueeze adds a dimension of size 1 at the specified position.
//
// Supports negative dim indexing.
// This is a view operation (no data copy).
//
// Example:
//
//	x := tensor.Randn[float32](Shape{2, 3}, backend)
//	y := x.Unsqueeze(1)  // Shape: [2, 1, 3]
//	z := x.Unsqueeze(-1) // Shape: [2, 3, 1]
func (t *Tensor[T, B]) Unsqueeze(dim int) *Tensor[T, B] {
	result := t.backend.Unsqueeze(t.raw, dim)
	return New[T, B](result, t.backend)
}

// Squeeze removes a dimension of size 1 at the specified position.
//
// Panics if the dimension size is not 1.
// Supports negative dim indexing.
// This is a view operation (no data copy).
//
// Example:
//
//	x := tensor.Randn[float32](Shape{2, 1, 3}, backend)
//	y := x.Squeeze(1)  // Shape: [2, 3]
//	z := x.Squeeze(-2) // Shape: [2, 3]
func (t *Tensor[T, B]) Squeeze(dim int) *Tensor[T, B] {
	result := t.backend.Squeeze(t.raw, dim)
	return New[T, B](result, t.backend)
}

// Where selects elements from x or y based on condition.
//
// For each element:
//   - If condition is true, select from x
//   - If condition is false, select from y
//
// Supports broadcasting between condition, x, and y.
//
// Example:
//
//	cond := tensor.Full[bool](Shape{3}, true, backend)
//	x := tensor.Full[float32](Shape{3}, 1.0, backend)
//	y := tensor.Full[float32](Shape{3}, 0.0, backend)
//	result := tensor.Where(cond, x, y)  // [1.0, 1.0, 1.0]
func Where[T DType, B Backend](cond *Tensor[bool, B], x, y *Tensor[T, B]) *Tensor[T, B] {
	result := x.backend.Where(cond.raw, x.raw, y.raw)
	return New[T, B](result, x.backend)
}
