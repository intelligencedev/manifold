package cpu

import (
	"github.com/born-ml/born/internal/tensor"
)

// Float32 inplace operations

func addInplaceFloat32(a, b []float32) {
	if simdAddInplaceFloat32 != nil {
		simdAddInplaceFloat32(a, b)
		return
	}
	for i := range a {
		a[i] += b[i]
	}
}

func subInplaceFloat32(a, b []float32) {
	if simdSubInplaceFloat32 != nil {
		simdSubInplaceFloat32(a, b)
		return
	}
	for i := range a {
		a[i] -= b[i]
	}
}

func mulInplaceFloat32(a, b []float32) {
	if simdMulInplaceFloat32 != nil {
		simdMulInplaceFloat32(a, b)
		return
	}
	for i := range a {
		a[i] *= b[i]
	}
}

func divInplaceFloat32(a, b []float32) {
	if simdDivInplaceFloat32 != nil {
		simdDivInplaceFloat32(a, b)
		return
	}
	for i := range a {
		a[i] /= b[i]
	}
}

// Float32 vectorized operations

func addVectorizedFloat32(dst, a, b []float32) {
	if simdAddVectorizedFloat32 != nil {
		simdAddVectorizedFloat32(dst, a, b)
		return
	}
	for i := range a {
		dst[i] = a[i] + b[i]
	}
}

func subVectorizedFloat32(dst, a, b []float32) {
	if simdSubVectorizedFloat32 != nil {
		simdSubVectorizedFloat32(dst, a, b)
		return
	}
	for i := range a {
		dst[i] = a[i] - b[i]
	}
}

func mulVectorizedFloat32(dst, a, b []float32) {
	if simdMulVectorizedFloat32 != nil {
		simdMulVectorizedFloat32(dst, a, b)
		return
	}
	for i := range a {
		dst[i] = a[i] * b[i]
	}
}

func divVectorizedFloat32(dst, a, b []float32) {
	if simdDivVectorizedFloat32 != nil {
		simdDivVectorizedFloat32(dst, a, b)
		return
	}
	for i := range a {
		dst[i] = a[i] / b[i]
	}
}

// Float32 broadcasting operations

// The broadcasting ops below write a contiguous, row-major dst, so instead of
// recomputing each source index with computeFlatIndex (an integer division and
// modulo per output dimension, for every element) they advance the source flat
// indices incrementally: a mixed-radix odometer over the output coordinates
// updates aIdx and bIdx with a couple of adds per step and a carry only at
// dimension boundaries. The result is bit-identical to the division form (the
// same operands combined in the same order).

func addBroadcastFloat32(dst, a, b []float32, aShape, bShape, outShape tensor.Shape) {
	aStrides := computeBroadcastStridesForShape(aShape, outShape)
	bStrides := computeBroadcastStridesForShape(bShape, outShape)

	n := outShape.NumElements()
	ndim := len(outShape)
	coords := make([]int, ndim)
	aIdx, bIdx := 0, 0
	for i := 0; i < n; i++ {
		dst[i] = a[aIdx] + b[bIdx]
		for d := ndim - 1; d >= 0; d-- {
			coords[d]++
			aIdx += aStrides[d]
			bIdx += bStrides[d]
			if coords[d] < outShape[d] {
				break
			}
			coords[d] = 0
			aIdx -= outShape[d] * aStrides[d]
			bIdx -= outShape[d] * bStrides[d]
		}
	}
}

func subBroadcastFloat32(dst, a, b []float32, aShape, bShape, outShape tensor.Shape) {
	aStrides := computeBroadcastStridesForShape(aShape, outShape)
	bStrides := computeBroadcastStridesForShape(bShape, outShape)

	n := outShape.NumElements()
	ndim := len(outShape)
	coords := make([]int, ndim)
	aIdx, bIdx := 0, 0
	for i := 0; i < n; i++ {
		dst[i] = a[aIdx] - b[bIdx]
		for d := ndim - 1; d >= 0; d-- {
			coords[d]++
			aIdx += aStrides[d]
			bIdx += bStrides[d]
			if coords[d] < outShape[d] {
				break
			}
			coords[d] = 0
			aIdx -= outShape[d] * aStrides[d]
			bIdx -= outShape[d] * bStrides[d]
		}
	}
}

func mulBroadcastFloat32(dst, a, b []float32, aShape, bShape, outShape tensor.Shape) {
	// Fast paths for the structured broadcasts the model uses: one operand spans the
	// full output and the other either is constant over a trailing run (per-channel
	// scale, e.g. [N,C,H,W]*[N,C,1,1]) or is a dense vector tiled over the leading
	// dims (per-feature scale, e.g. [M,L]*[L], the STFT front end). Both avoid the
	// per-element index odometer. Multiply commutes, so try either operand as full.
	if mulBroadcastFullFloat32(dst, a, b, aShape, bShape, outShape) ||
		mulBroadcastFullFloat32(dst, b, a, bShape, aShape, outShape) {
		return
	}

	aStrides := computeBroadcastStridesForShape(aShape, outShape)
	bStrides := computeBroadcastStridesForShape(bShape, outShape)

	n := outShape.NumElements()
	ndim := len(outShape)
	coords := make([]int, ndim)
	aIdx, bIdx := 0, 0
	for i := 0; i < n; i++ {
		dst[i] = a[aIdx] * b[bIdx]
		for d := ndim - 1; d >= 0; d-- {
			coords[d]++
			aIdx += aStrides[d]
			bIdx += bStrides[d]
			if coords[d] < outShape[d] {
				break
			}
			coords[d] = 0
			aIdx -= outShape[d] * aStrides[d]
			bIdx -= outShape[d] * bStrides[d]
		}
	}
}

