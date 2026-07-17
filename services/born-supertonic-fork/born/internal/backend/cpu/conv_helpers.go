package cpu

import "sync"

// colBufTPool reuses the transposed im2col buffer (colBuf^T) fed to the SIMD GEMM
// across convolutions. The buffer is fully overwritten by transposeF32 every call,
// so a pooled (un-zeroed) buffer is safe and avoids a large alloc + zero per conv.
var colBufTPool = sync.Pool{New: func() any { s := []float32(nil); return &s }}

// The im2col conv path (conv2dFloat32 / conv2dFloat64) recycles two more large
// ephemeral buffers per call: convColPool* holds the im2col column buffer, and
// convOutPool* holds the matmul output that feeds the NCHW rearrange. Both are
// fully overwritten before any read (im2col writes every column entry, including
// padding zeros; matMulColBuf* writes every output[i*colHeight+j]), so a pooled
// un-zeroed buffer is safe and skips a make + memclr per conv. Recycling the
// matmul output also removes the old copy(outputData -> tempBuf) memmove, because
// the matmul writes the scratch directly and rearrange then permutes it into the
// output. Mirrors colBufTPool above and the gemmScratch pool from the GEMM kernel.
var (
	convColPoolF32 = sync.Pool{New: func() any { s := []float32(nil); return &s }}
	convOutPoolF32 = sync.Pool{New: func() any { s := []float32(nil); return &s }}
	convColPoolF64 = sync.Pool{New: func() any { s := []float64(nil); return &s }}
	convOutPoolF64 = sync.Pool{New: func() any { s := []float64(nil); return &s }}
)

// poolScratch returns a length-n slice backed by a pooled array from p, growing
// the array only when the cached capacity is too small. The slice length is
// exactly n (no backing-slice slack leaks into indexing), and its contents are
// NOT zeroed: callers must fully overwrite the slice before reading it, then
// return the pointer to p with Put.
func poolScratch[T any](p *sync.Pool, n int) *[]T {
	sp := p.Get().(*[]T)
	if cap(*sp) < n {
		*sp = make([]T, n)
	} else {
		*sp = (*sp)[:n]
	}
	return sp
}

// conv_helpers.go — inner-loop helper functions for Conv2D and MaxPool2D.
//
// These helpers are extracted from the innermost loops to reduce cognitive
// complexity while staying small enough for the compiler to inline them.
// Following Burn (Rust) patterns: pure data-slice helpers, no RawTensor args.

// matMulColBufFloat32 performs the im2col matmul step for float32.
//
// Computes output[i*colHeight+j] = sum_k kernel[i*colWidth+k] * col[j*colWidth+k]
// for all i in [0, cOut) and j in [0, colHeight).
func matMulColBufFloat32(outputData, kernelData, colBuf []float32, cOut, colHeight, colWidth int) {
	// SIMD fast path: out = kernel[cOut,colWidth] @ colBuf^T[colWidth,colHeight].
	// Transpose colBuf so the reduction axis becomes the row axis, then reuse the
	// vendored GEMM kernel. In gemmF32(c, a, b, m, k, n) terms: m=cOut, k=colWidth,
	// n=colHeight. gemmMinCols is the minimum n for a full 16-wide column tile.
	// Tiny depthwise-style calls (cOut=1, small colHeight) stay scalar.
	if gemmF32 != nil && colHeight >= gemmMinCols && cOut*colWidth*colHeight >= blockThreshold {
		p := poolScratch[float32](&colBufTPool, colWidth*colHeight)
		defer colBufTPool.Put(p)
		colBufT := *p
		transposeF32(colBufT, colBuf, colHeight, colWidth) // fully overwrites colBufT
		gemmF32(outputData, kernelData, colBufT, cOut, colWidth, colHeight)
		return
	}
	for i := 0; i < cOut; i++ {
		kernelRow := kernelData[i*colWidth : i*colWidth+colWidth]
		for j := 0; j < colHeight; j++ {
			colRow := colBuf[j*colWidth : j*colWidth+colWidth]
			sum := float32(0.0)
			for k := 0; k < colWidth; k++ {
				sum += kernelRow[k] * colRow[k]
			}
			outputData[i*colHeight+j] = sum
		}
	}
}

