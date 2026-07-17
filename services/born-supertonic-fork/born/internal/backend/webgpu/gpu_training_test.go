//go:build windows || linux

package webgpu

import (
	"math/rand"
	"runtime"
	"testing"

	"github.com/born-ml/born/internal/autodiff"
	"github.com/born-ml/born/internal/nn"
	"github.com/born-ml/born/internal/optim"
	"github.com/born-ml/born/internal/tensor"
)

// TestGPUTraining_MLP_OOM runs a multi-step MLP training loop on GPU to verify
// no OOM crash occurs. Primary regression test for ADR-016/017.
func TestGPUTraining_MLP_OOM(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	gpu, err := New()
	if err != nil {
		t.Fatalf("GPU backend: %v", err)
	}
	defer gpu.Release()

	backend := autodiff.New(gpu)
	t.Logf("Backend: %s", backend.Name())

	const (
		batchSize  = 32
		inputDim   = 128
		hiddenDim  = 64
		hidden2Dim = 32
		outputDim  = 10
		numSteps   = 20
	)

	type B = *autodiff.AutodiffBackend[*Backend]

	linear1 := nn.NewLinear[B](inputDim, hiddenDim, backend)
	linear2 := nn.NewLinear[B](hiddenDim, hidden2Dim, backend)
	linear3 := nn.NewLinear[B](hidden2Dim, outputDim, backend)

	params := make([]*nn.Parameter[B], 0, len(linear1.Parameters())+len(linear2.Parameters())+len(linear3.Parameters()))
	params = append(params, linear1.Parameters()...)
	params = append(params, linear2.Parameters()...)
	params = append(params, linear3.Parameters()...)

	optimizer := optim.NewAdam(params, optim.AdamConfig{LR: 0.001}, backend)

	rng := rand.New(rand.NewSource(42))

	var baselineHeap uint64

	for step := 0; step < numSteps; step++ {
		inputData := make([]float32, batchSize*inputDim)
		for i := range inputData {
			inputData[i] = rng.Float32()*2 - 1
		}
		targetData := make([]int32, batchSize)
		for i := range targetData {
			targetData[i] = rng.Int31n(int32(outputDim))
		}

		input, _ := tensor.FromSlice(inputData, tensor.Shape{batchSize, inputDim}, backend)
		targets, _ := tensor.FromSlice(targetData, tensor.Shape{batchSize}, backend)

		backend.Tape().StartRecording()

		h1 := linear1.Forward(input)
		a1 := tensor.New[float32, B](backend.ReLU(h1.Raw()), backend)
		h2 := linear2.Forward(a1)
		a2 := tensor.New[float32, B](backend.ReLU(h2.Raw()), backend)
		h3 := linear3.Forward(a2)

		lossRaw := backend.CrossEntropy(h3.Raw(), targets.Raw())
		loss := tensor.New[float32, B](lossRaw, backend)

		// Read loss value BEFORE backward/ClearTape — after ClearTape the
		// GPU buffer may be returned to pool and overwritten.
		lossVal := loss.Data()[0]

		grads := autodiff.Backward(loss, backend)
		optimizer.Step(grads)
		autodiff.ReleaseGradients(grads)
		backend.ClearTape()

		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		if step == 0 {
			baselineHeap = ms.HeapAlloc
		}

		poolTotal, poolInUse, _, poolReserved := gpu.gpuPool.Stats()

		if step%5 == 0 || step == numSteps-1 {
			t.Logf("Step %2d: loss=%.4f heap=%dMB pool(total=%d inUse=%d reserved=%dKB)",
				step, lossVal,
				ms.HeapAlloc/1024/1024,
				poolTotal, poolInUse, poolReserved/1024)
		}

		if ms.HeapAlloc > baselineHeap+500*1024*1024 {
			t.Fatalf("Step %d: heap grew %d MB over baseline — likely OOM leak",
				step, (ms.HeapAlloc-baselineHeap)/1024/1024)
		}
	}

	t.Logf("Completed %d steps without OOM", numSteps)
}

// TestGPUTraining_PoolReuse verifies that after warmup the pool stabilizes.
func TestGPUTraining_PoolReuse(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	gpu, err := New()
	if err != nil {
		t.Fatalf("GPU backend: %v", err)
	}
	defer gpu.Release()

	backend := autodiff.New(gpu)

	const (
		batchSize = 16
		dim       = 64
		numSteps  = 10
	)

	type B = *autodiff.AutodiffBackend[*Backend]

	linear := nn.NewLinear[B](dim, dim, backend)
	params := linear.Parameters()
	optimizer := optim.NewAdam(params, optim.AdamConfig{LR: 0.001}, backend)

	rng := rand.New(rand.NewSource(99))

	var warmupPages int

	for step := 0; step < numSteps; step++ {
		inputData := make([]float32, batchSize*dim)
		for i := range inputData {
			inputData[i] = rng.Float32()
		}
		targetData := make([]float32, batchSize*dim)
		for i := range targetData {
			targetData[i] = rng.Float32()
		}

		input, _ := tensor.FromSlice(inputData, tensor.Shape{batchSize, dim}, backend)
		target, _ := tensor.FromSlice(targetData, tensor.Shape{batchSize, dim}, backend)

		backend.Tape().StartRecording()

		output := linear.Forward(input)
		diff := tensor.New[float32, B](backend.Sub(output.Raw(), target.Raw()), backend)
		loss := tensor.New[float32, B](backend.Sum(backend.Mul(diff.Raw(), diff.Raw())), backend)

		grads := autodiff.Backward(loss, backend)
		optimizer.Step(grads)
		autodiff.ReleaseGradients(grads)
		backend.ClearTape()

		total, _, _, _ := gpu.gpuPool.Stats()

		if step == 2 {
			warmupPages = total
		}

		if step > 2 && total > warmupPages*2 {
			t.Errorf("Step %d: pool grew to %d pages (warmup was %d) — pool not reusing",
				step, total, warmupPages)
		}
	}

	finalTotal, _, _, _ := gpu.gpuPool.Stats() //nolint:dogsled // only need total count
	t.Logf("Pool reuse: warmup=%d pages, final=%d pages (%.0f%% growth)",
		warmupPages, finalTotal,
		float64(finalTotal-warmupPages)/float64(max(warmupPages, 1))*100)
}
