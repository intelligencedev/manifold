package cpu

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/born-ml/born/internal/tensor"
)

// batchParallelThreshold is the minimum batch size before parallelising across goroutines.
// Below this threshold the goroutine-spawn overhead exceeds the compute savings.
// Empirical value from GoMLX; revisit with project benchmarks.
const batchParallelThreshold = 4

// bmmParallelWorkThreshold is the minimum total multiply-accumulate count before
// the row-splitting parallel path is worth its goroutine overhead.
const bmmParallelWorkThreshold = 1 << 15

// bmmMinRowsPerChunk keeps row chunks large enough that each job amortizes its
// spawn cost and matmulFloat32's blocking still has room to work.
const bmmMinRowsPerChunk = 16

type bmmJob struct{ batch, r0, r1 int }

// bmmParallelF32 runs a batched float32 matmul as (batch × row-chunk) jobs on a
// worker pool. Small batch counts (e.g. 4 attention heads) previously ran fully
// serial because parallelism was only ever across whole batches; splitting rows
// lets a batch=4 attention matmul use every core. Row slices of row-major C/A
// are themselves valid matmuls, and each job's C rows are disjoint, so this is
// race-free. aOff/bOff map a batch index to its (possibly broadcast) operand
// offset. Returns false when the work is too small to beat the serial loop.
func bmmParallelF32(c, a, b []float32, totalBatches, m, k, n int, aOff, bOff func(batch int) int) bool {
	if totalBatches*m*k*n < bmmParallelWorkThreshold {
		return false
	}
	workers := runtime.NumCPU()
	rowChunks := (workers*2 + totalBatches - 1) / totalBatches
	if maxChunks := (m + bmmMinRowsPerChunk - 1) / bmmMinRowsPerChunk; rowChunks > maxChunks {
		rowChunks = maxChunks
	}
	if rowChunks < 1 {
		rowChunks = 1
	}
	chunk := (m + rowChunks - 1) / rowChunks
	jobs := make([]bmmJob, 0, totalBatches*rowChunks)
	for bi := 0; bi < totalBatches; bi++ {
		for r := 0; r < m; r += chunk {
			jobs = append(jobs, bmmJob{bi, r, min(r+chunk, m)})
		}
	}
	if len(jobs) <= 1 {
		return false
	}
	if workers > len(jobs) {
		workers = len(jobs)
	}
	matrixSizeC := m * n
	var cursor atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				idx := int(cursor.Add(1)) - 1
				if idx >= len(jobs) {
					return
				}
				j := jobs[idx]
				rows := j.r1 - j.r0
				off := j.batch*matrixSizeC + j.r0*n
				matmulFloat32(c[off:off+rows*n], a[aOff(j.batch)+j.r0*k:], b[bOff(j.batch):], rows, k, n)
			}
		}()
	}
	wg.Wait()
	return true
}

// BatchMatMul performs batched matrix multiplication with numpy-style broadcasting.
// Supports tensors with 2 or more dimensions. At least one input must be 3D or higher.
//
// The last two dimensions are treated as matrix dimensions: A: (..., M, K), B: (..., K, N)
// Output: (..., M, N). The inner dimension K must match exactly.
// Batch dimensions are broadcast following numpy rules (dimensions compatible if equal or one is 1).
//
// Examples:
//
//	[B, M, K] @ [B, K, N]       -> [B, M, N]      (no broadcast)
//	[1, M, K] @ [B, K, N]       -> [B, M, N]      (singleton broadcast)
//	[M, K]    @ [B, K, N]       -> [B, M, N]      (2D broadcast to batch)
//	[A, 1, M, K] @ [1, C, K, N] -> [A, C, M, N]  (multi-dim broadcast)
func (cpu *CPUBackend) BatchMatMul(a, b *tensor.RawTensor) *tensor.RawTensor {
	aShape := a.Shape()
	bShape := b.Shape()

	if len(aShape) < 3 && len(bShape) < 3 {
		panic(fmt.Sprintf("BatchMatMul: at least one of the inputs must be > 2D, got %dD and %dD", len(aShape), len(bShape)))
	}

	outShape, needsBroadcast, err := tensor.BroadcastShapesMatMul(aShape, bShape)
	if err != nil {
		panic(fmt.Sprintf("BatchMatMul: %v", err))
	}

	result, err := tensor.NewRaw(outShape, a.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("BatchMatMul: failed to create result tensor: %v", err))
	}

	m := aShape[len(aShape)-2]
	k := aShape[len(aShape)-1]
	n := bShape[len(bShape)-1]

	if !needsBroadcast {
		batchSize := 1
		for i := 0; i < len(outShape)-2; i++ {
			batchSize *= outShape[i]
		}
		batchMatmul(result, a, b, batchSize, m, k, n)
	} else {
		aBatchShape := aShape[:len(aShape)-2]
		bBatchShape := bShape[:len(bShape)-2]
		outBatchShape := outShape[:len(outShape)-2]
		batchMatmulBroadcast(result, a, b, outBatchShape, aBatchShape, bBatchShape, m, k, n)
	}

	return result
}

