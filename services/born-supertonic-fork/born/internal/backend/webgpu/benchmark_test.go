//go:build windows || linux

package webgpu

import (
	"fmt"
	"math"
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
)

// =============================================================================
// Helper Functions
// =============================================================================

// createFloat32Tensor creates a tensor with random data for benchmarking.
func createFloat32Tensor(shape tensor.Shape, device tensor.Device) *tensor.RawTensor {
	raw, err := tensor.NewRaw(shape, tensor.Float32, device)
	if err != nil {
		panic(err)
	}
	// Fill with simple pattern (faster than random for benchmarks)
	data := raw.Data()
	for i := 0; i < len(data); i += 4 {
		val := float32(i % 1000)
		bits := math.Float32bits(val)
		data[i+0] = byte(bits)
		data[i+1] = byte(bits >> 8)
		data[i+2] = byte(bits >> 16)
		data[i+3] = byte(bits >> 24)
	}
	return raw
}

// =============================================================================
// Element-wise Addition Benchmarks
// =============================================================================

func benchmarkAdd(b *testing.B, backendType string, size int) {
	var a, other *tensor.RawTensor
	var backend tensor.Backend

	if backendType == "cpu" {
		backend = cpu.New()
		a = createFloat32Tensor(tensor.Shape{size}, tensor.CPU)
		other = createFloat32Tensor(tensor.Shape{size}, tensor.CPU)
	} else {
		if !IsAvailable() {
			b.Skip("WebGPU not available")
		}
		gpuBackend, err := New()
		if err != nil {
			b.Fatalf("failed to create WebGPU backend: %v", err)
		}
		defer gpuBackend.Release()
		backend = gpuBackend
		a = createFloat32Tensor(tensor.Shape{size}, tensor.WebGPU)
		other = createFloat32Tensor(tensor.Shape{size}, tensor.WebGPU)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = backend.Add(a, other)
	}
}

func BenchmarkCPU_Add_1K(b *testing.B)   { benchmarkAdd(b, "cpu", 1024) }
func BenchmarkCPU_Add_10K(b *testing.B)  { benchmarkAdd(b, "cpu", 10*1024) }
func BenchmarkCPU_Add_100K(b *testing.B) { benchmarkAdd(b, "cpu", 100*1024) }
func BenchmarkCPU_Add_1M(b *testing.B)   { benchmarkAdd(b, "cpu", 1024*1024) }

func BenchmarkWebGPU_Add_1K(b *testing.B)   { benchmarkAdd(b, "webgpu", 1024) }
func BenchmarkWebGPU_Add_10K(b *testing.B)  { benchmarkAdd(b, "webgpu", 10*1024) }
func BenchmarkWebGPU_Add_100K(b *testing.B) { benchmarkAdd(b, "webgpu", 100*1024) }
func BenchmarkWebGPU_Add_1M(b *testing.B)   { benchmarkAdd(b, "webgpu", 1024*1024) }

// =============================================================================
// Element-wise Multiplication Benchmarks
// =============================================================================

func benchmarkMul(b *testing.B, backendType string, size int) {
	var a, other *tensor.RawTensor
	var backend tensor.Backend

	if backendType == "cpu" {
		backend = cpu.New()
		a = createFloat32Tensor(tensor.Shape{size}, tensor.CPU)
		other = createFloat32Tensor(tensor.Shape{size}, tensor.CPU)
	} else {
		if !IsAvailable() {
			b.Skip("WebGPU not available")
		}
		gpuBackend, err := New()
		if err != nil {
			b.Fatalf("failed to create WebGPU backend: %v", err)
		}
		defer gpuBackend.Release()
		backend = gpuBackend
		a = createFloat32Tensor(tensor.Shape{size}, tensor.WebGPU)
		other = createFloat32Tensor(tensor.Shape{size}, tensor.WebGPU)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = backend.Mul(a, other)
	}
}

func BenchmarkCPU_Mul_1K(b *testing.B)   { benchmarkMul(b, "cpu", 1024) }
func BenchmarkCPU_Mul_100K(b *testing.B) { benchmarkMul(b, "cpu", 100*1024) }
func BenchmarkCPU_Mul_1M(b *testing.B)   { benchmarkMul(b, "cpu", 1024*1024) }

