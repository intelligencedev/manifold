//go:build !wasm

package operators

import (
	"fmt"
	"math"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/born-ml/born/internal/tensor"
)

// conv1DParallelThreshold is the minimum multiply-accumulate count below which
// Conv1d runs serially (goroutine overhead would dominate for tiny convs).
const conv1DParallelThreshold = 1 << 15

// conv1DMinMACsPerWorker targets this many multiply-accumulates per goroutine so
// small convs spawn fewer workers and avoid excessive spawn/barrier overhead.
const conv1DMinMACsPerWorker = 1 << 16

// conv1DTileFloats caps the per-job im2col scratch tile (in float32 elements,
// so 1<<20 = 4 MB) to keep column buffers cache-resident and bound peak memory
// for long-sequence convs (the vocoder runs Conv1d at ~60k samples).
const conv1DTileFloats = 1 << 20

var conv1DScratchPool sync.Pool

func conv1DScratchGet(size int) *[]float32 {
	if p, _ := conv1DScratchPool.Get().(*[]float32); p != nil && cap(*p) >= size {
		*p = (*p)[:size]
		return p
	}
	s := make([]float32, size)
	return &s
}

// im2colTile builds the transposed column buffer for one (batch, group, column
// tile): btile[icg*kL+k][j] = x[ic][ (jLo+j)*stride - padBegin + k*dilation ],
// with zeros where the index falls in padding. Row r matches the layout of the
// weight's reduction axis, so out = W_g @ btile is the conv output tile. All
// bounds checks are hoisted out of the inner loops: each row is a contiguous
// copy (stride 1) or strided gather over its precomputed valid range.
func im2colTile(btile, xd []float32, ni, g, cinPerG, l, xCL, kL, tw, jLo, stride, dilation, padBegin int) {
	for icg := 0; icg < cinPerG; icg++ {
		xRow := xd[ni*xCL+(g*cinPerG+icg)*l:][:l]
		for k := 0; k < kL; k++ {
			row := btile[(icg*kL+k)*tw:][:tw]
			base := jLo*stride - padBegin + k*dilation
			// Valid j range: 0 <= base + j*stride <= l-1.
			jLoV := 0
			if base < 0 {
				jLoV = (-base + stride - 1) / stride
			}
			jHiV := 0
			if last := l - 1 - base; last >= 0 {
				jHiV = last/stride + 1
			}
			if jHiV > tw {
				jHiV = tw
			}
			if jLoV > jHiV {
				jLoV = jHiV
			}
			for q := 0; q < jLoV; q++ {
				row[q] = 0
			}
			if stride == 1 {
				copy(row[jLoV:jHiV], xRow[base+jLoV:base+jHiV])
			} else {
				il := base + jLoV*stride
				for q := jLoV; q < jHiV; q++ {
					row[q] = xRow[il]
					il += stride
				}
			}
			for q := jHiV; q < tw; q++ {
				row[q] = 0
			}
		}
	}
}

type conv1DJob struct{ ni, g, jLo, jHi, iLo, iHi int }