// matMulColBufFloat64 performs the im2col matmul step for float64.
//
// Computes output[i*colHeight+j] = sum_k kernel[i*colWidth+k] * col[j*colWidth+k]
// for all i in [0, cOut) and j in [0, colHeight).
func matMulColBufFloat64(outputData, kernelData, colBuf []float64, cOut, colHeight, colWidth int) {
	for i := 0; i < cOut; i++ {
		kernelRow := kernelData[i*colWidth : i*colWidth+colWidth]
		for j := 0; j < colHeight; j++ {
			colRow := colBuf[j*colWidth : j*colWidth+colWidth]
			sum := float64(0.0)
			for k := 0; k < colWidth; k++ {
				sum += kernelRow[k] * colRow[k]
			}
			outputData[i*colHeight+j] = sum
		}
	}
}

// rearrangeOutputFloat32 copies from [cOut, n*hOut*wOut] layout to [n, cOut, hOut, wOut].
//
// src (tempBuf) holds data in [c, n*hOut*wOut + h*wOut + w] order.
// dst (outputData) receives data in [n, c, h, w] order.
func rearrangeOutputFloat32(outputData, tempBuf []float32, n, cOut, hOut, wOut, colHeight int) {
	for ni := 0; ni < n; ni++ {
		nSpatial := ni * hOut * wOut
		nBase := ni * cOut * hOut * wOut
		for c := 0; c < cOut; c++ {
			cColBase := c * colHeight
			cOutBase := nBase + c*hOut*wOut
			for h := 0; h < hOut; h++ {
				hBase := h * wOut
				for w := 0; w < wOut; w++ {
					outputData[cOutBase+hBase+w] = tempBuf[cColBase+nSpatial+hBase+w]
				}
			}
		}
	}
}

// rearrangeOutputFloat64 copies from [cOut, n*hOut*wOut] layout to [n, cOut, hOut, wOut].
//
// src (tempBuf) holds data in [c, n*hOut*wOut + h*wOut + w] order.
// dst (outputData) receives data in [n, c, h, w] order.
func rearrangeOutputFloat64(outputData, tempBuf []float64, n, cOut, hOut, wOut, colHeight int) {
	for ni := 0; ni < n; ni++ {
		nSpatial := ni * hOut * wOut
		nBase := ni * cOut * hOut * wOut
		for c := 0; c < cOut; c++ {
			cColBase := c * colHeight
			cOutBase := nBase + c*hOut*wOut
			for h := 0; h < hOut; h++ {
				hBase := h * wOut
				for w := 0; w < wOut; w++ {
					outputData[cOutBase+hBase+w] = tempBuf[cColBase+nSpatial+hBase+w]
				}
			}
		}
	}
}

// accumulateInputGradFloat32 distributes a single gradient value to input gradient positions.
//
// For each kernel offset (kh, kw), computes the input position and accumulates
// gradVal * kernel[kh*kw+kw] into the input gradient slice.
// Bounds are checked against inputH and inputW; out-of-bounds positions are skipped (padding).
func accumulateInputGradFloat32(
	inputGradCIn, kernelCIn []float32,
	gradVal float32,
	kh, kw int,
	outH, outW int,
	inputH, inputW int,
	stride, padding int,
) {
	for dkh := 0; dkh < kh; dkh++ {
		hPos := outH*stride - padding + dkh
		if hPos < 0 || hPos >= inputH {
			continue
		}
		hBase := hPos * inputW
		kBase := dkh * kw
		for dkw := 0; dkw < kw; dkw++ {
			wPos := outW*stride - padding + dkw
			if wPos < 0 || wPos >= inputW {
				continue
			}
			inputGradCIn[hBase+wPos] += gradVal * kernelCIn[kBase+dkw]
		}
	}
}

// accumulateInputGradFloat64 distributes a single gradient value to input gradient positions.
//
// For each kernel offset (kh, kw), computes the input position and accumulates
// gradVal * kernel[kh*kw+kw] into the input gradient slice.
// Bounds are checked against inputH and inputW; out-of-bounds positions are skipped (padding).
func accumulateInputGradFloat64(
	inputGradCIn, kernelCIn []float64,
	gradVal float64,
	kh, kw int,
	outH, outW int,
	inputH, inputW int,
	stride, padding int,
) {
	for dkh := 0; dkh < kh; dkh++ {
		hPos := outH*stride - padding + dkh
		if hPos < 0 || hPos >= inputH {
			continue
		}
		hBase := hPos * inputW
		kBase := dkh * kw
		for dkw := 0; dkw < kw; dkw++ {
			wPos := outW*stride - padding + dkw
			if wPos < 0 || wPos >= inputW {
				continue
			}
			inputGradCIn[hBase+wPos] += gradVal * kernelCIn[kBase+dkw]
		}
	}
}

