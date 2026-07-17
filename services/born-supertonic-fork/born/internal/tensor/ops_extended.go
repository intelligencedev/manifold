package tensor

// Extended tensor operations - typed wrappers for backend operations.
//
// This file provides type-safe wrappers at the Tensor[T, B] level for operations
// that exist in the backend but weren't previously exposed to the public API.
//
// Design follows Burn (Rust) naming conventions:
//   - Scalar ops: MulScalar(T) - explicit naming
//   - Comparisons: Greater(other) + Gt(other) alias - full + short
//   - Type conversion: Int32() - result type name
//
// See: tmp/PUBLIC_API_REQUEST.md for HRM requirements
// Reference: reference/burn/crates/burn-tensor/src/tensor/api/numeric.rs

// ============================================================================
// Scalar Operations (Phase 1 - CRITICAL)
// ============================================================================

// MulScalar multiplies each element of the tensor by a scalar value.
//
// The scalar is broadcast to all elements of the tensor.
//
// Example:
//
//	x := tensor.Randn[float32](Shape{2, 3}, backend)
//	y := x.MulScalar(2.5)  // multiply all elements by 2.5
func (t *Tensor[T, B]) MulScalar(scalar T) *Tensor[T, B] {
	result := t.backend.MulScalar(t.raw, scalar)
	return New[T, B](result, t.backend)
}

// AddScalar adds a scalar value to each element of the tensor.
//
// The scalar is broadcast to all elements of the tensor.
//
// Example:
//
//	x := tensor.Randn[float32](Shape{2, 3}, backend)
//	y := x.AddScalar(1.0)  // add 1.0 to all elements
func (t *Tensor[T, B]) AddScalar(scalar T) *Tensor[T, B] {
	result := t.backend.AddScalar(t.raw, scalar)
	return New[T, B](result, t.backend)
}

// SubScalar subtracts a scalar value from each element of the tensor.
//
// The scalar is broadcast to all elements of the tensor.
//
// Example:
//
//	x := tensor.Randn[float32](Shape{2, 3}, backend)
//	y := x.SubScalar(0.5)  // subtract 0.5 from all elements
func (t *Tensor[T, B]) SubScalar(scalar T) *Tensor[T, B] {
	result := t.backend.SubScalar(t.raw, scalar)
	return New[T, B](result, t.backend)
}

// DivScalar divides each element of the tensor by a scalar value.
//
// The scalar is broadcast to all elements of the tensor.
//
// Example:
//
//	x := tensor.Randn[float32](Shape{2, 3}, backend)
//	y := x.DivScalar(2.0)  // divide all elements by 2.0
func (t *Tensor[T, B]) DivScalar(scalar T) *Tensor[T, B] {
	result := t.backend.DivScalar(t.raw, scalar)
	return New[T, B](result, t.backend)
}

// ============================================================================
// Math Operations (Phase 1 - CRITICAL)
// ============================================================================

// Exp computes the exponential (e^x) of each element.
//
// Example:
//
//	x := tensor.Randn[float32](Shape{2, 3}, backend)
//	y := x.Exp()  // e^x for each element
func (t *Tensor[T, B]) Exp() *Tensor[T, B] {
	result := t.backend.Exp(t.raw)
	return New[T, B](result, t.backend)
}

// Erf computes the error function of each element.
//
// Example:
//
//	x := tensor.Randn[float32](Shape{2, 3}, backend)
//	y := x.Erf()  // erf(x) for each element
func (t *Tensor[T, B]) Erf() *Tensor[T, B] {
	result := t.backend.Erf(t.raw)
	return New[T, B](result, t.backend)
}

// Abs computes the absolute value of each element.
//
// For signed integer dtypes, this uses two's-complement wraparound semantics:
// abs(MinInt) == MinInt. This matches Burn, NumPy, and PyTorch behavior.
//
// Example:
//
//	x := tensor.Randn[float32](Shape{2, 3}, backend)
//	y := x.Abs()  // |x| for each element
func (t *Tensor[T, B]) Abs() *Tensor[T, B] {
	result := t.backend.Abs(t.raw)
	return New[T, B](result, t.backend)
}