// conv1DMatMulPath computes Conv1d as a per-(batch, group) matmul over im2col
// column tiles: out_g = W_g[coutPerG × kk] @ btile[kk × tileWidth], kk =
// cinPerG*kL. Work is decomposed into (batch, group, column-tile, row-chunk)
// jobs pulled by a fixed worker pool; each job owns a disjoint output region,
// builds its own column tile (pooled scratch, rebuilt per row-chunk — the
// rebuild is O(kk·tw) against the job's O(rows·kk·tw) matmul), and runs a
// 4-row axpy kernel whose inner loops are branch-free and contiguous. This
// replaces the naive gather loop's per-MAC bounds check with streaming FMA-able
// code while keeping full multicore utilization (Born's own MatMul is
// single-threaded and its SIMD GEMM is amd64-only, so it is not reused here).
func conv1DMatMulPath(od, xd, wd, bias []float32, n, cin, l, cout, kL, lout, stride, dilation, group, padBegin int) {
	cinPerG := cin / group
	coutPerG := cout / group
	kk := cinPerG * kL
	xCL := cin * l

	tw := lout
	if maxTW := conv1DTileFloats / kk; tw > maxTW {
		tw = maxTW
		if tw < 32 {
			tw = 32
		}
	}
	nTiles := (lout + tw - 1) / tw

	workers := runtime.GOMAXPROCS(0)
	baseJobs := n * group * nTiles
	iChunks := 1
	if baseJobs < workers*2 {
		iChunks = (workers*2 + baseJobs - 1) / baseJobs
		if maxI := (coutPerG + 7) / 8; iChunks > maxI {
			iChunks = maxI
		}
		if iChunks < 1 {
			iChunks = 1
		}
	}
	iChunkSize := (coutPerG + iChunks - 1) / iChunks

	jobs := make([]conv1DJob, 0, baseJobs*iChunks)
	for ni := 0; ni < n; ni++ {
		for g := 0; g < group; g++ {
			for t := 0; t < nTiles; t++ {
				jLo := t * tw
				jHi := min(jLo+tw, lout)
				for c := 0; c < iChunks; c++ {
					iLo := c * iChunkSize
					iHi := min(iLo+iChunkSize, coutPerG)
					if iLo >= iHi {
						break
					}
					jobs = append(jobs, conv1DJob{ni, g, jLo, jHi, iLo, iHi})
				}
			}
		}
	}

	runJob := func(j conv1DJob) {
		twj := j.jHi - j.jLo
		sp := conv1DScratchGet(kk * twj)
		defer conv1DScratchPool.Put(sp)
		btile := *sp
		im2colTile(btile, xd, j.ni, j.g, cinPerG, l, xCL, kL, twj, j.jLo, stride, dilation, padBegin)

		row := func(i int) []float32 {
			oc := j.g*coutPerG + i
			return od[(j.ni*cout+oc)*lout+j.jLo:][:twj]
		}
		wRow := func(i int) []float32 {
			oc := j.g*coutPerG + i
			return wd[oc*kk:][:kk]
		}
		biasAt := func(i int) float32 {
			if bias == nil {
				return 0
			}
			return bias[j.g*coutPerG+i]
		}
		fill := func(dst []float32, v float32) {
			for q := range dst {
				dst[q] = v
			}
		}

		i := j.iLo
		for ; i+4 <= j.iHi; i += 4 {
			c0, c1, c2, c3 := row(i), row(i+1), row(i+2), row(i+3)
			w0, w1, w2, w3 := wRow(i), wRow(i+1), wRow(i+2), wRow(i+3)
			// 4×4 register-tiled kernel: 16 accumulators live in FP registers
			// across the whole k-loop, so C is written once per tile instead of
			// loaded+stored on every reduction step (8 loads per 16 FMAs).
			b0, b1, b2, b3 := biasAt(i), biasAt(i+1), biasAt(i+2), biasAt(i+3)
			q := 0
			for ; q+4 <= twj; q += 4 {
				s00, s01, s02, s03 := b0, b0, b0, b0
				s10, s11, s12, s13 := b1, b1, b1, b1
				s20, s21, s22, s23 := b2, b2, b2, b2
				s30, s31, s32, s33 := b3, b3, b3, b3
				for p := 0; p < kk; p++ {
					bt := btile[p*twj+q:]
					v0, v1, v2, v3 := bt[0], bt[1], bt[2], bt[3]
					a0, a1, a2, a3 := w0[p], w1[p], w2[p], w3[p]
					s00 += a0 * v0
					s01 += a0 * v1
					s02 += a0 * v2
					s03 += a0 * v3
					s10 += a1 * v0
					s11 += a1 * v1
					s12 += a1 * v2
					s13 += a1 * v3
					s20 += a2 * v0
					s21 += a2 * v1
					s22 += a2 * v2
					s23 += a2 * v3
					s30 += a3 * v0
					s31 += a3 * v1
					s32 += a3 * v2
					s33 += a3 * v3
				}
				c0[q], c0[q+1], c0[q+2], c0[q+3] = s00, s01, s02, s03
				c1[q], c1[q+1], c1[q+2], c1[q+3] = s10, s11, s12, s13
				c2[q], c2[q+1], c2[q+2], c2[q+3] = s20, s21, s22, s23
				c3[q], c3[q+1], c3[q+2], c3[q+3] = s30, s31, s32, s33
			}
			// Column tail (twj % 4): scalar accumulators per column.
			for ; q < twj; q++ {
				s0, s1, s2, s3 := b0, b1, b2, b3
				for p := 0; p < kk; p++ {
					bv := btile[p*twj+q]
					s0 += w0[p] * bv
					s1 += w1[p] * bv
					s2 += w2[p] * bv
					s3 += w3[p] * bv
				}
				c0[q], c1[q], c2[q], c3[q] = s0, s1, s2, s3
			}
		}
		for ; i < j.iHi; i++ {
			c0 := row(i)
			w0 := wRow(i)
			fill(c0, biasAt(i))
			for p := 0; p < kk; p++ {
				b := btile[p*twj:][:twj]
				a0 := w0[p]
				for q, bv := range b {
					c0[q] += a0 * bv
				}
			}
		}
	}

	if workers > len(jobs) {
		workers = len(jobs)
	}
	if workers <= 1 {
		for _, j := range jobs {
			runJob(j)
		}
		return
	}
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
				runJob(jobs[idx])
			}
		}()
	}
	wg.Wait()
}

