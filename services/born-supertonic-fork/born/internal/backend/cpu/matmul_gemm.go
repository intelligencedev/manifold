package cpu

// gemmMinCols is the smallest n the SIMD GEMM kernel handles with at least one
// full 16-wide column tile. The kernel vectorizes over n (4-row tiles plus a
// 1-row remainder/GEMV path), so any m >= 1 is fine, but for n below this every
// column would fall to a naive scalar loop that loses to the cache-tiled scalar
// path, so the dispatch keeps such narrow shapes on scalar.
const gemmMinCols = 16

// gemmF32 is the optional vendored-SIMD GEMM fast path for float32:
//
//	C[m,n] = A[m,k] @ B[k,n]   (row-major, overwriting C)
//
// It is nil by default and wired in by an arch-specific init when the CPU supports
// the required instructions (AVX2+FMA on amd64, see matmul_gemm_amd64.go). When
// non-nil, matmulFloat32 dispatches large multiplications here instead of the
// scalar blocked path; when nil (other arches, older CPUs, or a GOEXPERIMENT=simd
// build where the archsimd micro-kernel owns dispatch) the scalar path is used
// unchanged.
var gemmF32 func(c, a, b []float32, m, k, n int)

// transposeF32 writes the [rows, cols] -> [cols, rows] transpose of src into dst:
// dst[c*rows+r] = src[r*cols+c]. Used to recast the conv im2col product
// out = kernel @ colBuf^T into the GEMM kernel's A @ B form.
func transposeF32(dst, src []float32, rows, cols int) {
	for r := 0; r < rows; r++ {
		base := r * cols
		for c := 0; c < cols; c++ {
			dst[c*rows+r] = src[base+c]
		}
	}
}