func BenchmarkWebGPU_Mul_1K(b *testing.B)   { benchmarkMul(b, "webgpu", 1024) }
func BenchmarkWebGPU_Mul_100K(b *testing.B) { benchmarkMul(b, "webgpu", 100*1024) }
func BenchmarkWebGPU_Mul_1M(b *testing.B)   { benchmarkMul(b, "webgpu", 1024*1024) }

// =============================================================================
// Matrix Multiplication Benchmarks
// =============================================================================

func benchmarkMatMul(b *testing.B, backendType string, size int) {
	var a, other *tensor.RawTensor
	var backend tensor.Backend

	if backendType == "cpu" {
		backend = cpu.New()
		a = createFloat32Tensor(tensor.Shape{size, size}, tensor.CPU)
		other = createFloat32Tensor(tensor.Shape{size, size}, tensor.CPU)
	} else {
		if !IsAvailable() {
			b.Skip("WebGPU not available")
		}
		gpuBackend, err := New()
		if err != nil {
			b.Fatalf("failed to create WebGPU backend: %v", err)
		}
		defer gpuBackend.Release()
		backend = gpuBackend
		a = createFloat32Tensor(tensor.Shape{size, size}, tensor.WebGPU)
		other = createFloat32Tensor(tensor.Shape{size, size}, tensor.WebGPU)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = backend.MatMul(a, other)
	}
}

func BenchmarkCPU_MatMul_64(b *testing.B)   { benchmarkMatMul(b, "cpu", 64) }
func BenchmarkCPU_MatMul_128(b *testing.B)  { benchmarkMatMul(b, "cpu", 128) }
func BenchmarkCPU_MatMul_256(b *testing.B)  { benchmarkMatMul(b, "cpu", 256) }
func BenchmarkCPU_MatMul_512(b *testing.B)  { benchmarkMatMul(b, "cpu", 512) }
func BenchmarkCPU_MatMul_1024(b *testing.B) { benchmarkMatMul(b, "cpu", 1024) }

func BenchmarkWebGPU_MatMul_64(b *testing.B)   { benchmarkMatMul(b, "webgpu", 64) }
func BenchmarkWebGPU_MatMul_128(b *testing.B)  { benchmarkMatMul(b, "webgpu", 128) }
func BenchmarkWebGPU_MatMul_256(b *testing.B)  { benchmarkMatMul(b, "webgpu", 256) }
func BenchmarkWebGPU_MatMul_512(b *testing.B)  { benchmarkMatMul(b, "webgpu", 512) }
func BenchmarkWebGPU_MatMul_1024(b *testing.B) { benchmarkMatMul(b, "webgpu", 1024) }

// =============================================================================
// Transpose Benchmarks
// =============================================================================

func benchmarkTranspose(b *testing.B, backendType string, rows, cols int) {
	var a *tensor.RawTensor
	var backend tensor.Backend

	if backendType == "cpu" {
		backend = cpu.New()
		a = createFloat32Tensor(tensor.Shape{rows, cols}, tensor.CPU)
	} else {
		if !IsAvailable() {
			b.Skip("WebGPU not available")
		}
		gpuBackend, err := New()
		if err != nil {
			b.Fatalf("failed to create WebGPU backend: %v", err)
		}
		defer gpuBackend.Release()
		backend = gpuBackend
		a = createFloat32Tensor(tensor.Shape{rows, cols}, tensor.WebGPU)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = backend.Transpose(a)
	}
}

func BenchmarkCPU_Transpose_256(b *testing.B)  { benchmarkTranspose(b, "cpu", 256, 256) }
func BenchmarkCPU_Transpose_512(b *testing.B)  { benchmarkTranspose(b, "cpu", 512, 512) }
func BenchmarkCPU_Transpose_1024(b *testing.B) { benchmarkTranspose(b, "cpu", 1024, 1024) }