// powInt64 implements ONNX Pow for int64 base/exponent (integer shape math).
// Exponent may be a scalar or match the base shape. Negative exponents yield 0
// (integer truncation).
func powInt64(base, expT *tensor.RawTensor) ([]*tensor.RawTensor, error) {
	b := base.AsInt64()
	e := expT.AsInt64()
	out, err := tensor.NewRaw(base.Shape(), tensor.Int64, base.Device())
	if err != nil {
		return nil, fmt.Errorf("pow int64: %w", err)
	}
	od := out.AsInt64()
	ipow := func(base, exp int64) int64 {
		if exp < 0 {
			return 0
		}
		r := int64(1)
		for ; exp > 0; exp-- {
			r *= base
		}
		return r
	}
	switch {
	case len(e) == 1:
		for i := range b {
			od[i] = ipow(b[i], e[0])
		}
	case base.Shape().Equal(expT.Shape()):
		for i := range b {
			od[i] = ipow(b[i], e[i])
		}
	default:
		return nil, fmt.Errorf("pow int64: exp shape %v incompatible with base %v", expT.Shape(), base.Shape())
	}
	return []*tensor.RawTensor{out}, nil
}

// clipInt64 implements ONNX Clip for int64 tensors. min/max come from the
// optional second and third inputs (opset 11+); absent bounds are unbounded.
func clipInt64(inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	data := inputs[0]
	lo := int64(math.MinInt64)
	hi := int64(math.MaxInt64)
	if len(inputs) >= 2 && inputs[1] != nil && inputs[1].NumElements() > 0 {
		lo = inputs[1].AsInt64()[0]
	}
	if len(inputs) >= 3 && inputs[2] != nil && inputs[2].NumElements() > 0 {
		hi = inputs[2].AsInt64()[0]
	}
	out, err := tensor.NewRaw(data.Shape(), tensor.Int64, data.Device())
	if err != nil {
		return nil, fmt.Errorf("clip int64: %w", err)
	}
	in := data.AsInt64()
	res := out.AsInt64()
	for i, v := range in {
		if v < lo {
			v = lo
		}
		if v > hi {
			v = hi
		}
		res[i] = v
	}
	return []*tensor.RawTensor{out}, nil
}