// batchMatmul performs batched matrix multiplication.
func batchMatmul(result, a, b *tensor.RawTensor, batchSize, m, k, n int) {
	switch a.DType() {
	case tensor.Float32:
		batchMatmulFloat32(result.AsFloat32(), a.AsFloat32(), b.AsFloat32(), batchSize, m, k, n)
	case tensor.Float64:
		batchMatmulFloat64(result.AsFloat64(), a.AsFloat64(), b.AsFloat64(), batchSize, m, k, n)
	default:
		panic(fmt.Sprintf("BatchMatMul: unsupported dtype %s", a.DType()))
	}
}

// batchMatmulBroadcast performs batched matrix multiplication with broadcast.
func batchMatmulBroadcast(
	result, a, b *tensor.RawTensor,
	outBatchShape, aBatchShape, bBatchShape tensor.Shape,
	m, k, n int,
) {
	switch a.DType() {
	case tensor.Float32:
		batchMatmulBroadcastFloat32(
			result.AsFloat32(), a.AsFloat32(), b.AsFloat32(),
			outBatchShape, aBatchShape, bBatchShape, m, k, n,
		)
	case tensor.Float64:
		batchMatmulBroadcastFloat64(
			result.AsFloat64(), a.AsFloat64(), b.AsFloat64(),
			outBatchShape, aBatchShape, bBatchShape, m, k, n,
		)
	default:
		panic(fmt.Sprintf("BatchMatMul: unsupported dtype %s", a.DType()))
	}
}

// batchMatmulFloat32 performs batched matrix multiplication for float32.
// Batches are independent, so large batch counts are parallelised across CPU cores.
//
//nolint:dupl // Intentional duplication for float32/float64; type-specific matmul call precludes generics without boxing.
func batchMatmulFloat32(c, a, b []float32, batchSize, m, k, n int) {
	matrixSizeA := m * k
	matrixSizeB := k * n
	matrixSizeC := m * n

	if bmmParallelF32(c, a, b, batchSize, m, k, n,
		func(batch int) int { return batch * matrixSizeA },
		func(batch int) int { return batch * matrixSizeB }) {
		return
	}

	if batchSize <= batchParallelThreshold {
		for batch := range batchSize {
			off := batch * matrixSizeC
			matmulFloat32(c[off:off+matrixSizeC], a[batch*matrixSizeA:], b[batch*matrixSizeB:], m, k, n)
		}
		return
	}

	numWorkers := min(runtime.NumCPU(), batchSize)
	batchesPerWorker := (batchSize + numWorkers - 1) / numWorkers

	var wg sync.WaitGroup
	for w := range numWorkers {
		startBatch := w * batchesPerWorker
		if startBatch >= batchSize {
			break
		}
		endBatch := min(startBatch+batchesPerWorker, batchSize)
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			for batch := start; batch < end; batch++ {
				off := batch * matrixSizeC
				// Cap the slice to exactly m*n elements so matmulFloat32's
				// zero-initialisation loop does not overwrite adjacent batch slots.
				matmulFloat32(c[off:off+matrixSizeC], a[batch*matrixSizeA:], b[batch*matrixSizeB:], m, k, n)
			}
		}(startBatch, endBatch)
	}
	wg.Wait()
}

// batchMatmulFloat64 performs batched matrix multiplication for float64.
// Batches are independent, so large batch counts are parallelised across CPU cores.
//
//nolint:dupl // Intentional duplication for float32/float64; type-specific matmul call precludes generics without boxing.
func batchMatmulFloat64(c, a, b []float64, batchSize, m, k, n int) {
	matrixSizeA := m * k
	matrixSizeB := k * n
	matrixSizeC := m * n

	if batchSize <= batchParallelThreshold {
		for batch := range batchSize {
			off := batch * matrixSizeC
			matmulFloat64(c[off:off+matrixSizeC], a[batch*matrixSizeA:], b[batch*matrixSizeB:], m, k, n)
		}
		return
	}

	numWorkers := min(runtime.NumCPU(), batchSize)
	batchesPerWorker := (batchSize + numWorkers - 1) / numWorkers

	var wg sync.WaitGroup
	for w := range numWorkers {
		startBatch := w * batchesPerWorker
		if startBatch >= batchSize {
			break
		}
		endBatch := min(startBatch+batchesPerWorker, batchSize)
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			for batch := start; batch < end; batch++ {
				off := batch * matrixSizeC
				// Cap the slice to exactly m*n elements so matmulFloat64's
				// zero-initialisation loop does not overwrite adjacent batch slots.
				matmulFloat64(c[off:off+matrixSizeC], a[batch*matrixSizeA:], b[batch*matrixSizeB:], m, k, n)
			}
		}(startBatch, endBatch)
	}
	wg.Wait()
}

