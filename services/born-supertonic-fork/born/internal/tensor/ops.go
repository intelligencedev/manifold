package tensor

// Add performs element-wise addition with broadcasting.
//
// Example:
//
//	a := tensor.Ones[float32](Shape{3, 1}, backend)
//	b := tensor.Ones[float32](Shape{3, 5}, backend)
//	c := a.Add(b) // Shape: [3, 5] (broadcasted)
func (t *Tensor[T, B]) Add(other *Tensor[T, B]) *Tensor[T, B] {
	result := t.backend.Add(t.raw, other.raw)
	return New[T, B](result, t.backend)
}

// Sub performs element-wise subtraction with broadcasting.
func (t *Tensor[T, B]) Sub(other *Tensor[T, B]) *Tensor[T, B] {
	result := t.backend.Sub(t.raw, other.raw)
	return New[T, B](result, t.backend)
}

// Mul performs element-wise multiplication with broadcasting.
func (t *Tensor[T, B]) Mul(other *Tensor[T, B]) *Tensor[T, B] {
	result := t.backend.Mul(t.raw, other.raw)
	return New[T, B](result, t.backend)
}

// Div performs element-wise division with broadcasting.
func (t *Tensor[T, B]) Div(other *Tensor[T, B]) *Tensor[T, B] {
	result := t.backend.Div(t.raw, other.raw)
	return New[T, B](result, t.backend)
}

// MatMul performs matrix multiplication.
//
// Requirements:
//   - For 2D tensors: (M, K) @ (K, N) → (M, N)
//   - For batched: (B, M, K) @ (B, K, N) → (B, M, N)
//
// Example:
//
//	a := tensor.Randn[float32](Shape{3, 4}, backend)
//	b := tensor.Randn[float32](Shape{4, 5}, backend)
//	c := a.MatMul(b) // Shape: [3, 5]
func (t *Tensor[T, B]) MatMul(other *Tensor[T, B]) *Tensor[T, B] {
	result := t.backend.MatMul(t.raw, other.raw)
	return New[T, B](result, t.backend)
}

// BatchMatMul performs batched matrix multiplication.
//
// For 3D tensors: [B, M, K] @ [B, K, N] -> [B, M, N]
// For 4D tensors: [B, H, M, K] @ [B, H, K, N] -> [B, H, M, N]
//
// Example (Attention scores):
//
//	q := tensor.Randn[float32](tensor.Shape{2, 4, 64, 16}, backend) // [B, H, S, D]
//	k := tensor.Randn[float32](tensor.Shape{2, 4, 64, 16}, backend)
//	kT := k.Transpose(0, 1, 3, 2)
//	scores := q.BatchMatMul(kT) // [2, 4, 64, 64]
func (t *Tensor[T, B]) BatchMatMul(other *Tensor[T, B]) *Tensor[T, B] {
	batchMatMulBackend, ok := any(t.backend).(interface {
		BatchMatMul(*RawTensor, *RawTensor) *RawTensor
	})
	if !ok {
		panic("BatchMatMul: backend does not support BatchMatMul")
	}

	result := batchMatMulBackend.BatchMatMul(t.raw, other.raw)
	return &Tensor[T, B]{raw: result, backend: t.backend}
}

// Reshape returns a tensor with the same data but different shape.
// The new shape must have the same number of elements.
//
// Example:
//
//	t := tensor.Arange[int32](0, 12, backend) // Shape: [12]
//	reshaped := t.Reshape(3, 4)               // Shape: [3, 4]
func (t *Tensor[T, B]) Reshape(newShape ...int) *Tensor[T, B] {
	result := t.backend.Reshape(t.raw, Shape(newShape))
	return New[T, B](result, t.backend)
}

// Transpose transposes the tensor by permuting its dimensions.
//
// If axes is empty, reverses all dimensions (for 2D, this is standard transpose).
// Otherwise, axes specifies the permutation.
//
// Example:
//
//	t := tensor.Randn[float32](Shape{2, 3, 4}, backend)
//	transposed := t.Transpose(2, 0, 1) // Shape: [4, 2, 3]
func (t *Tensor[T, B]) Transpose(axes ...int) *Tensor[T, B] {
	result := t.backend.Transpose(t.raw, axes...)
	return New[T, B](result, t.backend)
}

// T is a shortcut for 2D transpose (swaps rows and columns).
// Panics if the tensor is not 2D.
//
// Example:
//
//	t := tensor.Randn[float32](Shape{3, 4}, backend)
//	transposed := t.T() // Shape: [4, 3]
func (t *Tensor[T, B]) T() *Tensor[T, B] {
	if len(t.Shape()) != 2 {
		panic("T() only works for 2D tensors")
	}
	return t.Transpose(1, 0)
}

// SumDim sums tensor elements along the specified dimension.
//
// Parameters:
//   - dim: dimension to reduce (supports negative indexing: -1 = last dim)
//   - keepDim: if true, keep the reduced dimension with size 1; if false, remove it
//
// Example:
//
//	x := tensor.Randn[float32](Shape{2, 3, 4}, backend)
//	y := x.SumDim(-1, true)   // shape: [2, 3, 1]
//	z := x.SumDim(-1, false)  // shape: [2, 3]
func (t *Tensor[T, B]) SumDim(dim int, keepDim bool) *Tensor[T, B] {
	result := t.backend.SumDim(t.raw, dim, keepDim)
	return New[T, B](result, t.backend)
}

// MeanDim computes the mean of tensor elements along the specified dimension.
//
// Parameters:
//   - dim: dimension to reduce (supports negative indexing: -1 = last dim)
//   - keepDim: if true, keep the reduced dimension with size 1; if false, remove it
//
// Example:
//
//	x := tensor.Randn[float32](Shape{2, 3, 4}, backend)
//	y := x.MeanDim(-1, true)   // shape: [2, 3, 1]
//	z := x.MeanDim(-1, false)  // shape: [2, 3]
func (t *Tensor[T, B]) MeanDim(dim int, keepDim bool) *Tensor[T, B] {
	result := t.backend.MeanDim(t.raw, dim, keepDim)
	return New[T, B](result, t.backend)
}