// registerSupertonicOps registers ONNX operators required by the Supertonic
// TTS graphs that are not covered by Born's base operator set:
// Pad, BatchNormalization, Cos, Sin, Reciprocal, ReduceSum, Softplus, Tile.
func (r *Registry) registerSupertonicOps() {
	r.Register("Pad", handlePad)
	r.Register("Sin", handleSin)
	r.Register("Cos", handleCos)
	r.Register("Reciprocal", handleReciprocal)
	r.Register("Softplus", handleSoftplus)
	r.Register("Tile", handleTile)
	r.Register("BatchNormalization", handleBatchNorm)
	r.Register("ReduceSum", handleReduceSum)
}

func handleReduceSum(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	return handleReduce(node, inputs, reduceSum)
}

func unaryFloat32(name string, in *tensor.RawTensor, fn func(float32) float32) ([]*tensor.RawTensor, error) {
	if in == nil {
		return nil, fmt.Errorf("%s: nil input", name)
	}
	if in.DType() != tensor.Float32 {
		return nil, fmt.Errorf("%s: only float32 supported, got %s", name, in.DType())
	}
	out, err := tensor.NewRaw(in.Shape(), tensor.Float32, in.Device())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	src, dst := in.AsFloat32(), out.AsFloat32()
	for i := range src {
		dst[i] = fn(src[i])
	}
	return []*tensor.RawTensor{out}, nil
}

func handleSin(_ *Context, _ *Node, in []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	return unaryFloat32("sin", in[0], func(x float32) float32 { return float32(math.Sin(float64(x))) })
}

func handleCos(_ *Context, _ *Node, in []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	return unaryFloat32("cos", in[0], func(x float32) float32 { return float32(math.Cos(float64(x))) })
}

func handleReciprocal(_ *Context, _ *Node, in []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	return unaryFloat32("reciprocal", in[0], func(x float32) float32 { return 1.0 / x })
}

func handleSoftplus(_ *Context, _ *Node, in []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	// log(1 + exp(x)), numerically stable for large x.
	return unaryFloat32("softplus", in[0], func(x float32) float32 {
		xf := float64(x)
		if xf > 20 {
			return x
		}
		return float32(math.Log1p(math.Exp(xf)))
	})
}

// handleTile repeats the input along each dimension per the int64 "repeats"
// second input (one entry per dim).
func handleTile(_ *Context, _ *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) < 2 || inputs[0] == nil || inputs[1] == nil {
		return nil, fmt.Errorf("tile: requires input and repeats")
	}
	x := inputs[0]
	if x.DType() != tensor.Float32 {
		return nil, fmt.Errorf("tile: only float32 supported, got %s", x.DType())
	}
	reps := inputs[1].AsInt64()
	inShape := x.Shape()
	if len(reps) != len(inShape) {
		return nil, fmt.Errorf("tile: repeats len %d != rank %d", len(reps), len(inShape))
	}
	outShape := make(tensor.Shape, len(inShape))
	for i := range inShape {
		outShape[i] = inShape[i] * int(reps[i])
	}
	out, err := tensor.NewRaw(outShape, tensor.Float32, x.Device())
	if err != nil {
		return nil, fmt.Errorf("tile: %w", err)
	}
	src, dst := x.AsFloat32(), out.AsFloat32()
	inStrides := rowMajorStrides(inShape)
	outStrides := rowMajorStrides(outShape)
	rank := len(inShape)
	total := len(dst)
	idx := make([]int, rank)
	for lin := 0; lin < total; lin++ {
		rem := lin
		srcOff := 0
		for i := 0; i < rank; i++ {
			idx[i] = rem / outStrides[i]
			rem %= outStrides[i]
			srcOff += (idx[i] % inShape[i]) * inStrides[i]
		}
		dst[lin] = src[srcOff]
	}
	return []*tensor.RawTensor{out}, nil
}

