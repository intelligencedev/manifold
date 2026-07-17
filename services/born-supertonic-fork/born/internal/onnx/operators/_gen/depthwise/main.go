// Command depthwise generates the vendored AVX2+FMA float32 3x3 depthwise kernel
// for the operators package. Run via `go generate` (see depthwise_simd_amd64.go);
// lives in a separate module (_gen/depthwise/go.mod) so avo never enters born's
// module graph. The generated artifacts (depthwise_simd_amd64.s and its Go stub)
// are committed.
//
// depthwise3x3PlaneAVX2 computes one stride=1, padding=0 3x3 depthwise plane:
// out[oh*wOut + j .. +8] = sum over the nine taps weight[kh*3+kw] of
// weight[kh*3+kw] * in[(oh+kh)*wp + j+kw .. +8], for the first w8 (multiple of 8)
// output columns of each of hOut rows. Each input row gets its own accumulator so
// the three 3-tap chains run independently; the Go caller broadcasts the channel
// weights, loops planes, and fills the sub-8 column tail.
package main

import (
	. "github.com/mmcloughlin/avo/build"
	. "github.com/mmcloughlin/avo/operand"
	"github.com/mmcloughlin/avo/reg"
)

func main() {
	TEXT("depthwise3x3PlaneAVX2", NOSPLIT, "func(out, in, weight []float32, wp, wOut, hOut, w8 int)")
	Doc("depthwise3x3PlaneAVX2 computes one stride=1, padding=0 3x3 depthwise plane:",
		"out[oh*wOut+j..] = sum over the nine taps of weight[k]*in[(oh+kh)*wp + j+kw ..],",
		"for the first w8 (multiple of 8) output columns of each of hOut rows. The Go",
		"caller fills the sub-8 column tail and loops planes.")
	Pragma("noescape")

	outBase := Load(Param("out").Base(), GP64())
	inBase := Load(Param("in").Base(), GP64())
	wPtr := Load(Param("weight").Base(), GP64())
	wp := Load(Param("wp"), GP64())
	wOut := Load(Param("wOut"), GP64())
	hOut := Load(Param("hOut"), GP64())
	w8 := Load(Param("w8"), GP64())

	// Broadcast the nine taps into persistent YMM registers.
	w := make([]reg.VecVirtual, 9)
	for i := 0; i < 9; i++ {
		w[i] = YMM()
		VBROADCASTSS(Mem{Base: wPtr, Disp: i * 4}, w[i])
	}

	// Byte strides for one input / output row.
	wpB := GP64()
	MOVQ(wp, wpB)
	SHLQ(Imm(2), wpB) // wp * 4 bytes
	woutB := GP64()
	MOVQ(wOut, woutB)
	SHLQ(Imm(2), woutB) // wOut * 4 bytes

	// Running row pointers, advanced once per output row.
	inRow := GP64()
	MOVQ(inBase, inRow)
	outRow := GP64()
	MOVQ(outBase, outRow)

	oh := GP64()
	MOVQ(hOut, oh)

	Label("rowLoop")
	CMPQ(oh, Imm(0))
	JE(LabelRef("done"))

	// Column pointers into the three input rows and the output row.
	p0 := GP64()
	MOVQ(inRow, p0)
	p1 := GP64()
	LEAQ(Mem{Base: inRow, Index: wpB, Scale: 1}, p1) // inRow + wpB
	p2 := GP64()
	LEAQ(Mem{Base: p1, Index: wpB, Scale: 1}, p2) // inRow + 2*wpB
	pout := GP64()
	MOVQ(outRow, pout)

	blocks := GP64()
	MOVQ(w8, blocks)
	SHRQ(Imm(3), blocks) // w8 / 8 vector blocks per row

	Label("colLoop")
	CMPQ(blocks, Imm(0))
	JE(LabelRef("colDone"))

	a0, a1, a2 := YMM(), YMM(), YMM()
	t := YMM()
	// Input row 0: taps (0,0) (0,1) (0,2).
	VMOVUPS(Mem{Base: p0}, t)
	VMULPS(t, w[0], a0)
	VMOVUPS(Mem{Base: p0, Disp: 4}, t)
	VFMADD231PS(t, w[1], a0)
	VMOVUPS(Mem{Base: p0, Disp: 8}, t)
	VFMADD231PS(t, w[2], a0)
	// Input row 1: taps (1,0) (1,1) (1,2).
	VMOVUPS(Mem{Base: p1}, t)
	VMULPS(t, w[3], a1)
	VMOVUPS(Mem{Base: p1, Disp: 4}, t)
	VFMADD231PS(t, w[4], a1)
	VMOVUPS(Mem{Base: p1, Disp: 8}, t)
	VFMADD231PS(t, w[5], a1)
	// Input row 2: taps (2,0) (2,1) (2,2).
	VMOVUPS(Mem{Base: p2}, t)
	VMULPS(t, w[6], a2)
	VMOVUPS(Mem{Base: p2, Disp: 4}, t)
	VFMADD231PS(t, w[7], a2)
	VMOVUPS(Mem{Base: p2, Disp: 8}, t)
	VFMADD231PS(t, w[8], a2)
	// Reduce the three row accumulators and store.
	VADDPS(a1, a0, a0)
	VADDPS(a2, a0, a0)
	VMOVUPS(a0, Mem{Base: pout})

	ADDQ(Imm(32), p0)
	ADDQ(Imm(32), p1)
	ADDQ(Imm(32), p2)
	ADDQ(Imm(32), pout)
	DECQ(blocks)
	JMP(LabelRef("colLoop"))

	Label("colDone")
	ADDQ(wpB, inRow)
	ADDQ(woutB, outRow)
	DECQ(oh)
	JMP(LabelRef("rowLoop"))

	Label("done")
	VZEROUPPER()
	RET()

	Generate()
}
