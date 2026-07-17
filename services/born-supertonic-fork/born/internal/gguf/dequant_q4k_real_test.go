package gguf

// TestDequantizeQ4K_RealData verifies Q4_K dequantization against the GGML reference
// algorithm (ggml-quants.c dequantize_row_q4_K) applied to the actual first block of
// blk.0.attn_q.weight in TinyLlama-1.1B-Chat Q4_K_M.gguf.
//
// All expected values match the GGML algorithm exactly:
//   - getScaleMinK4 for scale/min extraction
//   - 4 groups of 64 elements: 32 lo-nibbles (d1/m1) + 32 hi-nibbles (d2/m2)
//
// Block layout (144 bytes):
//
//	[0:2]   d    = 0x0428 → 6.342e-05 (F16)
//	[2:4]   dmin = 0x1033 → 5.126e-04 (F16)
//	[4:16]  scales[12] packed
//	[16:144] qs[128] 4-bit values

import (
	"math"
	"testing"
)

// realQ4KBlock contains the exact first 144 bytes of blk.0.attn_q.weight
// from tinyllama-1.1b-chat.Q4_K_M.gguf (tensor offset 115732480, dtype=12/Q4_K).
var realQ4KBlock = []byte{
	0x28, 0x04, 0x33, 0x10, 0xdd, 0xad, 0xed, 0xa8, 0xdc, 0x5d, 0xf7, 0xd8, 0x13, 0x7d, 0xff, 0x0a, // [0..15]
	0x57, 0x76, 0x64, 0x50, 0x86, 0x76, 0x48, 0x86, 0x58, 0x6f, 0xf7, 0x78, 0x5a, 0xd8, 0x47, 0x66, // [16..31]
	0x7a, 0x37, 0x4c, 0x67, 0x99, 0x44, 0x88, 0x68, 0x08, 0x86, 0x38, 0xa8, 0xac, 0x46, 0x48, 0x69, // [32..47]
	0x4a, 0x8c, 0x3f, 0xcb, 0x5a, 0x59, 0x0b, 0x5a, 0x2b, 0x28, 0x5f, 0x5b, 0x3a, 0x7a, 0x54, 0xa9, // [48..63]
	0x59, 0xea, 0x9a, 0x3a, 0x89, 0x4c, 0x6c, 0x0b, 0x7d, 0x8b, 0x5a, 0x5b, 0x5a, 0x1d, 0x58, 0x30, // [64..79]
	0x4a, 0x59, 0x67, 0x76, 0x6c, 0x38, 0x8b, 0x48, 0x46, 0x48, 0x58, 0x46, 0x08, 0x67, 0x67, 0x36, // [80..95]
	0x88, 0x69, 0x00, 0x46, 0x48, 0x78, 0x3b, 0x48, 0x1f, 0x49, 0x58, 0x74, 0x38, 0xf7, 0x6b, 0x84, // [96..111]
	0x99, 0xa9, 0xa8, 0x98, 0xba, 0xa6, 0x89, 0x08, 0xbc, 0x6a, 0x54, 0x80, 0xd9, 0xd8, 0xb9, 0xc9, // [112..127]
	0x97, 0x86, 0x76, 0x99, 0xb9, 0xb8, 0xf8, 0xcf, 0xd9, 0xba, 0xa9, 0x78, 0xd8, 0x77, 0x88, 0x84, // [128..143]
}

// TestDequantizeQ4K_RealData_First10 verifies the first 10 dequantized values against
// the GGML reference (dequantize_row_q4_K from ggml-quants.c).
//
// GGML getScaleMinK4 for this block's scales[12]=data[4:16]:
//
//	j=0: sc=29, m=28  → d1=d*29, m1v=dmin*28
//	j=1: sc=45, m=29  → d2=d*45, m2v=dmin*29
//
// Group 0 (j=0..1): result[0..31] use d1/m1v (lo nibbles), result[32..63] use d2/m2v (hi nibbles).
// These values were computed by running the GGML algorithm in Go (D:/tmp/qcheck/main.go).
func TestDequantizeQ4K_RealData_First10(t *testing.T) {
	result, err := dequantizeBlockQ4_K(realQ4KBlock)
	if err != nil {
		t.Fatalf("dequantizeBlockQ4_K failed: %v", err)
	}
	if len(result) != 256 {
		t.Fatalf("expected 256 elements, got %d", len(result))
	}

	const tol = 1e-5

	type want struct {
		idx int
		val float32
	}

	// Correct GGML values for the first 10 elements.
	// Group 0 lo-nibble pass (elements 0..31): d1=d*sc(j=0)=d*29, m1v=dmin*m(j=0)=dmin*28.
	wantVals := []want{
		{0, -0.00147867},
		{1, -0.00331783},
		{2, -0.00699615},
		{3, -0.01435280},
		{4, -0.00331783},
		{5, -0.00331783},
		{6, 0.00036049},
		{7, -0.00331783},
		{8, 0.00036049},
		{9, 0.01323462},
	}

	for _, w := range wantVals {
		if math.Abs(float64(result[w.idx]-w.val)) > tol {
			t.Errorf("result[%d] = %.8f, want %.8f (diff=%.2e)",
				w.idx, result[w.idx], w.val, math.Abs(float64(result[w.idx]-w.val)))
		}
	}
}