// Log computes the natural logarithm (ln(x)) of each element.
//
// Example:
//
//	x := tensor.Randn[float32](Shape{2, 3}, backend)
//	y := x.Log()  // ln(x) for each element
func (t *Tensor[T, B]) Log() *Tensor[T, B] {
	result := t.backend.Log(t.raw)
	return New[T, B](result, t.backend)
}

// Sqrt computes the square root of each element.
//
// Example:
//
//	x := tensor.Randn[float32](Shape{2, 3}, backend)
//	y := x.Sqrt()  // sqrt(x) for each element
func (t *Tensor[T, B]) Sqrt() *Tensor[T, B] {
	result := t.backend.Sqrt(t.raw)
	return New[T, B](result, t.backend)
}

// Rsqrt computes the reciprocal square root (1/sqrt(x)) of each element.
//
// This is often faster than computing Sqrt and then taking the reciprocal.
//
// Example:
//
//	x := tensor.Randn[float32](Shape{2, 3}, backend)
//	y := x.Rsqrt()  // 1/sqrt(x) for each element
func (t *Tensor[T, B]) Rsqrt() *Tensor[T, B] {
	result := t.backend.Rsqrt(t.raw)
	return New[T, B](result, t.backend)
}

// Cos computes the cosine of each element (input in radians).
//
// Example:
//
//	x := tensor.Arange[float32](0, 10, backend)
//	y := x.Cos()  // cos(x) for each element
func (t *Tensor[T, B]) Cos() *Tensor[T, B] {
	result := t.backend.Cos(t.raw)
	return New[T, B](result, t.backend)
}

// Sin computes the sine of each element (input in radians).
//
// Example:
//
//	x := tensor.Arange[float32](0, 10, backend)
//	y := x.Sin()  // sin(x) for each element
func (t *Tensor[T, B]) Sin() *Tensor[T, B] {
	result := t.backend.Sin(t.raw)
	return New[T, B](result, t.backend)
}

// Sign computes the sign of each element.
//
// Returns -1 for negative, 0 for zero, +1 for positive. For unsigned integer
// dtypes (uint8), only 0 and +1 are produced since negative values cannot be
// represented.
//
// Example:
//
//	x := tensor.Arange[float32](0, 10, backend)
//	y := x.Sign()  // sign(x) for each element
func (t *Tensor[T, B]) Sign() *Tensor[T, B] {
	result := t.backend.Sign(t.raw)
	return New[T, B](result, t.backend)
}

// ============================================================================
// Activation Functions (Phase 1 - CRITICAL)
// ============================================================================

// Softmax computes the softmax function along the specified dimension.
//
// Softmax(x_i) = exp(x_i) / sum(exp(x_j)) for all j in dimension.
// Supports negative dimension indexing (-1 = last dimension).
//
// Example:
//
//	logits := tensor.Randn[float32](Shape{2, 10}, backend)
//	probs := logits.Softmax(1)  // softmax along last dimension
func (t *Tensor[T, B]) Softmax(dim int) *Tensor[T, B] {
	result := t.backend.Softmax(t.raw, dim)
	return New[T, B](result, t.backend)
}

// ============================================================================
// Comparison Operations (Phase 2 - CRITICAL)
//
// All comparison operations return Tensor[bool, B].
// ============================================================================

// Greater returns a boolean tensor where each element is true if the
// corresponding element in this tensor is greater than the corresponding
// element in other.
//
// Supports broadcasting between tensors of different shapes.
//
// Example:
//
//	a := tensor.Arange[float32](0, 5, backend)
//	b := tensor.Full[float32](Shape{5}, 2.0, backend)
//	mask := a.Greater(b)  // [false, false, false, true, true]
func (t *Tensor[T, B]) Greater(other *Tensor[T, B]) *Tensor[bool, B] {
	result := t.backend.Greater(t.raw, other.raw)
	return New[bool, B](result, t.backend)
}

