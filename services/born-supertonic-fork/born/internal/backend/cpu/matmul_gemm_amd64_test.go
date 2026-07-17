//go:build amd64 && !goexperiment.simd

package cpu

import (
	"math"
	"math/rand"
	"testing"

	"github.com/born-ml/born/internal/tensor"
	"golang.org/x/sys/cpu"
)

// rawFromSlice builds a row-major float32 RawTensor with the given shape,
// copying data into its backing buffer.
func rawFromSlice(t *testing.T, data []float32, shape ...int) *tensor.RawTensor {
	t.Helper()
	sh := make(tensor.Shape, len(shape))
	copy(sh, shape)
	rt, err := tensor.NewRaw(sh, tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(rt.AsFloat32(), data)
	return rt
}

// naiveMatMulF32 is an independent reference: C[m,n] = A[m,k] @ B[k,n], all
// row-major. Used as the oracle for the vendored SIMD GEMM kernel.
func naiveMatMulF32(a, b []float32, m, k, n int) []float32 {
	c := make([]float32, m*n)
	for i := 0; i < m; i++ {
		for j := 0; j < n; j++ {
			var sum float32
			for kk := 0; kk < k; kk++ {
				sum += a[i*k+kk] * b[kk*n+j]
			}
			c[i*n+j] = sum
		}
	}
	return c
}

func randSliceF32(r *rand.Rand, n int) []float32 {
	s := make([]float32, n)
	for i := range s {
		s[i] = float32(r.NormFloat64())
	}
	return s
}

// TestGemmAVX2F32MatchesScalar verifies the vendored AVX2+FMA GEMM kernel
// produces the same product as the naive reference across tile-aligned shapes,
// row/col/k tails, and a model-representative large-K shape.
func TestGemmAVX2F32MatchesScalar(t *testing.T) {
	if !cpu.X86.HasAVX2 || !cpu.X86.HasFMA {
		t.Skip("AVX2+FMA not available on this CPU")
	}
	r := rand.New(rand.NewSource(0x6770656d6d)) // "gemm"

	shapes := []struct{ m, k, n int }{
		{1, 1, 1},
		{4, 0, 32},   // empty inner dimension (k==0) with a full tile: zero matrix
		{1, 64, 32},  // GEMV: 1 row, full column tiles (1x16 kernel)
		{2, 100, 48}, // 2 remainder rows
		{3, 77, 33},  // 3 remainder rows + column tail
		{4, 8, 16},   // 4 remainder rows
		{4, 8, 17},   // column tail
		{5, 8, 16},   // 5 remainder rows (max mr tail)
		{5, 8, 17},   // row + column tail
		{3, 7, 11},   // small odd
		{8, 32, 32},  // multi-tile
		{64, 96, 128},
		{33, 65, 129}, // odd primes-ish, both tails
		// 6x16 packing stress: every m%6 residue against full/partial n tiles.
		{6, 8, 16},     // exact 6x16 tile
		{6, 40, 48},    // multi-k, multi-n-tile, exact rows
		{7, 16, 16},    // 1 remainder row over a full block
		{12, 16, 32},   // two full row blocks
		{13, 17, 33},   // two blocks + 1 row + k/n tails
		{6, 5, 16},     // k < default block, exact rows
		{6, 16, 17},    // exact rows + column tail
		{17, 16, 16},   // 2 blocks + 5 remainder rows
		{1, 1024, 96},  // classifier-like GEMV, full n tiles
		{7, 2048, 513}, // model-representative large K + row/col tails
		{6, 2048, 32},  // large K, exact rows
		// Column-tail stress: n%gemmNr == 8 (the n=24 conv shapes) across row residues.
		{7, 100, 24},   // 1 full tile + 8-col tail, row tail
		{13, 64, 40},   // 2 full tiles + 8-col tail, row tail
		{192, 192, 24}, // model-shaped: many blocks + 8-col tail
		{1, 1024, 24},  // GEMV + 8-col tail
		{5, 64, 8},     // pure tail (n < gemmNr), thin rows
		{18, 128, 8},   // pure tail (n < gemmNr), full blocks
	}

	for _, s := range shapes {
		a := randSliceF32(r, s.m*s.k)
		b := randSliceF32(r, s.k*s.n)
		want := naiveMatMulF32(a, b, s.m, s.k, s.n)

		got := make([]float32, s.m*s.n)
		for i := range got {
			got[i] = 12345.0 // poison: kernel must overwrite, not accumulate
		}
		gemmAVX2F32(got, a, b, s.m, s.k, s.n)

		var maxDiff float64
		for i := range want {
			d := math.Abs(float64(got[i]-want[i])) / (1 + math.Abs(float64(want[i])))
			if d > maxDiff {
				maxDiff = d
			}
		}
		if maxDiff > 1e-4 {
			t.Errorf("shape %dx%dx%d: max rel diff %.3e exceeds 1e-4", s.m, s.k, s.n, maxDiff)
		}
	}
}

// withGemmF32 temporarily forces gemmF32 to fn for the duration of a test
// (regardless of what init wired in on this host) and restores the previous value
// afterwards. Mutating the package global is safe here only because no test in
// this package calls t.Parallel(); keep matmul-exercising tests sequential, or
// this becomes a data race.
func withGemmF32(t *testing.T, fn func(c, a, b []float32, m, k, n int)) {
	t.Helper()
	prev := gemmF32
	gemmF32 = fn
	t.Cleanup(func() { gemmF32 = prev })
}

// TestMatMulGemmDispatch verifies matmulFloat32 routes large multiplications to
// the SIMD kernel (m*k*n >= blockThreshold) and small ones to the scalar path,
// and that both produce correct results. A sentinel records whether the gemm
// path was actually taken so the test is not vacuous.
func TestMatMulGemmDispatch(t *testing.T) {
	if !cpu.X86.HasAVX2 || !cpu.X86.HasFMA {
		t.Skip("AVX2+FMA not available on this CPU")
	}
	r := rand.New(rand.NewSource(0x6469737061746368)) // "dispatch"

	var called bool
	withGemmF32(t, func(c, a, b []float32, m, k, n int) {
		called = true
		gemmAVX2F32(c, a, b, m, k, n)
	})

	cases := []struct {
		name          string
		m, k, n       int
		wantGemmTaken bool
	}{
		{"large routes to gemm", 64, 2048, 96, true},            // full 4x16 tiles
		{"thin m<4 routes to gemm (GEMV)", 1, 1024, 6522, true}, // 1x16 remainder/GEMV path
		{"small stays scalar", 8, 8, 8, false},                  // 512 < blockThreshold
		{"narrow n<16 stays scalar", 64, 1024, 8, false},        // n < one full column tile
	}

	be := New()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called = false
			a := randSliceF32(r, tc.m*tc.k)
			b := randSliceF32(r, tc.k*tc.n)
			at := rawFromSlice(t, a, tc.m, tc.k)
			bt := rawFromSlice(t, b, tc.k, tc.n)

			got := be.MatMul(at, bt).AsFloat32()
			want := naiveMatMulF32(a, b, tc.m, tc.k, tc.n)

			if called != tc.wantGemmTaken {
				t.Errorf("gemm path taken = %v, want %v", called, tc.wantGemmTaken)
			}
			var maxDiff float64
			for i := range want {
				d := math.Abs(float64(got[i]-want[i])) / (1 + math.Abs(float64(want[i])))
				if d > maxDiff {
					maxDiff = d
				}
			}
			if maxDiff > 1e-4 {
				t.Errorf("max rel diff %.3e exceeds 1e-4", maxDiff)
			}
		})
	}
}

