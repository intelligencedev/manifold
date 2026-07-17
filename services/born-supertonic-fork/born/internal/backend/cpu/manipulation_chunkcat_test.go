package cpu

import (
	"bytes"
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// fillSeqTyped fills x with deterministic, dtype-valid values via its typed view
// so the round-trip preserves the exact bytes (e.g. bool normalizes to 0/1).
func fillSeqTyped(x *tensor.RawTensor, dt tensor.DataType) {
	switch dt {
	case tensor.Float64:
		s := x.AsFloat64()
		for i := range s {
			s[i] = float64(i)
		}
	case tensor.Int32:
		s := x.AsInt32()
		for i := range s {
			s[i] = int32(i)
		}
	case tensor.Int64:
		s := x.AsInt64()
		for i := range s {
			s[i] = int64(i)
		}
	case tensor.Uint8:
		s := x.AsUint8()
		for i := range s {
			s[i] = uint8(i)
		}
	case tensor.Bool:
		s := x.AsBool()
		for i := range s {
			s[i] = i%2 == 0
		}
	}
}

// TestChunkCatDtypesRoundTrip exercises Cat(Chunk(x)) == x for every non-float32
// dtype the CPU backend supports. The float32 path is covered in depth by
// TestChunkCatFloat32Contiguous; this guards the mechanically-derived variants,
// which are otherwise only reachable through these helpers (the tensor-package
// dtype tests run against MockBackend, not this code).
func TestChunkCatDtypesRoundTrip(t *testing.T) {
	be := New()
	shape := tensor.Shape{2, 6, 2} // dim=1 split into 3: outer>1 and inner>1
	const n, dim = 3, 1
	dtypes := []tensor.DataType{tensor.Float64, tensor.Int32, tensor.Int64, tensor.Uint8, tensor.Bool}
	for _, dt := range dtypes {
		t.Run(dt.String(), func(t *testing.T) {
			x, err := tensor.NewRaw(shape, dt, tensor.CPU)
			if err != nil {
				t.Fatal(err)
			}
			fillSeqTyped(x, dt)
			orig := append([]byte(nil), x.Data()...)
			chunks := be.Chunk(x, n, dim)
			if len(chunks) != n {
				t.Fatalf("%s: got %d chunks, want %d", dt, len(chunks), n)
			}
			if got := be.Cat(chunks, dim).Data(); !bytes.Equal(got, orig) {
				t.Fatalf("%s: Cat(Chunk(x)) bytes differ from original", dt)
			}
		})
	}
}

// naiveChunkRef computes, for a row-major float32 tensor split into n chunks
// along dim, the expected contents of chunk ci using explicit coordinate math.
// Independent reference for the contiguous-copy implementation.
func naiveChunkRef(data []float32, shape []int, n, dim, ci int) []float32 {
	strides := make([]int, len(shape))
	strides[len(shape)-1] = 1
	for d := len(shape) - 2; d >= 0; d-- {
		strides[d] = strides[d+1] * shape[d+1]
	}
	chunkSize := shape[dim] / n
	outShape := append([]int(nil), shape...)
	outShape[dim] = chunkSize
	outStrides := make([]int, len(outShape))
	outStrides[len(outShape)-1] = 1
	for d := len(outShape) - 2; d >= 0; d-- {
		outStrides[d] = outStrides[d+1] * outShape[d+1]
	}
	total := 1
	for _, s := range shape {
		total *= s
	}
	out := make([]float32, total/n)
	coords := make([]int, len(shape))
	for i := 0; i < total; i++ {
		t := i
		for d := range shape {
			coords[d] = t / strides[d]
			t %= strides[d]
		}
		if coords[dim]/chunkSize != ci {
			continue
		}
		oi := 0
		for d := range coords {
			c := coords[d]
			if d == dim {
				c %= chunkSize
			}
			oi += c * outStrides[d]
		}
		out[oi] = data[i]
	}
	return out
}

// BenchmarkChunkFloat32 measures the contiguous chunk copy on a depthwise-style
// split ([1,256,8,8] -> 256 single-channel chunks). Result tensors are
// preallocated so the benchmark isolates the copy: it should report 0 allocs/op
// (the old per-element implementation allocated a coords slice per element).
func BenchmarkChunkFloat32(b *testing.B) {
	const c = 256
	shape := tensor.Shape{1, c, 8, 8}
	x, err := tensor.NewRaw(shape, tensor.Float32, tensor.CPU)
	if err != nil {
		b.Fatal(err)
	}
	results := make([]*tensor.RawTensor, c)
	for i := range results {
		results[i], err = tensor.NewRaw(tensor.Shape{1, 1, 8, 8}, tensor.Float32, tensor.CPU)
		if err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chunkFloat32(x, results, 1)
	}
}

func newSeqTensorF32(t *testing.T, shape ...int) (*tensor.RawTensor, []float32) {
	t.Helper()
	n := 1
	for _, s := range shape {
		n *= s
	}
	data := make([]float32, n)
	for i := range data {
		data[i] = float32(i)
	}
	rt, err := tensor.NewRaw(tensor.Shape(shape), tensor.Float32, tensor.CPU)
	if err != nil {
		t.Fatal(err)
	}
	copy(rt.AsFloat32(), data)
	return rt, data
}

// TestChunkCatFloat32Contiguous checks chunk contents against an independent
// coordinate-based reference and verifies Cat(Chunk(x)) round-trips to x, across
// several ranks and split dimensions (including the channel axis of a 4D NCHW
// tensor, the depthwise-conv pattern).
func TestChunkCatFloat32Contiguous(t *testing.T) {
	be := New()
	cases := []struct {
		shape  []int
		n, dim int
	}{
		{[]int{6}, 3, 0},
		{[]int{4, 8}, 2, 0},
		{[]int{4, 8}, 4, 1},
		{[]int{2, 6, 3}, 3, 1},
		{[]int{1, 12, 2, 2}, 12, 1}, // 4D channel chunk (depthwise pattern)
		{[]int{2, 3, 5}, 5, 2},
		{[]int{4, 8}, 1, 0},    // n=1: single-chunk identity split
		{[]int{2, 3, 5}, 1, 1}, // n=1 on an inner dim
	}
	for _, c := range cases {
		x, data := newSeqTensorF32(t, c.shape...)
		chunks := be.Chunk(x, c.n, c.dim)
		if len(chunks) != c.n {
			t.Fatalf("shape %v dim %d: got %d chunks, want %d", c.shape, c.dim, len(chunks), c.n)
		}
		assertChunksMatchRef(t, data, c.shape, c.n, c.dim, chunks)
		assertCatReconstructs(t, be, chunks, c.dim, data, c.shape)
	}
}

// assertChunksMatchRef checks each chunk's contents against naiveChunkRef.
func assertChunksMatchRef(t *testing.T, data []float32, shape []int, n, dim int, chunks []*tensor.RawTensor) {
	t.Helper()
	for ci, ch := range chunks {
		want := naiveChunkRef(data, shape, n, dim, ci)
		got := ch.AsFloat32()
		if len(got) != len(want) {
			t.Fatalf("shape %v dim %d chunk %d: len %d want %d", shape, dim, ci, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("shape %v dim %d chunk %d: [%d]=%v want %v", shape, dim, ci, i, got[i], want[i])
			}
		}
	}
}

// assertCatReconstructs verifies Cat(chunks) reproduces the original data.
func assertCatReconstructs(t *testing.T, be *CPUBackend, chunks []*tensor.RawTensor, dim int, data []float32, shape []int) {
	t.Helper()
	got := be.Cat(chunks, dim).AsFloat32()
	if len(got) != len(data) {
		t.Fatalf("shape %v dim %d: cat len %d want %d", shape, dim, len(got), len(data))
	}
	for i := range data {
		if got[i] != data[i] {
			t.Fatalf("shape %v dim %d: round-trip [%d]=%v want %v", shape, dim, i, got[i], data[i])
		}
	}
}