// Gt is a short alias for Greater.
//
// Example:
//
//	mask := a.Gt(b)  // same as a.Greater(b)
func (t *Tensor[T, B]) Gt(other *Tensor[T, B]) *Tensor[bool, B] {
	return t.Greater(other)
}

// Lower returns a boolean tensor where each element is true if the
// corresponding element in this tensor is less than the corresponding
// element in other.
//
// Supports broadcasting between tensors of different shapes.
//
// Example:
//
//	a := tensor.Arange[float32](0, 5, backend)
//	b := tensor.Full[float32](Shape{5}, 2.0, backend)
//	mask := a.Lower(b)  // [true, true, false, false, false]
func (t *Tensor[T, B]) Lower(other *Tensor[T, B]) *Tensor[bool, B] {
	result := t.backend.Lower(t.raw, other.raw)
	return New[bool, B](result, t.backend)
}

// Lt is a short alias for Lower.
//
// Example:
//
//	mask := a.Lt(b)  // same as a.Lower(b)
func (t *Tensor[T, B]) Lt(other *Tensor[T, B]) *Tensor[bool, B] {
	return t.Lower(other)
}

// GreaterEqual returns a boolean tensor where each element is true if the
// corresponding element in this tensor is greater than or equal to the
// corresponding element in other.
//
// Supports broadcasting between tensors of different shapes.
//
// Example:
//
//	a := tensor.Arange[float32](0, 5, backend)
//	b := tensor.Full[float32](Shape{5}, 2.0, backend)
//	mask := a.GreaterEqual(b)  // [false, false, true, true, true]
func (t *Tensor[T, B]) GreaterEqual(other *Tensor[T, B]) *Tensor[bool, B] {
	result := t.backend.GreaterEqual(t.raw, other.raw)
	return New[bool, B](result, t.backend)
}

// Ge is a short alias for GreaterEqual.
//
// Example:
//
//	mask := a.Ge(b)  // same as a.GreaterEqual(b)
func (t *Tensor[T, B]) Ge(other *Tensor[T, B]) *Tensor[bool, B] {
	return t.GreaterEqual(other)
}

// LowerEqual returns a boolean tensor where each element is true if the
// corresponding element in this tensor is less than or equal to the
// corresponding element in other.
//
// Supports broadcasting between tensors of different shapes.
//
// Example:
//
//	a := tensor.Arange[float32](0, 5, backend)
//	b := tensor.Full[float32](Shape{5}, 2.0, backend)
//	mask := a.LowerEqual(b)  // [true, true, true, false, false]
func (t *Tensor[T, B]) LowerEqual(other *Tensor[T, B]) *Tensor[bool, B] {
	result := t.backend.LowerEqual(t.raw, other.raw)
	return New[bool, B](result, t.backend)
}

// Le is a short alias for LowerEqual.
//
// Example:
//
//	mask := a.Le(b)  // same as a.LowerEqual(b)
func (t *Tensor[T, B]) Le(other *Tensor[T, B]) *Tensor[bool, B] {
	return t.LowerEqual(other)
}

// Equal returns a boolean tensor where each element is true if the
// corresponding elements in this tensor and other are equal.
//
// Supports broadcasting between tensors of different shapes.
//
// Example:
//
//	a := tensor.Arange[float32](0, 5, backend)
//	b := tensor.Full[float32](Shape{5}, 2.0, backend)
//	mask := a.Equal(b)  // [false, false, true, false, false]
func (t *Tensor[T, B]) Equal(other *Tensor[T, B]) *Tensor[bool, B] {
	result := t.backend.Equal(t.raw, other.raw)
	return New[bool, B](result, t.backend)
}