// handleBatchNorm implements ONNX BatchNormalization inference (spatial):
// y = (x - mean) / sqrt(var + eps) * scale + B, with per-channel params over
// axis 1. Inputs: X, scale, B, mean, var. Only Y is returned.
func handleBatchNorm(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) < 5 {
		return nil, fmt.Errorf("batchnorm: requires X, scale, B, mean, var")
	}
	x := inputs[0]
	if x.DType() != tensor.Float32 {
		return nil, fmt.Errorf("batchnorm: only float32 supported, got %s", x.DType())
	}
	eps := GetAttrFloat(node, "epsilon", 1e-5)
	scale := inputs[1].AsFloat32()
	bias := inputs[2].AsFloat32()
	mean := inputs[3].AsFloat32()
	variance := inputs[4].AsFloat32()

	shape := x.Shape()
	if len(shape) < 2 {
		return nil, fmt.Errorf("batchnorm: expected rank>=2, got %v", shape)
	}
	channels := shape[1]
	// Elements per channel = product of dims after axis 1.
	inner := 1
	for i := 2; i < len(shape); i++ {
		inner *= shape[i]
	}
	out, err := tensor.NewRaw(shape, tensor.Float32, x.Device())
	if err != nil {
		return nil, fmt.Errorf("batchnorm: %w", err)
	}
	src, dst := x.AsFloat32(), out.AsFloat32()

	// Precompute per-channel scale/shift.
	a := make([]float32, channels)
	b := make([]float32, channels)
	for c := 0; c < channels; c++ {
		inv := float32(1.0 / math.Sqrt(float64(variance[c]+eps)))
		a[c] = scale[c] * inv
		b[c] = bias[c] - mean[c]*a[c]
	}
	for i := range src {
		c := (i / inner) % channels
		dst[i] = src[i]*a[c] + b[c]
	}
	return []*tensor.RawTensor{out}, nil
}

// clipInt64 implements ONNX Clip for int64 tensors.