// TestDequantizeQ4K_RealData_GroupBoundaries verifies values at the start of each
// of the 4 GGML groups (indices 0, 64, 128, 192) and at the hi-nibble boundary
// within each group (indices 32, 96, 160, 224).
//
// GGML scale/min pairs for this block:
//
//	j=0: sc=29, m=28   j=1: sc=45, m=29   → group 0: result[0..31]=d1/m1v, result[32..63]=d2/m2v
//	j=2: sc=45, m=55   j=3: sc=40, m=24   → group 1: result[64..95]=d1/m1v, result[96..127]=d2/m2v
//	j=4: sc=51, m=49   j=5: sc=45, m=23   → group 2: result[128..159]=d1/m1v, result[160..191]=d2/m2v
//	j=6: sc=63, m=63   j=7: sc=42, m=48   → group 3: result[192..223]=d1/m1v, result[224..255]=d2/m2v
func TestDequantizeQ4K_RealData_GroupBoundaries(t *testing.T) {
	result, err := dequantizeBlockQ4_K(realQ4KBlock)
	if err != nil {
		t.Fatalf("dequantizeBlockQ4_K failed: %v", err)
	}

	const tol = 2e-5

	tests := []struct {
		name string
		idx  int
		want float32
	}{
		// Group 0 lo-nibble pass (j=0: sc=29, m=28):
		{"group0_lo[0]", 0, -0.001479},
		{"group0_lo[1]", 1, -0.003318},
		// Group 0 hi-nibble pass (j=1: sc=45, m=29):
		{"group0_hi[0]", 32, -0.000596},
		{"group0_hi[31]", 63, 0.002258},
		// Group 1 lo-nibble pass (j=2: sc=45, m=55):
		{"group1_lo[0]", 64, 0.000346},
		{"group1_lo[1]", 65, 0.006053},
		// Group 1 hi-nibble pass (j=3: sc=40, m=24):
		{"group1_hi[0]", 96, -0.002155},
		{"group1_hi[31]", 127, -0.004692},
		// Group 2 lo-nibble pass (j=4: sc=51, m=49):
		{"group2_lo[0]", 128, 0.007226},
		{"group2_lo[1]", 129, 0.003992},
		// Group 2 hi-nibble pass (j=5: sc=45, m=23):
		{"group2_hi[0]", 160, -0.000374},
		{"group2_hi[31]", 191, 0.011041},
		// Group 3 lo-nibble pass (j=6: sc=63, m=63):
		{"group3_lo[0]", 192, 0.003665},
		{"group3_lo[1]", 193, 0.003665},
		// Group 3 hi-nibble pass (j=7: sc=42, m=48):
		{"group3_hi[0]", 224, -0.000632},
		{"group3_hi[31]", 255, -0.003296},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if math.Abs(float64(result[tt.idx]-tt.want)) > tol {
				t.Errorf("result[%d] = %.8f, want %.8f (diff=%.2e)",
					tt.idx, result[tt.idx], tt.want,
					math.Abs(float64(result[tt.idx]-tt.want)))
			}
		})
	}
}

// TestDequantizeQ4K_RealData_ScaleUnpacking verifies all 8 scale/min pairs via getScaleMinK4
// against the GGML reference values for this block's scales[12].
//
// GGML getScaleMinK4 values for data[4:16] of realQ4KBlock:
//
//	j=0: sc=29, m=28   j=1: sc=45, m=29
//	j=2: sc=45, m=55   j=3: sc=40, m=24
//	j=4: sc=51, m=49   j=5: sc=45, m=23
//	j=6: sc=63, m=63   j=7: sc=42, m=48
func TestDequantizeQ4K_RealData_ScaleUnpacking(t *testing.T) {
	q := realQ4KBlock[4:16]

	wantScales := []uint8{29, 45, 45, 40, 51, 45, 63, 42}
	wantMins := []uint8{28, 29, 55, 24, 49, 23, 63, 48}

	for j := 0; j < 8; j++ {
		sc, m := getScaleMinK4(j, q)
		if sc != wantScales[j] {
			t.Errorf("scales[%d] = %d, want %d", j, sc, wantScales[j])
		}
		if m != wantMins[j] {
			t.Errorf("mins[%d] = %d, want %d", j, m, wantMins[j])
		}
	}
}

// TestDequantizeQ4K_RealData_AllElementsFinite checks that no NaN or Inf
// values are produced for the complete 256-element block.
func TestDequantizeQ4K_RealData_AllElementsFinite(t *testing.T) {
	result, err := dequantizeBlockQ4_K(realQ4KBlock)
	if err != nil {
		t.Fatalf("dequantizeBlockQ4_K failed: %v", err)
	}

	for i, v := range result {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			t.Errorf("result[%d] = %v (not finite)", i, v)
		}
	}
}
