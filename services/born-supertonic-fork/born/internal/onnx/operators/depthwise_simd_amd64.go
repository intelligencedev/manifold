//go:build amd64

package operators

//go:generate sh -c "cd _gen/depthwise && go run . -out ../../depthwise_simd_amd64.s -stubs ../../depthwise_simd_stub_gen_amd64.go -pkg operators"

import "golang.org/x/sys/cpu"

// init wires the vendored AVX2+FMA 3x3 depthwise kernel into the dispatch whenever
// the CPU supports AVX2+FMA. It compiles into every default amd64 build (no build
// tag or env flag); dispatch is decided here at startup from runtime CPU
// detection. CPUs without AVX2+FMA leave depthwise3x3F32 nil and use the scalar
// path.
func init() {
	if cpu.X86.HasAVX2 && cpu.X86.HasFMA {
		depthwise3x3F32 = depthwise3x3Stride1AVX2
	}
}

// depthwise3x3Stride1AVX2 runs the stride=1, padding=0 3x3 depthwise convolution.
// For each (n,c) plane it feeds the channel's nine taps to the vendored kernel,
// which vectorizes the first wOut&^7 output columns of every row, then finishes
// the sub-8 column tail with the scalar nine-tap sum. The input is already padded
// by the caller, so this runs at padding 0.
//
// Precondition (guaranteed by the 3x3/stride=1 dispatch in
// depthwiseConvForward3x3Float32): hp == hOut+2 and wp == wOut+2. The vendored
// kernel's last-row, last-column 8-wide load reaches exactly the final element of
// each plane, so a caller passing a smaller hp/wp would read out of bounds.
func depthwise3x3Stride1AVX2(out, in, weight []float32, n, c, hp, wp, hOut, wOut int) {
	planeIn := hp * wp
	planeOut := hOut * wOut
	wMain := wOut &^ 7 // output columns handled by the vector kernel (multiple of 8)
	for plane := 0; plane < n*c; plane++ {
		inBase := plane * planeIn
		outBase := plane * planeOut
		wBase := (plane % c) * 9
		wch := weight[wBase : wBase+9]

		if wMain > 0 {
			depthwise3x3PlaneAVX2(out[outBase:], in[inBase:], wch, wp, wOut, hOut, wMain)
		}
		if wMain == wOut {
			continue // wOut is a multiple of 8: the aligned blocks cover every column.
		}
		if wOut >= 8 {
			// Cover the trailing wOut%8 columns with one more vector block starting
			// at wOut-8. It overlaps [wOut-8, wMain) and recomputes those columns to
			// the same values (the kernel is a pure function of the input), which is
			// far cheaper than a scalar remainder loop and keeps every output column
			// on the vector path. The offset load stays in bounds: it still reaches
			// exactly the final element of the plane (see the precondition above).
			off := wOut - 8
			depthwise3x3PlaneAVX2(out[outBase+off:], in[inBase+off:], wch, wp, wOut, hOut, 8)
			continue
		}

		// wOut < 8: too narrow for a vector block, so use the scalar nine-tap sum.
		w0, w1, w2 := wch[0], wch[1], wch[2]
		w3, w4, w5 := wch[3], wch[4], wch[5]
		w6, w7, w8 := wch[6], wch[7], wch[8]
		for oh := 0; oh < hOut; oh++ {
			r0 := inBase + oh*wp
			r1 := r0 + wp
			r2 := r1 + wp
			outRow := outBase + oh*wOut
			for ow := 0; ow < wOut; ow++ {
				out[outRow+ow] = in[r0+ow]*w0 + in[r0+ow+1]*w1 + in[r0+ow+2]*w2 +
					in[r1+ow]*w3 + in[r1+ow+1]*w4 + in[r1+ow+2]*w5 +
					in[r2+ow]*w6 + in[r2+ow+1]*w7 + in[r2+ow+2]*w8
			}
		}
	}
}