// mulBroadcastFullFloat32 computes dst = full * bc when `full` spans the whole
// output and bc broadcasts in one of two structured ways (trailing-run scale or
// dense leading-tiled vector). It returns false without writing otherwise, so the
// caller can try the commuted order or fall back to the general odometer.
func mulBroadcastFullFloat32(dst, full, bc []float32, fullShape, bcShape, outShape tensor.Shape) bool {
	if !fullShape.Equal(outShape) {
		return false
	}
	bcStrides := computeBroadcastStridesForShape(bcShape, outShape)
	// Iterate over the logical element count, not len(dst): the backing slice may
	// carry slack beyond the tensor's shape, and walking it would read/write past
	// the valid region.
	n := outShape.NumElements()

	// Case A: bc is constant over a trailing run (zeros at the tail of bcStrides),
	// so each run is a scalar multiply.
	if run := trailingBroadcastRun(bcStrides, outShape); run > 1 {
		outStrides := outShape.ComputeStrides()
		for base := 0; base < n; base += run {
			s := bc[computeFlatIndex(base, outStrides, bcStrides)]
			d := dst[base : base+run]
			f := full[base : base+run]
			for j := range d {
				d[j] = f[j] * s
			}
		}
		return true
	}

	// Case B: bc is a dense trailing tile, constant over the leading dims, so
	// dst[i] = full[i] * bc[i % tile] (each full row times the bc vector).
	if tile := leadingBroadcastTile(bcStrides, bcShape, outShape); tile > 0 {
		bct := bc[:tile]
		for base := 0; base < n; base += tile {
			d := dst[base : base+tile]
			f := full[base : base+tile]
			for j, bv := range bct {
				d[j] = f[j] * bv
			}
		}
		return true
	}
	return false
}

// trailingBroadcastRun returns the product of outShape's trailing dimensions over
// which bStrides is 0 (the operand is constant there), i.e. the largest contiguous
// run of the output that maps to a single source element. Returns 1 when the last
// dimension is not broadcast.
func trailingBroadcastRun(bStrides []int, outShape tensor.Shape) int {
	run := 1
	for d := len(outShape) - 1; d >= 0; d-- {
		if bStrides[d] != 0 {
			break
		}
		run *= outShape[d]
	}
	return run
}

// leadingBroadcastTile returns the dense trailing-tile length when bc is constant
// over the leading output dims and dense (its natural strides) over a contiguous
// trailing tile, so dst[i] = full[i] * bc[i % tile]. Returns 0 if the pattern does
// not hold.
func leadingBroadcastTile(bcStrides []int, bcShape, outShape tensor.Shape) int {
	ndim := len(outShape)
	split := 0
	for split < ndim && bcStrides[split] == 0 {
		split++
	}
	if split == 0 || split == ndim {
		return 0 // no leading broadcast, or fully broadcast (the trailing-run case)
	}
	outStrides := outShape.ComputeStrides()
	tile := 1
	for d := split; d < ndim; d++ {
		if bcStrides[d] != outStrides[d] {
			return 0
		}
		tile *= outShape[d]
	}
	if tile != bcShape.NumElements() {
		return 0
	}
	return tile
}

func divBroadcastFloat32(dst, a, b []float32, aShape, bShape, outShape tensor.Shape) {
	aStrides := computeBroadcastStridesForShape(aShape, outShape)
	bStrides := computeBroadcastStridesForShape(bShape, outShape)

	n := outShape.NumElements()
	ndim := len(outShape)
	coords := make([]int, ndim)
	aIdx, bIdx := 0, 0
	for i := 0; i < n; i++ {
		dst[i] = a[aIdx] / b[bIdx]
		for d := ndim - 1; d >= 0; d-- {
			coords[d]++
			aIdx += aStrides[d]
			bIdx += bStrides[d]
			if coords[d] < outShape[d] {
				break
			}
			coords[d] = 0
			aIdx -= outShape[d] * aStrides[d]
			bIdx -= outShape[d] * bStrides[d]
		}
	}
}

// Transpose float32.
func transposeFloat32(dst, src []float32, shape tensor.Shape, axes []int) {
	ndim := len(shape)
	srcStrides := shape.ComputeStrides()

	// Compute destination shape and strides
	dstShape := make(tensor.Shape, ndim)
	for i, ax := range axes {
		dstShape[i] = shape[ax]
	}
	dstStrides := dstShape.ComputeStrides()

	// Transpose data
	n := shape.NumElements()
	for i := 0; i < n; i++ {
		// Compute multi-dimensional coordinates in source
		coords := make([]int, ndim)
		idx := i
		for dim := 0; dim < ndim; dim++ {
			coords[dim] = idx / srcStrides[dim]
			idx %= srcStrides[dim]
		}

		// Permute coordinates according to axes
		permutedCoords := make([]int, ndim)
		for dstDim, srcDim := range axes {
			permutedCoords[dstDim] = coords[srcDim]
		}

		// Compute flat index in destination
		dstIdx := 0
		for dim := 0; dim < ndim; dim++ {
			dstIdx += permutedCoords[dim] * dstStrides[dim]
		}

		dst[dstIdx] = src[i]
	}
}