// TestGemmAVX2F32NoAllocs asserts the GEMM fast path performs no heap
// allocations in steady state (the micro-kernel is //go:noescape; the driver
// reslices and reuses pooled packing scratch).
//
// Skipped under -short (which CI uses, alongside -race): the scratch lives in a
// package-global sync.Pool that every other matmul test now exercises through the
// always-on dispatch, so AllocsPerRun can attribute a spurious per-call allocation
// to pool churn or race-detector bookkeeping, and that varies by platform. This is
// a deterministic local check (go test, no -short); a regression that dropped
// pooling would allocate the packing buffers on every call regardless.
func TestGemmAVX2F32NoAllocs(t *testing.T) {
	if testing.Short() {
		t.Skip("AllocsPerRun over the shared sync.Pool is unreliable under -short -race")
	}
	if !cpu.X86.HasAVX2 || !cpu.X86.HasFMA {
		t.Skip("AVX2+FMA not available on this CPU")
	}
	const m, k, n = 64, 256, 96
	r := rand.New(rand.NewSource(1))
	a := randSliceF32(r, m*k)
	b := randSliceF32(r, k*n)
	c := make([]float32, m*n)
	if allocs := testing.AllocsPerRun(20, func() { gemmAVX2F32(c, a, b, m, k, n) }); allocs != 0 {
		t.Errorf("gemmAVX2F32 allocated %v times, want 0", allocs)
	}
}