// accumulateInputGradFloat32Stride1NoPad distributes gradient with stride=1, padding=0.
//
// Simplified version for the stride=1, padding=0 fast path — no bounds checks needed.
func accumulateInputGradFloat32Stride1NoPad(
	inputGradCIn, kernelCIn []float32,
	gradVal float32,
	kh, kw int,
	outH, outW, inputW int,
) {
	for dkh := 0; dkh < kh; dkh++ {
		hBase := (outH + dkh) * inputW
		kBase := dkh * kw
		for dkw := 0; dkw < kw; dkw++ {
			inputGradCIn[hBase+outW+dkw] += gradVal * kernelCIn[kBase+dkw]
		}
	}
}

// accumulateInputGradFloat64Stride1NoPad distributes gradient with stride=1, padding=0.
//
// Simplified version for the stride=1, padding=0 fast path — no bounds checks needed.
func accumulateInputGradFloat64Stride1NoPad(
	inputGradCIn, kernelCIn []float64,
	gradVal float64,
	kh, kw int,
	outH, outW, inputW int,
) {
	for dkh := 0; dkh < kh; dkh++ {
		hBase := (outH + dkh) * inputW
		kBase := dkh * kw
		for dkw := 0; dkw < kw; dkw++ {
			inputGradCIn[hBase+outW+dkw] += gradVal * kernelCIn[kBase+dkw]
		}
	}
}

// kernelGradSumFloat32 accumulates the gradient contribution for one kernel weight position.
//
// Sums inputData[n, cIn, h, w] * gradData[n, cOut, outH, outW] over all valid
// (n, outH, outW) positions. Returns the accumulated sum.
// h = outH*stride - padding + kh; w = outW*stride - padding + kw; bounds checked.
//
//nolint:dupl // Intentional duplication: identical structure to kernelGradSumFloat64, different numeric type.
func kernelGradSumFloat32(
	inputData, gradData []float32,
	n, cIn, h, w, cOut, hOut, wOut int,
	cOutIdx, cInIdx, kh, kw int,
	stride, padding int,
) float32 {
	sum := float32(0.0)
	for ni := 0; ni < n; ni++ {
		inputNBase := ni * cIn * h * w
		gradNBase := ni * cOut * hOut * wOut
		for outH := 0; outH < hOut; outH++ {
			hPos := outH*stride - padding + kh
			if hPos < 0 || hPos >= h {
				continue
			}
			gradOutHBase := gradNBase + cOutIdx*hOut*wOut + outH*wOut
			inputHBase := inputNBase + cInIdx*h*w + hPos*w
			for outW := 0; outW < wOut; outW++ {
				wPos := outW*stride - padding + kw
				if wPos < 0 || wPos >= w {
					continue
				}
				sum += inputData[inputHBase+wPos] * gradData[gradOutHBase+outW]
			}
		}
	}
	return sum
}

// kernelGradSumFloat64 accumulates the gradient contribution for one kernel weight position.
//
// Sums inputData[n, cIn, h, w] * gradData[n, cOut, outH, outW] over all valid
// (n, outH, outW) positions. Returns the accumulated sum.
// h = outH*stride - padding + kh; w = outW*stride - padding + kw; bounds checked.
//
//nolint:dupl // Intentional duplication: identical structure to kernelGradSumFloat32, different numeric type.
func kernelGradSumFloat64(
	inputData, gradData []float64,
	n, cIn, h, w, cOut, hOut, wOut int,
	cOutIdx, cInIdx, kh, kw int,
	stride, padding int,
) float64 {
	sum := float64(0.0)
	for ni := 0; ni < n; ni++ {
		inputNBase := ni * cIn * h * w
		gradNBase := ni * cOut * hOut * wOut
		for outH := 0; outH < hOut; outH++ {
			hPos := outH*stride - padding + kh
			if hPos < 0 || hPos >= h {
				continue
			}
			gradOutHBase := gradNBase + cOutIdx*hOut*wOut + outH*wOut
			inputHBase := inputNBase + cInIdx*h*w + hPos*w
			for outW := 0; outW < wOut; outW++ {
				wPos := outW*stride - padding + kw
				if wPos < 0 || wPos >= w {
					continue
				}
				sum += inputData[inputHBase+wPos] * gradData[gradOutHBase+outW]
			}
		}
	}
	return sum
}