// handleConv1D implements ONNX Conv for 3D (NCL) tensors: 1-D convolution with
// stride, asymmetric padding, dilation, and grouped/depthwise support. Weight is
// [C_out, C_in/group, kL]; optional bias is [C_out]. Naive direct convolution
// (correctness first).
func handleConv1D(node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) < 2 || inputs[0] == nil || inputs[1] == nil {
		return nil, fmt.Errorf("conv1d: requires x and w inputs")
	}
	x, w := inputs[0], inputs[1]
	if x.DType() != tensor.Float32 || w.DType() != tensor.Float32 {
		return nil, fmt.Errorf("conv1d: only float32 supported")
	}
	xs, ws := x.Shape(), w.Shape()
	if len(xs) != 3 || len(ws) != 3 {
		return nil, fmt.Errorf("conv1d: expected 3D x and w, got %v %v", xs, ws)
	}
	n, cin, l := xs[0], xs[1], xs[2]
	cout, cinPerG, kL := ws[0], ws[1], ws[2]

	stride := 1
	if s := GetAttrInts(node, "strides"); len(s) >= 1 {
		stride = int(s[0])
	}
	dilation := 1
	if d := GetAttrInts(node, "dilations"); len(d) >= 1 {
		dilation = int(d[0])
	}
	group := int(GetAttrInt(node, "group", 1))
	padBegin, padEnd := 0, 0
	if p := GetAttrInts(node, "pads"); len(p) >= 2 {
		padBegin, padEnd = int(p[0]), int(p[1])
	}
	if GetAttrString(node, "auto_pad", autoPadNotset) == autoPadValid {
		padBegin, padEnd = 0, 0
	}

	if group < 1 || cin%group != 0 || cout%group != 0 {
		return nil, fmt.Errorf("conv1d: invalid group %d for Cin=%d Cout=%d", group, cin, cout)
	}
	if cinPerG != cin/group {
		return nil, fmt.Errorf("conv1d: weight Cin/group=%d, expected %d", cinPerG, cin/group)
	}
	if stride < 1 || dilation < 1 {
		return nil, fmt.Errorf("conv1d: invalid stride %d / dilation %d", stride, dilation)
	}

	lout := (l+padBegin+padEnd-dilation*(kL-1)-1)/stride + 1
	if lout < 0 {
		lout = 0
	}

	var bias []float32
	if len(inputs) >= 3 && inputs[2] != nil && inputs[2].NumElements() > 0 {
		bias = inputs[2].AsFloat32()
	}

	xd, wd := x.AsFloat32(), w.AsFloat32()
	out, err := tensor.NewRaw(tensor.Shape{n, cout, lout}, tensor.Float32, x.Device())
	if err != nil {
		return nil, fmt.Errorf("conv1d: %w", err)
	}
	od := out.AsFloat32()

	coutPerG := cout / group
	xCL := cin * l
	wKL := cinPerG * kL

	// Matmul fast path: convs with a nontrivial reduction axis (kk = cinPerG*kL)
	// run as im2col + blocked matmul — pointwise (kL=1) convs, the FLOP-heavy
	// ConvNeXt MLPs, reduce to a plain copy + matmul. Depthwise convs (kk ≤ 7)
	// and tiny convs stay on the direct path below, where matmul form doesn't
	// pay for its tile-building overhead.
	if fastMACs := n * cout * lout * wKL; lout > 0 && wKL >= 8 && coutPerG >= 8 &&
		fastMACs >= conv1DParallelThreshold {
		conv1DMatMulPath(od, xd, wd, bias, n, cin, l, cout, kL, lout, stride, dilation, group, padBegin)
		return []*tensor.RawTensor{out}, nil
	}

	// One output channel of one batch item is an independent unit of work with a
	// disjoint output slice; parallelize across the flattened (batch, channel)
	// space. computeRange handles indices [lo, hi) of that space.
	computeRange := func(lo, hi int) {
		for idx := lo; idx < hi; idx++ {
			ni := idx / cout
			oc := idx % cout
			g := oc / coutPerG
			var biasVal float32
			if bias != nil {
				biasVal = bias[oc]
			}
			outBase := (ni*cout + oc) * lout
			for ol := 0; ol < lout; ol++ {
				sum := biasVal
				base := ol*stride - padBegin
				for icg := 0; icg < cinPerG; icg++ {
					xoff := ni*xCL + (g*cinPerG+icg)*l
					woff := oc*wKL + icg*kL
					for k := 0; k < kL; k++ {
						il := base + k*dilation
						if il >= 0 && il < l {
							sum += xd[xoff+il] * wd[woff+k]
						}
					}
				}
				od[outBase+ol] = sum
			}
		}
	}

	units := n * cout
	macs := units * lout * cinPerG * kL
	// Size the worker count to the work available so small convs don't pay the
	// goroutine spawn/barrier cost for little gain (target ~conv1DMinMACsPerWorker
	// multiply-accumulates per goroutine), capped at GOMAXPROCS and unit count.
	workers := macs / conv1DMinMACsPerWorker
	if maxW := runtime.GOMAXPROCS(0); workers > maxW {
		workers = maxW
	}
	if workers > units {
		workers = units
	}
	if workers <= 1 || macs < conv1DParallelThreshold {
		computeRange(0, units)
		return []*tensor.RawTensor{out}, nil
	}

	var wg sync.WaitGroup
	chunk := (units + workers - 1) / workers
	for w := 0; w < workers; w++ {
		lo := w * chunk
		hi := lo + chunk
		if hi > units {
			hi = units
		}
		if lo >= hi {
			break
		}
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			computeRange(lo, hi)
		}(lo, hi)
	}
	wg.Wait()
	return []*tensor.RawTensor{out}, nil
}

// onnxBroadcastShape returns the numpy/ONNX bidirectional broadcast of two
// shapes (align from the right; each output dim is max where one side is 1 or
// they are equal). Used by Expand, which broadcasts input against its target
// shape rather than reshaping to it exactly.
func onnxBroadcastShape(a, b tensor.Shape) (tensor.Shape, error) {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	out := make(tensor.Shape, n)
	for i := 0; i < n; i++ {
		ad, bd := 1, 1
		if i < len(a) {
			ad = a[len(a)-1-i]
		}
		if i < len(b) {
			bd = b[len(b)-1-i]
		}
		switch {
		case ad == bd:
			out[n-1-i] = ad
		case ad == 1:
			out[n-1-i] = bd
		case bd == 1:
			out[n-1-i] = ad
		default:
			return nil, fmt.Errorf("incompatible broadcast dims %d and %d", ad, bd)
		}
	}
	return out, nil
}