// BenchmarkGemmF32 compares the scalar matmul against the vendored AVX2+FMA kernel
// at the model's dominant GEMM shapes and a square reference. Run with:
// go test -run x -bench BenchmarkGemmF32 -benchmem (no GOEXPERIMENT needed; the
// kernel is in the default build). The kernel is wired into gemmF32 at init on a
// capable CPU, so the scalar sub-benchmark forces gemmF32 nil to actually measure
// the scalar path rather than re-dispatching to SIMD.
func BenchmarkGemmF32(b *testing.B) {
	shapes := []struct {
		name    string
		m, k, n int
	}{
		{"stft_511x2048x1025", 511, 2048, 1025}, // dominant front-end GEMM
		{"511x1024x513", 511, 1024, 513},
		{"classifier_1x1024x6522", 1, 1024, 6522},
		{"square_512", 512, 512, 512},
	}
	r := rand.New(rand.NewSource(7))
	for _, s := range shapes {
		a := randSliceF32(r, s.m*s.k)
		bb := randSliceF32(r, s.k*s.n)
		c := make([]float32, s.m*s.n)
		b.Run(s.name+"/scalar", func(b *testing.B) {
			prev := gemmF32
			gemmF32 = nil // force the scalar path, even on a capable CPU
			defer func() { gemmF32 = prev }()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				matmulFloat32(c, a, bb, s.m, s.k, s.n)
			}
		})
		b.Run(s.name+"/simd", func(b *testing.B) {
			if !cpu.X86.HasAVX2 || !cpu.X86.HasFMA {
				b.Skip("AVX2+FMA not available")
			}
			for i := 0; i < b.N; i++ {
				gemmAVX2F32(c, a, bb, s.m, s.k, s.n)
			}
		})
	}
}

// TestGemmWiredInMatchesCPU verifies the always-on dispatch contract: init wires
// the kernel into gemmF32 exactly when the CPU has AVX2+FMA, with no env flag or
// build tag involved. A regression that dropped the CPU check (wiring it in on an
// unsupported CPU) or failed to wire it in on a capable one would fail here.
//
// gemmF32 is read at its init value: every other test restores it via withGemmF32's
// cleanup, and no test runs in parallel, so it holds the init wiring here.
func TestGemmWiredInMatchesCPU(t *testing.T) {
	want := cpu.X86.HasAVX2 && cpu.X86.HasFMA
	if got := gemmF32 != nil; got != want {
		t.Errorf("gemmF32 wired in = %v, want %v (AVX2=%v FMA=%v)",
			got, want, cpu.X86.HasAVX2, cpu.X86.HasFMA)
	}
}