// kernelGradSumFloat32Stride1NoPad accumulates kernel gradient for stride=1, padding=0.
//
// Simplified fast path: no bounds checks, h = outH+kh, w = outW+kw.
func kernelGradSumFloat32Stride1NoPad(
	inputData, gradData []float32,
	n, cIn, h, w, cOut, hOut, wOut int,
	cOutIdx, cInIdx, kh, kw int,
) float32 {
	sum := float32(0.0)
	for ni := 0; ni < n; ni++ {
		inputNBase := ni * cIn * h * w
		gradNBase := ni * cOut * hOut * wOut
		for outH := 0; outH < hOut; outH++ {
			gradOutHBase := gradNBase + cOutIdx*hOut*wOut + outH*wOut
			inputHBase := inputNBase + cInIdx*h*w + (outH+kh)*w
			for outW := 0; outW < wOut; outW++ {
				sum += inputData[inputHBase+outW+kw] * gradData[gradOutHBase+outW]
			}
		}
	}
	return sum
}

// kernelGradSumFloat64Stride1NoPad accumulates kernel gradient for stride=1, padding=0.
//
// Simplified fast path: no bounds checks, h = outH+kh, w = outW+kw.
func kernelGradSumFloat64Stride1NoPad(
	inputData, gradData []float64,
	n, cIn, h, w, cOut, hOut, wOut int,
	cOutIdx, cInIdx, kh, kw int,
) float64 {
	sum := float64(0.0)
	for ni := 0; ni < n; ni++ {
		inputNBase := ni * cIn * h * w
		gradNBase := ni * cOut * hOut * wOut
		for outH := 0; outH < hOut; outH++ {
			gradOutHBase := gradNBase + cOutIdx*hOut*wOut + outH*wOut
			inputHBase := inputNBase + cInIdx*h*w + (outH+kh)*w
			for outW := 0; outW < wOut; outW++ {
				sum += inputData[inputHBase+outW+kw] * gradData[gradOutHBase+outW]
			}
		}
	}
	return sum
}

// poolWindowMaxFloat32 finds the maximum value in a pooling window for float32.
//
// channelData is a pre-sliced [inputH*inputW] plane. hStart, wStart are the window origin.
// kernelSize is the window side length. inputW is the input width (for row indexing).
func poolWindowMaxFloat32(channelData []float32, hStart, wStart, kernelSize, inputW int) float32 {
	maxVal := float32(-1e38)
	rowStart := hStart * inputW
	for kh := 0; kh < kernelSize; kh++ {
		// Pre-slice row once per kh to eliminate per-kw bounds check.
		rowData := channelData[rowStart : rowStart+inputW]
		for kw := 0; kw < kernelSize; kw++ {
			if v := rowData[wStart+kw]; v > maxVal {
				maxVal = v
			}
		}
		rowStart += inputW
	}
	return maxVal
}

// poolWindowMaxFloat64 finds the maximum value in a pooling window for float64.
//
// channelData is a pre-sliced [inputH*inputW] plane. hStart, wStart are the window origin.
// kernelSize is the window side length. inputW is the input width (for row indexing).
func poolWindowMaxFloat64(channelData []float64, hStart, wStart, kernelSize, inputW int) float64 {
	maxVal := float64(-1e308)
	rowStart := hStart * inputW
	for kh := 0; kh < kernelSize; kh++ {
		// Pre-slice row once per kh to eliminate per-kw bounds check.
		rowData := channelData[rowStart : rowStart+inputW]
		for kw := 0; kw < kernelSize; kw++ {
			if v := rowData[wStart+kw]; v > maxVal {
				maxVal = v
			}
		}
		rowStart += inputW
	}
	return maxVal
}
