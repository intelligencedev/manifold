package tensor

import "fmt"

// Shape represents the dimensions of a tensor.
type Shape []int

// NumElements returns the total number of elements in the tensor.
func (s Shape) NumElements() int {
	if len(s) == 0 {
		return 1 // Scalar has 1 element
	}
	n := 1
	for _, dim := range s {
		n *= dim
	}
	return n
}

// Validate checks if the shape is valid (all dimensions >= 0). Zero-sized
// dimensions are permitted: ONNX allows empty tensors (e.g. ConstantOfShape([0])
// as a Concat seed in attention), and they must flow through the graph.
func (s Shape) Validate() error {
	for i, dim := range s {
		if dim < 0 {
			return fmt.Errorf("invalid dimension at index %d: %d (must be >= 0)", i, dim)
		}
	}
	return nil
}

// Equal checks if two shapes are equal.
func (s Shape) Equal(other Shape) bool {
	if len(s) != len(other) {
		return false
	}
	for i := range s {
		if s[i] != other[i] { //nolint:gosec // G602: false positive, len equality checked above
			return false
		}
	}
	return true
}

// Clone returns a copy of the shape.
func (s Shape) Clone() Shape {
	clone := make(Shape, len(s))
	copy(clone, s)
	return clone
}

// ComputeStrides calculates row-major strides for the shape.
// Strides define memory layout: stride[i] = product of all dimensions after i.
func (s Shape) ComputeStrides() []int {
	strides := make([]int, len(s))
	if len(s) == 0 {
		return strides
	}

	strides[len(s)-1] = 1
	for i := len(s) - 2; i >= 0; i-- {
		strides[i] = strides[i+1] * s[i+1]
	}
	return strides
}

// BroadcastShapes implements NumPy-style broadcasting rules.
//
// Rules:
// 1. Compare shapes element-wise from right to left
// 2. Dimensions are compatible if:
//   - They are equal, OR
//   - One of them is 1
//
// 3. Missing dimensions are treated as 1
//
// Returns the broadcasted shape, a flag indicating if broadcasting is needed, and an error if incompatible.
//
// Examples:
//
//	(3, 1) + (3, 5) → (3, 5), true, nil
//	(1, 5) + (3, 5) → (3, 5), true, nil
//	(3, 5) + (3, 5) → (3, 5), false, nil
//	(3, 4) + (3, 5) → nil, false, Error
func BroadcastShapes(a, b Shape) (Shape, bool, error) {
	maxLen := maxInt(len(a), len(b))
	result := make(Shape, maxLen)
	needsBroadcast := false

	for i := 0; i < maxLen; i++ {
		aIdx := len(a) - 1 - i
		bIdx := len(b) - 1 - i

		aDim := 1
		if aIdx >= 0 {
			aDim = a[aIdx]
		}

		bDim := 1
		if bIdx >= 0 {
			bDim = b[bIdx]
		}

		switch {
		case aDim == bDim:
			result[maxLen-1-i] = aDim
		case aDim == 1:
			result[maxLen-1-i] = bDim
			needsBroadcast = true
		case bDim == 1:
			result[maxLen-1-i] = aDim
			needsBroadcast = true
		default:
			return nil, false, fmt.Errorf("shapes not compatible for broadcasting: %v vs %v (dimension %d: %d vs %d)",
				a, b, maxLen-1-i, aDim, bDim)
		}
	}

	return result, needsBroadcast, nil
}

// BroadcastShapesMatMul implements NumPy-style broadcasting rules for matrix multiplication.
//
// Rules:
// 1. Both shapes must have at least 2 dimensions (matrix dimensions)
// 2. The inner dimensions must match: a[last] == b[second-to-last] (k1 == k2)
// 3. The batch dimensions (all dimensions except the last two) are broadcasted using standard broadcasting rules:
//   - Compare shapes element-wise from right to left
//   - Dimensions are compatible if they are equal OR one of them is 1
//   - Missing dimensions are treated as 1
//
// Returns the broadcasted shape, a flag indicating if broadcasting is needed, and an error if incompatible.
//
// Examples:
//
//	(3, 4) + (4, 5) → (3, 5), false, nil
//	(2, 3, 4) + (2, 4, 5) → (2, 3, 5), false, nil
//	(2, 1, 3, 4) + (5, 4, 2) → (2, 5, 3, 2), true, nil
//	(3, 4) + (4) → nil, false, Error
//	(3, 1, 4) + (5, 4, 2) → nil, false, Error
//	(3, 5) + (4, 5) → nil, false, Error
func BroadcastShapesMatMul(a, b Shape) (Shape, bool, error) {
	adim := len(a)
	bdim := len(b)
	if adim < 2 || bdim < 2 {
		return nil, false, fmt.Errorf("shapes must have at least 2 dims for multiplication, got: %v and %v", adim, bdim)
	}

	m := a[adim-2]
	k1 := a[adim-1]
	n := b[bdim-1]
	k2 := b[bdim-2]

	if k1 != k2 {
		return nil, false, fmt.Errorf("inner dimension mismatch: %d vs %d", k1, k2)
	}

	batchShape, needsBroadcast, err := BroadcastShapes(a[:adim-2], b[:bdim-2])
	if err != nil {
		return nil, false, err
	}

	batchShape = append(batchShape, m, n)
	return batchShape, needsBroadcast, nil
}

// maxInt returns the maximum of two integers.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