// Eq is a short alias for Equal.
//
// Example:
//
//	mask := a.Eq(b)  // same as a.Equal(b)
func (t *Tensor[T, B]) Eq(other *Tensor[T, B]) *Tensor[bool, B] {
	return t.Equal(other)
}

// NotEqual returns a boolean tensor where each element is true if the
// corresponding elements in this tensor and other are not equal.
//
// Supports broadcasting between tensors of different shapes.
//
// Example:
//
//	a := tensor.Arange[float32](0, 5, backend)
//	b := tensor.Full[float32](Shape{5}, 2.0, backend)
//	mask := a.NotEqual(b)  // [true, true, false, true, true]
func (t *Tensor[T, B]) NotEqual(other *Tensor[T, B]) *Tensor[bool, B] {
	result := t.backend.NotEqual(t.raw, other.raw)
	return New[bool, B](result, t.backend)
}

// Ne is a short alias for NotEqual.
//
// Example:
//
//	mask := a.Ne(b)  // same as a.NotEqual(b)
func (t *Tensor[T, B]) Ne(other *Tensor[T, B]) *Tensor[bool, B] {
	return t.NotEqual(other)
}

// ============================================================================
// Boolean Operations (Phase 2 - CRITICAL)
//
// These operations work on Tensor[bool, B] and return Tensor[bool, B].
// ============================================================================

// Or computes element-wise logical OR between two boolean tensors.
//
// Supports broadcasting between tensors of different shapes.
//
// Example:
//
//	a := tensor.Full[bool](Shape{3}, true, backend)
//	b := tensor.Full[bool](Shape{3}, false, backend)
//	c := a.Or(b)  // [true, true, true]
func (t *Tensor[bool, B]) Or(other *Tensor[bool, B]) *Tensor[bool, B] {
	result := t.backend.Or(t.raw, other.raw)
	return New[bool, B](result, t.backend)
}

// And computes element-wise logical AND between two boolean tensors.
//
// Supports broadcasting between tensors of different shapes.
//
// Example:
//
//	a := tensor.Full[bool](Shape{3}, true, backend)
//	b := tensor.Full[bool](Shape{3}, false, backend)
//	c := a.And(b)  // [false, false, false]
func (t *Tensor[bool, B]) And(other *Tensor[bool, B]) *Tensor[bool, B] {
	result := t.backend.And(t.raw, other.raw)
	return New[bool, B](result, t.backend)
}

// Not computes element-wise logical NOT of a boolean tensor.
//
// Example:
//
//	a := tensor.Full[bool](Shape{3}, true, backend)
//	b := a.Not()  // [false, false, false]
func (t *Tensor[bool, B]) Not() *Tensor[bool, B] {
	result := t.backend.Not(t.raw)
	return New[bool, B](result, t.backend)
}

// ============================================================================
// Indexing Operations (Phase 2 - CRITICAL)
// ============================================================================

// Gather selects elements from the tensor along a dimension using an index tensor.
//
// For each element in the index tensor, Gather selects the corresponding element
// from the input tensor along the specified dimension.
//
// Example:
//
//	x := tensor.Randn[float32](Shape{3, 5}, backend)
//	indices := tensor.FromSlice([]int32{0, 2, 4}, Shape{3}, backend)
//	y := x.Gather(1, indices)  // select columns 0, 2, 4 for each row
func (t *Tensor[T, B]) Gather(dim int, index *Tensor[int32, B]) *Tensor[T, B] {
	result := t.backend.Gather(t.raw, dim, index.raw)
	return New[T, B](result, t.backend)
}

