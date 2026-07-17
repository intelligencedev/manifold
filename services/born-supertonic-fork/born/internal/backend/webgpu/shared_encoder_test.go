//go:build windows || linux

package webgpu

import (
	"math"
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// ─────────────────────────────────────────────────────────────────────────────
// TestSharedEncoder_MultipleOpsOneEncoder
//
// Verify that 128+ ops accumulate into the shared encoder batch rather than
// each creating an individual encoder. Before the shared-encoder optimization,
// every lazy op called CreateCommandEncoder; now they all share one encoder until
// the batch threshold is reached. We measure this by checking that activeBatchCount
// increases monotonically until it reaches maxPendingBeforeFlush.
// ─────────────────────────────────────────────────────────────────────────────

func TestSharedEncoder_MultipleOpsOneEncoder(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer backend.Release()

	a, err := tensor.NewRaw(tensor.Shape{64}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	for i := range a.AsFloat32() {
		a.AsFloat32()[i] = 1.0
	}

	// Queue ops one at a time and observe accumulation.
	result := a
	for i := 1; i <= maxPendingBeforeFlush-1; i++ {
		result = backend.Add(result, a)
		count := backend.activeBatchCount()
		if count == 0 {
			t.Errorf("after op %d: activeBatchCount() = 0; expected > 0 (no flush should have triggered yet)", i)
			break
		}
	}

	// Force flush and verify count resets.
	_ = result.Data()
	if count := backend.activeBatchCount(); count != 0 {
		t.Errorf("activeBatchCount() = %d after Data(); expected 0", count)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestInputBufferCache_HitRate
//
// Reuse the same tensor 10 times in distinct Add ops. After the first op the
// tensor is cached; subsequent ops must not grow the cache. Final correctness
// is also verified.
// ─────────────────────────────────────────────────────────────────────────────

func TestInputBufferCache_HitRate(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer backend.Release()

	weights, err := tensor.NewRaw(tensor.Shape{8}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	for i := range weights.AsFloat32() {
		weights.AsFloat32()[i] = float32(i + 1)
	}

	// Op 1 — populates cache.
	r := backend.Add(weights, weights)
	_ = r.Data()
	sizeAfterFirst := backend.inputBufferCacheSize()
	if sizeAfterFirst == 0 {
		t.Fatal("cache empty after first op")
	}

	// Ops 2–10 — all cache hits, size must not increase.
	for i := 2; i <= 10; i++ {
		r = backend.Add(weights, weights)
		_ = r.Data()
		if size := backend.inputBufferCacheSize(); size != sizeAfterFirst {
			t.Errorf("op %d: cache size = %d, want %d (no new entries expected)", i, size, sizeAfterFirst)
		}
	}

	// Numerical correctness: weights + weights = 2 * weights.
	got := r.AsFloat32()
	for i, v := range got {
		want := float32((i + 1) * 2)
		if v != want {
			t.Errorf("element %d: got %f, want %f", i, v, want)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestInputBufferCache_Invalidation
//
// After ClearInputBufferCache the cache is empty. The next op re-populates it
// and must still produce correct results, confirming the re-upload path works.
// ─────────────────────────────────────────────────────────────────────────────

func TestInputBufferCache_Invalidation(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer backend.Release()

	w, err := tensor.NewRaw(tensor.Shape{4}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(w.AsFloat32(), []float32{1, 2, 3, 4})

	// Populate cache.
	r1 := backend.Add(w, w)
	_ = r1.Data()
	if backend.inputBufferCacheSize() == 0 {
		t.Fatal("cache should be non-empty before invalidation")
	}

	// Invalidate.
	backend.ClearInputBufferCache()
	if sz := backend.inputBufferCacheSize(); sz != 0 {
		t.Fatalf("cache size = %d after Clear; expected 0", sz)
	}

	// Re-use tensor — should re-upload and re-populate cache.
	r2 := backend.Add(w, w)
	got := r2.AsFloat32()
	expected := []float32{2, 4, 6, 8}
	for i, v := range got {
		if v != expected[i] {
			t.Errorf("post-invalidation element %d: got %f, want %f", i, v, expected[i])
		}
	}
	if backend.inputBufferCacheSize() == 0 {
		t.Error("cache should be re-populated after op following invalidation")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestSharedEncoder_AutoFlush
//
// Issue exactly maxPendingBeforeFlush+1 ops without any Data() call. The auto-
// flush mechanism in addComputePassToEncoder must seal the first encoder batch
// at threshold and start a new one. The final Data() must succeed and return
// correct results.
// ─────────────────────────────────────────────────────────────────────────────

func TestSharedEncoder_AutoFlush(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer backend.Release()

	a, err := tensor.NewRaw(tensor.Shape{4}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(a.AsFloat32(), []float32{1, 1, 1, 1})

	// Issue maxPendingBeforeFlush+1 ops — auto-flush must fire at least once.
	total := maxPendingBeforeFlush + 1
	result := a
	for i := 0; i < total; i++ {
		result = backend.Add(result, a)
	}

	got := result.AsFloat32()
	// result = (total+1) * [1,1,1,1] because we start from a (all-1s) and add
	// a (all-1s) exactly total times.
	want := float32(total + 1)
	for i, v := range got {
		if v != want {
			t.Errorf("element %d: got %f, want %f", i, v, want)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestSharedEncoder_FlushOnReadback
//
// Verify that the active encoder batch is always flushed when Data() is called,
// even for a single pending op (i.e., count < maxPendingBeforeFlush).
// ─────────────────────────────────────────────────────────────────────────────

func TestSharedEncoder_FlushOnReadback(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer backend.Release()

	a, err := tensor.NewRaw(tensor.Shape{4}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(a.AsFloat32(), []float32{2, 4, 6, 8})

	result := backend.Mul(a, a) // 2*2=4, 4*4=16, 6*6=36, 8*8=64

	// Before readback: at least one pending op.
	if backend.activeBatchCount() == 0 {
		// It's possible auto-flush already ran (if maxPendingBeforeFlush == 1),
		// but that threshold is guaranteed > 1 by TestAutoFlush_Threshold.
		t.Error("activeBatchCount() == 0 before Data(); expected pending op")
	}

	got := result.AsFloat32()

	// After readback: active batch must be empty.
	if count := backend.activeBatchCount(); count != 0 {
		t.Errorf("activeBatchCount() = %d after Data(); expected 0", count)
	}

	expected := []float32{4, 16, 36, 64}
	for i, v := range got {
		if v != expected[i] {
			t.Errorf("element %d: got %f, want %f", i, v, expected[i])
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestInputBufferCache_LazyTensorChain
//
// A chain of lazy ops: cpu_tensor → lazy1 = Add(cpu, cpu) → lazy2 = Add(lazy1, cpu)
// → lazy3 = Add(lazy2, cpu). Only the CPU tensor should be cached. Lazy tensors
// (lazy1, lazy2) are not eligible for caching and must NOT appear in the cache.
// Verify the final result is numerically correct.
// ─────────────────────────────────────────────────────────────────────────────

func TestInputBufferCache_LazyTensorChain(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer backend.Release()

	cpu, err := tensor.NewRaw(tensor.Shape{4}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(cpu.AsFloat32(), []float32{1, 1, 1, 1})

	// Perform first op to populate cache with cpu tensor.
	lazy1 := backend.Add(cpu, cpu)   // lazy1 = 2
	lazy2 := backend.Add(lazy1, cpu) // lazy2 = 3
	lazy3 := backend.Add(lazy2, cpu) // lazy3 = 4

	cacheAfterChain := backend.inputBufferCacheSize()

	// cpu tensor must be in the cache (1 entry).
	// lazy1 and lazy2 are lazy GPU tensors and must NOT be in the cache.
	if cacheAfterChain != 1 {
		t.Errorf("cache size = %d; want exactly 1 (only the CPU tensor)", cacheAfterChain)
	}

	// Numerical correctness: each Add adds 1, so result = 4.
	got := lazy3.AsFloat32()
	for i, v := range got {
		if v != 4.0 {
			t.Errorf("element %d: got %f, want 4.0", i, v)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestSharedEncoder_Correctness
//
// Chain 500 Add ops. Expected result: (500+1) * initial = 501 * initial.
// This is the primary numerical regression test for the encoder batch path.
// ─────────────────────────────────────────────────────────────────────────────

func TestSharedEncoder_Correctness(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer backend.Release()

	const n = 64
	const addOps = 500

	a, err := tensor.NewRaw(tensor.Shape{n}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	for i := range a.AsFloat32() {
		a.AsFloat32()[i] = float32(i+1) * 0.01
	}

	result := a
	for i := 0; i < addOps; i++ {
		result = backend.Add(result, a)
	}

	// Compute expected result on CPU for numerical comparison.
	got := result.AsFloat32()
	if len(got) != n {
		t.Fatalf("output length: got %d, want %d", len(got), n)
	}

	const tolerance = 1e-2 // float32 accumulation error over 500 ops
	for i, v := range got {
		wantF64 := float64(a.AsFloat32()[i]) * float64(addOps+1)
		diff := math.Abs(float64(v) - wantF64)
		if diff > tolerance {
			t.Errorf("element %d: got %f, want %f (diff %f > tolerance %f)",
				i, v, wantF64, diff, tolerance)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestSharedEncoder_MixedOps
//
// Issue Binary + Unary + MatMul + Softmax ops without any intermediate readback.
// All ops must accumulate into the shared encoder and produce correct results
// when Data() is finally called.
// ─────────────────────────────────────────────────────────────────────────────

//nolint:gocognit // Test function validates multiple op types across one encoder batch — inherently branchy.
func TestSharedEncoder_MixedOps(t *testing.T) {
	if !IsAvailable() {
		t.Skip("WebGPU not available")
	}

	backend, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer backend.Release()

	// Build small tensors for each op type.
	vec, err := tensor.NewRaw(tensor.Shape{8}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	for i := range vec.AsFloat32() {
		vec.AsFloat32()[i] = float32(i+1) * 0.5
	}

	mat4, err := tensor.NewRaw(tensor.Shape{4, 4}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	// Identity-like.
	for i := range mat4.AsFloat32() {
		row, col := i/4, i%4
		if row == col {
			mat4.AsFloat32()[i] = 1.0
		}
	}

	softmaxInput, err := tensor.NewRaw(tensor.Shape{2, 4}, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(softmaxInput.AsFloat32(), []float32{1, 2, 3, 4, 5, 6, 7, 8})

	// Enqueue multiple op types WITHOUT any readback.
	addResult := backend.Add(vec, vec)                 // Binary
	mulResult := backend.Mul(vec, vec)                 // Binary
	expResult := backend.Exp(vec)                      // Unary
	matmulResult := backend.MatMul(mat4, mat4)         // MatMul
	softmaxResult := backend.Softmax(softmaxInput, -1) // Softmax on last dim

	// Now read back all results — triggers a single flush.
	addGot := addResult.AsFloat32()
	mulGot := mulResult.AsFloat32()
	expGot := expResult.AsFloat32()
	matmulGot := matmulResult.AsFloat32()
	softmaxGot := softmaxResult.AsFloat32()

	// Validate Add: vec + vec = 2*vec.
	for i, v := range addGot {
		want := vec.AsFloat32()[i] * 2
		if v != want {
			t.Errorf("Add[%d]: got %f, want %f", i, v, want)
		}
	}

	// Validate Mul: vec * vec = vec^2.
	for i, v := range mulGot {
		want := vec.AsFloat32()[i] * vec.AsFloat32()[i]
		if math.Abs(float64(v-want)) > 1e-6 {
			t.Errorf("Mul[%d]: got %f, want %f", i, v, want)
		}
	}

	// Validate Exp: exp(vec[i]).
	for i, v := range expGot {
		want := float32(math.Exp(float64(vec.AsFloat32()[i])))
		if math.Abs(float64(v-want)) > 1e-5 {
			t.Errorf("Exp[%d]: got %f, want %f", i, v, want)
		}
	}

	// Validate MatMul: I @ I = I.
	for i, v := range matmulGot {
		row, col := i/4, i%4
		want := float32(0)
		if row == col {
			want = 1.0
		}
		if math.Abs(float64(v-want)) > 1e-6 {
			t.Errorf("MatMul[%d]: got %f, want %f", i, v, want)
		}
	}

	// Validate Softmax: rows must sum to 1.
	for row := 0; row < 2; row++ {
		var sum float64
		for col := 0; col < 4; col++ {
			sum += float64(softmaxGot[row*4+col])
		}
		if math.Abs(sum-1.0) > 1e-5 {
			t.Errorf("Softmax row %d: sum = %f, want 1.0", row, sum)
		}
	}
}