func rowMajorStrides(shape tensor.Shape) []int {
	r := len(shape)
	strides := make([]int, r)
	acc := 1
	for i := r - 1; i >= 0; i-- {
		strides[i] = acc
		acc *= shape[i]
	}
	return strides
}

// reflectIndex mirrors numpy/ONNX "reflect" padding: reflect without repeating
// the edge element. period = 2*(dim-1).
func reflectIndex(p, dim int) int {
	if dim == 1 {
		return 0
	}
	period := 2 * (dim - 1)
	m := ((p % period) + period) % period
	if m >= dim {
		m = period - m
	}
	return m
}

// handlePad implements ONNX Pad (opset 11-19) for float32 tensors in
// "constant", "edge", and "reflect" modes. pads is the second input (int64,
// length 2*rank: [begin_0..begin_{r-1}, end_0..end_{r-1}]); an optional third
// input supplies the constant value.
func handlePad(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) < 2 || inputs[0] == nil || inputs[1] == nil {
		return nil, fmt.Errorf("pad: requires data and pads inputs")
	}
	data := inputs[0]
	if data.DType() != tensor.Float32 {
		return nil, fmt.Errorf("pad: only float32 supported, got %s", data.DType())
	}
	if inputs[1].DType() != tensor.Int64 {
		return nil, fmt.Errorf("pad: pads must be int64, got %s", inputs[1].DType())
	}
	pads := inputs[1].AsInt64()
	mode := GetAttrString(node, "mode", "constant")

	var cval float32
	if len(inputs) >= 3 && inputs[2] != nil && inputs[2].NumElements() > 0 {
		cval = inputs[2].AsFloat32()[0]
	}

	inShape := data.Shape()
	rank := len(inShape)
	if len(pads) != 2*rank {
		return nil, fmt.Errorf("pad: expected %d pad values for rank %d, got %d", 2*rank, rank, len(pads))
	}

	begins := make([]int, rank)
	outShape := make(tensor.Shape, rank)
	for i := 0; i < rank; i++ {
		begins[i] = int(pads[i])
		outShape[i] = inShape[i] + int(pads[i]) + int(pads[i+rank])
		if outShape[i] < 0 {
			return nil, fmt.Errorf("pad: negative output dim %d", outShape[i])
		}
	}

	out, err := tensor.NewRaw(outShape, tensor.Float32, data.Device())
	if err != nil {
		return nil, fmt.Errorf("pad: %w", err)
	}
	src := data.AsFloat32()
	dst := out.AsFloat32()

	inStrides := rowMajorStrides(inShape)
	outStrides := rowMajorStrides(outShape)

	total := 1
	for _, d := range outShape {
		total *= d
	}
	idx := make([]int, rank)
	for lin := 0; lin < total; lin++ {
		rem := lin
		for i := 0; i < rank; i++ {
			idx[i] = rem / outStrides[i]
			rem %= outStrides[i]
		}
		srcOff := 0
		isConst := false
		for i := 0; i < rank; i++ {
			p := idx[i] - begins[i]
			dim := inShape[i]
			switch mode {
			case "edge":
				if p < 0 {
					p = 0
				} else if p >= dim {
					p = dim - 1
				}
			case "reflect":
				p = reflectIndex(p, dim)
			default: // constant
				if p < 0 || p >= dim {
					isConst = true
				}
			}
			if isConst {
				break
			}
			srcOff += p * inStrides[i]
		}
		if isConst {
			dst[lin] = cval
		} else {
			dst[lin] = src[srcOff]
		}
	}
	return []*tensor.RawTensor{out}, nil
}