// Embedding performs embedding lookup using the tensor as the weight matrix.
//
// The tensor t should have shape [numEmbeddings, embeddingDim].
// The indices tensor contains integer indices to look up.
// Returns embeddings with shape [...indices.shape, embeddingDim].
//
// This operation is differentiable - gradients flow back to the weight tensor
// via scatter-add (same indices accumulate gradients).
//
// Example:
//
//	weight := tensor.Randn[float32](Shape{1000, 256}, backend)  // vocab=1000, dim=256
//	indices := tensor.FromSlice([]int32{0, 5, 3}, Shape{3}, backend)
//	embeddings := weight.Embedding(indices)  // shape: [3, 256]
func (t *Tensor[T, B]) Embedding(indices *Tensor[int32, B]) *Tensor[T, B] {
	result := t.backend.Embedding(t.raw, indices.raw)
	return New[T, B](result, t.backend)
}

// ============================================================================
// Reduction Operations (Phase 3 - IMPORTANT)
// ============================================================================

// Sum computes the sum of all elements in the tensor, returning a scalar.
//
// The result is a tensor with shape [] (scalar).
//
// Example:
//
//	x := tensor.Randn[float32](Shape{3, 4}, backend)
//	total := x.Sum()  // sum of all 12 elements
func (t *Tensor[T, B]) Sum() *Tensor[T, B] {
	result := t.backend.Sum(t.raw)
	return New[T, B](result, t.backend)
}

// Argmax returns the index of the maximum value along the specified dimension.
//
// Returns a tensor of type int32 with the same shape as the input except
// the specified dimension is removed.
//
// Supports negative dimension indexing (-1 = last dimension).
//
// Example:
//
//	x := tensor.Randn[float32](Shape{3, 4}, backend)
//	indices := x.Argmax(1)  // Shape: [3], index of max in each row
func (t *Tensor[T, B]) Argmax(dim int) *Tensor[int32, B] {
	result := t.backend.Argmax(t.raw, dim)
	return New[int32, B](result, t.backend)
}

// ============================================================================
// Shape Operations (Phase 3 - IMPORTANT)
// ============================================================================

// Expand broadcasts the tensor to a new shape.
//
// The new shape must be compatible with the current shape according to
// NumPy broadcasting rules. Dimensions of size 1 can be broadcast to any size.
//
// Example:
//
//	x := tensor.Randn[float32](Shape{1, 3}, backend)
//	y := x.Expand(Shape{5, 3})  // broadcast to [5, 3]
func (t *Tensor[T, B]) Expand(shape Shape) *Tensor[T, B] {
	result := t.backend.Expand(t.raw, shape)
	return New[T, B](result, t.backend)
}

// ============================================================================
// Type Conversion Operations (Phase 3 - IMPORTANT)
// ============================================================================

// Int32 casts the tensor to int32 dtype.
//
// Example:
//
//	x := tensor.Randn[float32](Shape{3, 4}, backend)
//	y := x.Int32()  // Tensor[int32, B]
func (t *Tensor[T, B]) Int32() *Tensor[int32, B] {
	result := t.backend.Cast(t.raw, Int32)
	return New[int32, B](result, t.backend)
}

// Float32 casts the tensor to float32 dtype.
//
// Example:
//
//	x := tensor.Arange[int32](0, 10, backend)
//	y := x.Float32()  // Tensor[float32, B]
func (t *Tensor[T, B]) Float32() *Tensor[float32, B] {
	result := t.backend.Cast(t.raw, Float32)
	return New[float32, B](result, t.backend)
}

// Float64 casts the tensor to float64 dtype.
//
// Example:
//
//	x := tensor.Randn[float32](Shape{3, 4}, backend)
//	y := x.Float64()  // Tensor[float64, B]
func (t *Tensor[T, B]) Float64() *Tensor[float64, B] {
	result := t.backend.Cast(t.raw, Float64)
	return New[float64, B](result, t.backend)
}

// Int64 casts the tensor to int64 dtype.
//
// Example:
//
//	x := tensor.Arange[int32](0, 10, backend)
//	y := x.Int64()  // Tensor[int64, B]
func (t *Tensor[T, B]) Int64() *Tensor[int64, B] {
	result := t.backend.Cast(t.raw, Int64)
	return New[int64, B](result, t.backend)
}