// batchMatmulBroadcastFloat32 performs batched matrix multiplication for float32 with broadcast.
// Batch indices are independent, so large batch counts are parallelised across CPU cores.
// Stride slices are read-only, making concurrent access safe without locks.
//
//nolint:dupl // Intentional duplication for float32/float64; type-specific matmul call precludes generics without boxing.
func batchMatmulBroadcastFloat32(
	c, a, b []float32,
	outBatchShape, aBatchShape, bBatchShape tensor.Shape,
	m, k, n int,
) {
	outBatchStrides := outBatchShape.ComputeStrides()
	aBroadcastStrides := computeBroadcastStridesForShape(aBatchShape, outBatchShape)
	bBroadcastStrides := computeBroadcastStridesForShape(bBatchShape, outBatchShape)

	matrixSizeA := m * k
	matrixSizeB := k * n
	matrixSizeC := m * n

	totalBatches := outBatchShape.NumElements()
	if bmmParallelF32(c, a, b, totalBatches, m, k, n,
		func(batch int) int {
			return computeFlatIndex(batch, outBatchStrides, aBroadcastStrides) * matrixSizeA
		},
		func(batch int) int {
			return computeFlatIndex(batch, outBatchStrides, bBroadcastStrides) * matrixSizeB
		}) {
		return
	}
	if totalBatches <= batchParallelThreshold {
		for batchIdx := range totalBatches {
			aBatchFlat := computeFlatIndex(batchIdx, outBatchStrides, aBroadcastStrides)
			bBatchFlat := computeFlatIndex(batchIdx, outBatchStrides, bBroadcastStrides)
			off := batchIdx * matrixSizeC
			matmulFloat32(c[off:off+matrixSizeC], a[aBatchFlat*matrixSizeA:], b[bBatchFlat*matrixSizeB:], m, k, n)
		}
		return
	}

	numWorkers := min(runtime.NumCPU(), totalBatches)
	batchesPerWorker := (totalBatches + numWorkers - 1) / numWorkers

	var wg sync.WaitGroup
	for w := range numWorkers {
		startBatch := w * batchesPerWorker
		if startBatch >= totalBatches {
			break
		}
		endBatch := min(startBatch+batchesPerWorker, totalBatches)
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			for batchIdx := start; batchIdx < end; batchIdx++ {
				aBatchFlat := computeFlatIndex(batchIdx, outBatchStrides, aBroadcastStrides)
				bBatchFlat := computeFlatIndex(batchIdx, outBatchStrides, bBroadcastStrides)
				off := batchIdx * matrixSizeC
				// Cap the slice to exactly m*n elements so matmulFloat32's
				// zero-initialisation loop does not overwrite adjacent batch slots.
				matmulFloat32(c[off:off+matrixSizeC], a[aBatchFlat*matrixSizeA:], b[bBatchFlat*matrixSizeB:], m, k, n)
			}
		}(startBatch, endBatch)
	}
	wg.Wait()
}

// batchMatmulBroadcastFloat64 performs batched matrix multiplication for float64 with broadcast.
// Batch indices are independent, so large batch counts are parallelised across CPU cores.
// Stride slices are read-only, making concurrent access safe without locks.
//
//nolint:dupl // Intentional duplication for float32/float64; type-specific matmul call precludes generics without boxing.
func batchMatmulBroadcastFloat64(
	c, a, b []float64,
	outBatchShape, aBatchShape, bBatchShape tensor.Shape,
	m, k, n int,
) {
	outBatchStrides := outBatchShape.ComputeStrides()
	aBroadcastStrides := computeBroadcastStridesForShape(aBatchShape, outBatchShape)
	bBroadcastStrides := computeBroadcastStridesForShape(bBatchShape, outBatchShape)

	matrixSizeA := m * k
	matrixSizeB := k * n
	matrixSizeC := m * n

	totalBatches := outBatchShape.NumElements()
	if totalBatches <= batchParallelThreshold {
		for batchIdx := range totalBatches {
			aBatchFlat := computeFlatIndex(batchIdx, outBatchStrides, aBroadcastStrides)
			bBatchFlat := computeFlatIndex(batchIdx, outBatchStrides, bBroadcastStrides)
			off := batchIdx * matrixSizeC
			matmulFloat64(c[off:off+matrixSizeC], a[aBatchFlat*matrixSizeA:], b[bBatchFlat*matrixSizeB:], m, k, n)
		}
		return
	}

	numWorkers := min(runtime.NumCPU(), totalBatches)
	batchesPerWorker := (totalBatches + numWorkers - 1) / numWorkers

	var wg sync.WaitGroup
	for w := range numWorkers {
		startBatch := w * batchesPerWorker
		if startBatch >= totalBatches {
			break
		}
		endBatch := min(startBatch+batchesPerWorker, totalBatches)
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			for batchIdx := start; batchIdx < end; batchIdx++ {
				aBatchFlat := computeFlatIndex(batchIdx, outBatchStrides, aBroadcastStrides)
				bBatchFlat := computeFlatIndex(batchIdx, outBatchStrides, bBroadcastStrides)
				off := batchIdx * matrixSizeC
				// Cap the slice to exactly m*n elements so matmulFloat64's
				// zero-initialisation loop does not overwrite adjacent batch slots.
				matmulFloat64(c[off:off+matrixSizeC], a[aBatchFlat*matrixSizeA:], b[bBatchFlat*matrixSizeB:], m, k, n)
			}
		}(startBatch, endBatch)
	}
	wg.Wait()
}