func BenchmarkWebGPU_Transpose_256(b *testing.B)  { benchmarkTranspose(b, "webgpu", 256, 256) }
func BenchmarkWebGPU_Transpose_512(b *testing.B)  { benchmarkTranspose(b, "webgpu", 512, 512) }
func BenchmarkWebGPU_Transpose_1024(b *testing.B) { benchmarkTranspose(b, "webgpu", 1024, 1024) }

// =============================================================================
// Memory Transfer Benchmarks (WebGPU only)
// =============================================================================

func BenchmarkWebGPU_Transfer_Upload_1M(b *testing.B) {
	if !IsAvailable() {
		b.Skip("WebGPU not available")
	}
	gpuBackend, err := New()
	if err != nil {
		b.Fatalf("failed to create WebGPU backend: %v", err)
	}
	defer gpuBackend.Release()

	// Create CPU data
	size := 1024 * 1024
	data := make([]byte, size*4) // float32

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate upload by creating buffer with data
		_ = gpuBackend.createBuffer(data, 0x80) // BufferUsageStorage
	}
}

// =============================================================================
// Combined Operation Benchmarks (simulating real workload)
// =============================================================================

func benchmarkCombinedOps(b *testing.B, backendType string, size int) {
	var a, other, c *tensor.RawTensor
	var backend tensor.Backend

	if backendType == "cpu" {
		backend = cpu.New()
		a = createFloat32Tensor(tensor.Shape{size}, tensor.CPU)
		other = createFloat32Tensor(tensor.Shape{size}, tensor.CPU)
		c = createFloat32Tensor(tensor.Shape{size}, tensor.CPU)
	} else {
		if !IsAvailable() {
			b.Skip("WebGPU not available")
		}
		gpuBackend, err := New()
		if err != nil {
			b.Fatalf("failed to create WebGPU backend: %v", err)
		}
		defer gpuBackend.Release()
		backend = gpuBackend
		a = createFloat32Tensor(tensor.Shape{size}, tensor.WebGPU)
		other = createFloat32Tensor(tensor.Shape{size}, tensor.WebGPU)
		c = createFloat32Tensor(tensor.Shape{size}, tensor.WebGPU)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate: result = (a + b) * c
		tmp := backend.Add(a, other)
		_ = backend.Mul(tmp, c)
	}
}

func BenchmarkCPU_Combined_100K(b *testing.B)    { benchmarkCombinedOps(b, "cpu", 100*1024) }
func BenchmarkCPU_Combined_1M(b *testing.B)      { benchmarkCombinedOps(b, "cpu", 1024*1024) }
func BenchmarkWebGPU_Combined_100K(b *testing.B) { benchmarkCombinedOps(b, "webgpu", 100*1024) }
func BenchmarkWebGPU_Combined_1M(b *testing.B)   { benchmarkCombinedOps(b, "webgpu", 1024*1024) }

// =============================================================================
// Summary Report Generator (run with -v flag)
// =============================================================================

func TestPrintBenchmarkInfo(_ *testing.T) {
	fmt.Println("\n" + "=" + "===========================================")
	fmt.Println("  Born ML Framework - Benchmark Suite")
	fmt.Println("============================================")

	fmt.Println("\nWebGPU Status:")
	if IsAvailable() {
		fmt.Println("  - WebGPU: AVAILABLE")
		backend, err := New()
		if err == nil {
			fmt.Printf("  - Backend: %s\n", backend.Name())
			stats := backend.MemoryStats()
			fmt.Printf("  - Memory tracking: Active\n")
			fmt.Printf("  - Pool stats: %d allocated, %d pooled\n", stats.PoolAllocated, stats.PooledBuffers)
			backend.Release()
		}
	} else {
		fmt.Println("  - WebGPU: NOT AVAILABLE")
	}

	fmt.Println("\nRun benchmarks with:")
	fmt.Println("  go test -bench=. -benchmem ./internal/backend/webgpu/")
	fmt.Println("\nFor specific comparisons:")
	fmt.Println("  go test -bench=MatMul -benchmem ./internal/backend/webgpu/")
	fmt.Println("  go test -bench=Add -benchmem ./internal/backend/webgpu/")
}
