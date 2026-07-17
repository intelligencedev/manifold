package gguf

import (
	"encoding/binary"
	"math"
	"testing"
)

// TestFloat16ToFloat32 tests IEEE 754 half precision conversion.
func TestFloat16ToFloat32(t *testing.T) {
	tests := []struct {
		name string
		h    uint16
		want float32
	}{
		{"zero", 0x0000, 0.0},
		{"one", 0x3C00, 1.0},
		{"minus_one", 0xBC00, -1.0},
		{"two", 0x4000, 2.0},
		{"half", 0x3800, 0.5},
		{"max_normal", 0x7BFF, 65504.0},
		{"min_positive_normal", 0x0400, 6.103515625e-05},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Float16ToFloat32(tt.h)
			if math.Abs(float64(got-tt.want)) > 1e-6 {
				t.Errorf("Float16ToFloat32(0x%04X) = %v, want %v", tt.h, got, tt.want)
			}
		})
	}
}

// TestDequantizeF32 tests identity operation for F32 type.
func TestDequantizeF32(t *testing.T) {
	// Create test data: [1.0, 2.0, 3.0, 4.0].
	data := make([]byte, 16)
	binary.LittleEndian.PutUint32(data[0:4], math.Float32bits(1.0))
	binary.LittleEndian.PutUint32(data[4:8], math.Float32bits(2.0))
	binary.LittleEndian.PutUint32(data[8:12], math.Float32bits(3.0))
	binary.LittleEndian.PutUint32(data[12:16], math.Float32bits(4.0))

	result, err := Dequantize(data, GGMLTypeF32, 4)
	if err != nil {
		t.Fatalf("Dequantize failed: %v", err)
	}

	expected := []float32{1.0, 2.0, 3.0, 4.0}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("result[%d] = %v, want %v", i, result[i], v)
		}
	}
}

// TestDequantizeF16 tests half precision conversion.
func TestDequantizeF16(t *testing.T) {
	// Create test data: [1.0, 2.0, 0.5, -1.0] in F16.
	data := make([]byte, 8)
	binary.LittleEndian.PutUint16(data[0:2], 0x3C00) // 1.0
	binary.LittleEndian.PutUint16(data[2:4], 0x4000) // 2.0
	binary.LittleEndian.PutUint16(data[4:6], 0x3800) // 0.5
	binary.LittleEndian.PutUint16(data[6:8], 0xBC00) // -1.0

	result, err := Dequantize(data, GGMLTypeF16, 4)
	if err != nil {
		t.Fatalf("Dequantize failed: %v", err)
	}

	expected := []float32{1.0, 2.0, 0.5, -1.0}
	for i, v := range expected {
		if math.Abs(float64(result[i]-v)) > 1e-6 {
			t.Errorf("result[%d] = %v, want %v", i, result[i], v)
		}
	}
}

// TestDequantizeQ8_0 tests 8-bit quantization (simplest quantized format).
func TestDequantizeQ8_0(t *testing.T) {
	// Create Q8_0 block: d=0.5, qs=[1, 2, -1, -2, ...].
	data := make([]byte, 34)

	// d = 0.5 in F16.
	binary.LittleEndian.PutUint16(data[0:2], 0x3800)

	// qs: 32 int8 values.
	for i := 0; i < 32; i++ {
		data[2+i] = byte(int8(i - 16))
	}

	result, err := DequantizeBlock(data, GGMLTypeQ8_0)
	if err != nil {
		t.Fatalf("DequantizeBlock failed: %v", err)
	}

	if len(result) != 32 {
		t.Fatalf("expected 32 elements, got %d", len(result))
	}

	// Check first few values: d * q[i].
	d := float32(0.5)
	for i := 0; i < 4; i++ {
		expected := d * float32(int8(i-16))
		if math.Abs(float64(result[i]-expected)) > 1e-6 {
			t.Errorf("result[%d] = %v, want %v", i, result[i], expected)
		}
	}
}

// TestDequantizeQ4_0 tests 4-bit quantization.
func TestDequantizeQ4_0(t *testing.T) {
	// Create Q4_0 block: d=1.0, qs packed 4-bit values.
	data := make([]byte, 18)

	// d = 1.0 in F16.
	binary.LittleEndian.PutUint16(data[0:2], 0x3C00)

	// qs: 16 bytes, each containing two 4-bit values (0-15).
	// First byte: 0x10 = low=0, high=1.
	for i := 0; i < 16; i++ {
		data[2+i] = byte(i) | (byte(i+1) << 4)
	}

	result, err := DequantizeBlock(data, GGMLTypeQ4_0)
	if err != nil {
		t.Fatalf("DequantizeBlock failed: %v", err)
	}

	if len(result) != 32 {
		t.Fatalf("expected 32 elements, got %d", len(result))
	}

	// Formula: d * (q - 8).
	// First element: q=0, result = 1.0 * (0 - 8) = -8.0.
	if result[0] != -8.0 {
		t.Errorf("result[0] = %v, want -8.0", result[0])
	}

	// Second element: q=1, result = 1.0 * (1 - 8) = -7.0.
	if result[1] != -7.0 {
		t.Errorf("result[1] = %v, want -7.0", result[1])
	}
}

// TestDequantizeQ4_1 tests 4-bit quantization with offset.
func TestDequantizeQ4_1(t *testing.T) {
	// Create Q4_1 block: d=0.5, m=1.0, qs packed 4-bit values.
	data := make([]byte, 20)

	// d = 0.5 in F16.
	binary.LittleEndian.PutUint16(data[0:2], 0x3800)
	// m = 1.0 in F16.
	binary.LittleEndian.PutUint16(data[2:4], 0x3C00)

	// qs: 16 bytes.
	for i := 0; i < 16; i++ {
		data[4+i] = 0x00 // All zeros for simplicity.
	}

	result, err := DequantizeBlock(data, GGMLTypeQ4_1)
	if err != nil {
		t.Fatalf("DequantizeBlock failed: %v", err)
	}

	if len(result) != 32 {
		t.Fatalf("expected 32 elements, got %d", len(result))
	}

	// Formula: d * q + m = 0.5 * 0 + 1.0 = 1.0.
	expected := float32(1.0)
	for i := 0; i < 32; i++ {
		if math.Abs(float64(result[i]-expected)) > 1e-6 {
			t.Errorf("result[%d] = %v, want %v", i, result[i], expected)
		}
	}
}

// TestDequantizeQ5_0 tests 5-bit quantization (Q5_0).
//
// Block layout (22 bytes):
//
//	[0:2]  d (F16)
//	[2:6]  qh (uint32 — high bits for all 32 elements, 1 bit each)
//	[6:22] qs[16] (4 low bits per element, 2 per byte)
//
// Formula: x[i] = d * (q[i] - 16)  where q[i] is 5-bit unsigned (0..31).
//
//nolint:gocognit // Table-driven test: multiple subtests with inline closures inflate gocognit score.
func TestDequantizeQ5_0(t *testing.T) {
	tests := []struct {
		name  string
		d     uint16
		qh    uint32
		qs    []byte
		check func(t *testing.T, result []float32)
	}{
		{
			name: "all_zero_no_high_bits",
			// q=0, d=1.0 → result = 1.0*(0-16) = -16.0
			d:  0x3C00,
			qh: 0,
			qs: make([]byte, 16),
			check: func(t *testing.T, result []float32) {
				t.Helper()
				expected := float32(-16.0)
				for i := 0; i < 32; i++ {
					if math.Abs(float64(result[i]-expected)) > 1e-6 {
						t.Errorf("result[%d] = %v, want %v", i, result[i], expected)
					}
				}
			},
		},
		{
			name: "high_bits_set_makes_max_value",
			// qs=0xFF (lo=0xF, hi=0xF per byte), qh=0xFFFFFFFF (all high bits set).
			// q = 0xF | (1<<4) = 31, d=1.0 → result = 1.0*(31-16) = 15.0
			d:  0x3C00,
			qh: 0xFFFFFFFF,
			qs: []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
				0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
			check: func(t *testing.T, result []float32) {
				t.Helper()
				expected := float32(15.0) // d*(31-16)
				for i := 0; i < 32; i++ {
					if math.Abs(float64(result[i]-expected)) > 1e-6 {
						t.Errorf("result[%d] = %v, want %v", i, result[i], expected)
					}
				}
			},
		},
		{
			name: "mixed_nibbles_known_high_bits",
			// qs[0] = 0x53 → lo=3, hi=5 (two elements).
			// qh = 0x00000003 → bit0=1 (elem0 high), bit1=0 (elem1 high=0).
			// q0 = 3|(1<<4) = 19, q1 = 5|(0<<4) = 5
			// d=1.0: result[0] = 1.0*(19-16)=3.0, result[1]=1.0*(5-16)=-11.0
			d:  0x3C00,
			qh: 0x00000001, // only bit0 set
			qs: func() []byte {
				b := make([]byte, 16)
				b[0] = 0x53
				return b
			}(),
			check: func(t *testing.T, result []float32) {
				t.Helper()
				if math.Abs(float64(result[0]-3.0)) > 1e-6 {
					t.Errorf("result[0] = %v, want 3.0 (q=3|16=19, 19-16=3)", result[0])
				}
				if math.Abs(float64(result[1]+11.0)) > 1e-6 {
					t.Errorf("result[1] = %v, want -11.0 (q=5, 5-16=-11)", result[1])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, 22)
			binary.LittleEndian.PutUint16(data[0:2], tt.d)
			binary.LittleEndian.PutUint32(data[2:6], tt.qh)
			copy(data[6:22], tt.qs)

			result, err := DequantizeBlock(data, GGMLTypeQ5_0)
			if err != nil {
				t.Fatalf("DequantizeBlock failed: %v", err)
			}
			if len(result) != 32 {
				t.Fatalf("expected 32 elements, got %d", len(result))
			}
			tt.check(t, result)
		})
	}
}

// TestDequantizeQ4_K tests 4-bit K-quant dequantization against the GGML reference algorithm.
//
// Block layout (144 bytes):
//
//	[0:2]   d    (F16 super-scale)
//	[2:4]   dmin (F16 super-min)
//	[4:16]  scales[12] (packed via get_scale_min_k4)
//	[16:144] qs[128] (4-bit values, 2 per byte)
//
// GGML element ordering: 4 groups of 64. Each group j (j=0..3):
//   - scale pair (is, is+1) via getScaleMinK4 where is = j*2
//   - first 32 outputs: lo nibbles of qs[j*32..(j+1)*32-1], using d1=d*sc(is),   m1v=dmin*m(is)
//   - next  32 outputs: hi nibbles of the same range,       using d2=d*sc(is+1), m2v=dmin*m(is+1)
//
// Test data: d=1.0, dmin=0.5
//
//	q[0]=3 (sc(j=0)=3, m(j=0)=q[4]&63=0)
//	q[1]=5 (sc(j=1)=5, m(j=1)=q[5]&63=0)
//	qs[16]=0x53 → lo_nibble=3, hi_nibble=5
//
// GGML group 0 (j=0): d1=1.0*3=3.0, m1v=0.5*0=0.0; d2=1.0*5=5.0, m2v=0.0
//
//	result[0]  = d1 * lo_nibble(qs[0]) - m1v = 3.0 * 3 - 0 = 9.0
//	result[1]  = d1 * lo_nibble(qs[1]) - m1v = 3.0 * 0 - 0 = 0.0  (qs[1]=0)
//	result[32] = d2 * hi_nibble(qs[0]) - m2v = 5.0 * 5 - 0 = 25.0
//	result[33] = d2 * hi_nibble(qs[1]) - m2v = 5.0 * 0 - 0 = 0.0
func TestDequantizeQ4_K(t *testing.T) {
	data := make([]byte, 144)

	// d=1.0 (F16 0x3C00), dmin=0.5 (F16 0x3800).
	binary.LittleEndian.PutUint16(data[0:2], 0x3C00)
	binary.LittleEndian.PutUint16(data[2:4], 0x3800)

	// q[0..11] = scales[12] at data[4..15].
	// getScaleMinK4(j=0, q): sc=q[0]&63=3, m=q[4]&63=data[8]&63=0
	// getScaleMinK4(j=1, q): sc=q[1]&63=5, m=q[5]&63=data[9]&63=0
	data[4+0] = 3 // q[0]: sc(j=0)=3
	data[4+1] = 5 // q[1]: sc(j=1)=5
	// q[4]=data[8]=0 → m(j=0)=0; q[5]=data[9]=0 → m(j=1)=0

	// qs[128] at data[16..143]. Group 0 uses qs[0..31] = data[16..47].
	// qs[0] = data[16] = 0x53 → lo_nibble=3, hi_nibble=5.
	data[16] = 0x53

	result, err := DequantizeBlock(data, GGMLTypeQ4_K)
	if err != nil {
		t.Fatalf("DequantizeBlock failed: %v", err)
	}

	if len(result) != 256 {
		t.Fatalf("expected 256 elements, got %d", len(result))
	}

	// Group 0, first lo-nibble element: d1*3 - m1v = 3.0*3 - 0 = 9.0.
	if math.Abs(float64(result[0]-9.0)) > 0.01 {
		t.Errorf("result[0] = %.4f, want 9.0 (d1=3.0, lo_nibble=3, m1v=0)", result[0])
	}
	// Group 0, second lo-nibble element (qs[1]=0): d1*0 - m1v = 0.0.
	if math.Abs(float64(result[1])) > 0.01 {
		t.Errorf("result[1] = %.4f, want 0.0 (d1=3.0, lo_nibble=0, m1v=0)", result[1])
	}
	// Group 0, first hi-nibble element: d2*5 - m2v = 5.0*5 - 0 = 25.0.
	if math.Abs(float64(result[32]-25.0)) > 0.01 {
		t.Errorf("result[32] = %.4f, want 25.0 (d2=5.0, hi_nibble=5, m2v=0)", result[32])
	}
	// Group 0, second hi-nibble element (qs[1] hi=0): d2*0 - m2v = 0.0.
	if math.Abs(float64(result[33])) > 0.01 {
		t.Errorf("result[33] = %.4f, want 0.0 (d2=5.0, hi_nibble=0, m2v=0)", result[33])
	}

	t.Logf("Q4_K group0: result[0]=%.2f result[32]=%.2f (expect 9.0, 25.0)", result[0], result[32])
}

// TestDequantizeQ4_K_ScaleExtraction verifies getScaleMinK4 unpacking and scale application.
//
// GGML getScaleMinK4 for q=[1,2,3,4,10,20,30,40,0,0,0,0]:
//
//	j=0: sc=q[0]&63=1, m=q[4]&63=10
//	j=1: sc=q[1]&63=2, m=q[5]&63=20
//	j=2: sc=q[2]&63=3, m=q[6]&63=30
//	j=3: sc=q[3]&63=4, m=q[7]&63=40
//	j=4: sc=(q[8]&0xF)|((q[0]>>6)<<4)=0, m=(q[8]>>4)|((q[4]>>6)<<4)=0
//	j=5..7: sc=0, m=0
//
// With d=dmin=1.0, qs[0]=0x01 (lo_nibble=1):
//
//	group 0 (j=0,1): d1=1*1=1.0, m1v=1*10=10.0
//	result[0] = 1.0*1 - 10.0 = -9.0
func TestDequantizeQ4_K_ScaleExtraction(t *testing.T) {
	data := make([]byte, 144)
	binary.LittleEndian.PutUint16(data[0:2], 0x3C00) // d=1.0
	binary.LittleEndian.PutUint16(data[2:4], 0x3C00) // dmin=1.0

	// q[0..7] set sc and m values via getScaleMinK4:
	// j<4: sc=q[j]&63, m=q[j+4]&63
	data[4+0] = 1  // q[0]: sc(j=0)=1
	data[4+1] = 2  // q[1]: sc(j=1)=2
	data[4+2] = 3  // q[2]: sc(j=2)=3
	data[4+3] = 4  // q[3]: sc(j=3)=4
	data[4+4] = 10 // q[4]: m(j=0)=10
	data[4+5] = 20 // q[5]: m(j=1)=20
	data[4+6] = 30 // q[6]: m(j=2)=30
	data[4+7] = 40 // q[7]: m(j=3)=40
	// q[8..11]=0 → j=4..7: sc=0, m=0

	// Verify all-zero qs produce no error.
	if _, err := DequantizeBlock(data, GGMLTypeQ4_K); err != nil {
		t.Fatal(err)
	}

	// Group 0 uses scale pair (j=0, j=1): d1=1*1=1.0, m1v=1*10=10.0
	// qs[0]=0x01 → lo_nibble=1 → result[0] = 1.0*1 - 10.0 = -9.0
	data[16] = 0x01
	result, _ := DequantizeBlock(data, GGMLTypeQ4_K)

	if math.Abs(float64(result[0]+9.0)) > 0.01 {
		t.Errorf("sc(j=0)/m(j=0) extraction: result[0] = %.4f, want -9.0 (d1=1, lo=1, m1v=10)", result[0])
	}

	// Group 2 (j=2) uses scale pair (j=4, j=5): both sc=0, m=0 (q[8..11]=0).
	// qs[64]=0x01 → lo_nibble=1 → result[128] = 0*1 - 0 = 0.0
	data[16+64] = 0x01
	result, _ = DequantizeBlock(data, GGMLTypeQ4_K)

	if math.Abs(float64(result[128])) > 0.01 {
		t.Errorf("sc(j=4) extraction: result[128] = %.4f, want 0.0 (sc=0 because q[8]=0)", result[128])
	}
}

// TestDequantizeQ5_K tests 5-bit K-quant dequantization against the GGML reference algorithm.
//
// Block layout (176 bytes):
//
//	[0:2]    d    (F16 super-scale)
//	[2:4]    dmin (F16 super-min)
//	[4:16]   scales[12] (packed via getScaleMinK4, same layout as Q4_K)
//	[16:48]  qh[32] (high bits, bit-packed: qh[elem/8] bit (elem%8))
//	[48:176] qs[128] (low 4 bits per element)
//
// GGML element ordering: 4 groups of 64. Each group j (j=0..3):
//   - first 32 outputs: lo nibbles + qh high bit, using d1=d*sc(is),   m1v=dmin*m(is)
//   - next  32 outputs: hi nibbles + qh high bit, using d2=d*sc(is+1), m2v=dmin*m(is+1)
//
// Key difference from old Born implementation: hi-nibble elements now occupy positions
// [32..63] within each group-of-64, NOT interleaved with lo-nibble elements.
//
//nolint:gocognit // Table-driven test: multiple subtests with inline closures inflate gocognit score.
func TestDequantizeQ5_K(t *testing.T) {
	tests := []struct {
		name  string
		setup func(data []byte)
		check func(t *testing.T, result []float32)
	}{
		{
			name: "all_zero_qs_no_high_bits",
			setup: func(data []byte) {
				// d=1.0, dmin=0.0, q[0]=1 → sc(j=0)=1, m(j=0)=q[4]&63=0.
				// qh=0 → all high bits zero. qs=0 → all q=0.
				// result[0..31] (group0 lo-nibble pass): d1=1.0, m1v=0.0, q=0 → 0.0
				binary.LittleEndian.PutUint16(data[0:2], 0x3C00) // d=1.0
				binary.LittleEndian.PutUint16(data[2:4], 0x0000) // dmin=0
				data[4] = 0x01                                   // q[0]: sc(j=0)=1
			},
			check: func(t *testing.T, result []float32) {
				t.Helper()
				// Group 0 lo-nibble pass (result[0..31]): d1=1.0, qs=0, qh=0 → 0.0.
				for i := 0; i < 32; i++ {
					if result[i] != 0.0 {
						t.Errorf("result[%d] = %v, want 0.0", i, result[i])
					}
				}
			},
		},
		{
			name: "known_nibbles_no_high_bits",
			setup: func(data []byte) {
				// d=1.0, dmin=0.0.
				// q[0]=1 → sc(j=0)=1, m(j=0)=0; q[1]=0 → sc(j=1)=0, m(j=1)=0.
				// d1=1.0, m1v=0.0; d2=0.0, m2v=0.0.
				// qh=0 → all high bits zero.
				// ql[0]=0x32 → lo_nibble=2, hi_nibble=3.
				// result[0]  = d1 * (lo_nibble(ql[0]) | 0<<4) - m1v = 1.0*2 - 0 = 2.0
				// result[1]  = d1 * (lo_nibble(ql[1]=0) | 0) - m1v = 0.0
				// result[32] = d2 * (hi_nibble(ql[0]) | 0<<4) - m2v = 0.0*3 - 0 = 0.0
				binary.LittleEndian.PutUint16(data[0:2], 0x3C00) // d=1.0
				binary.LittleEndian.PutUint16(data[2:4], 0x0000) // dmin=0
				data[4] = 0x01                                   // q[0]: sc(j=0)=1
				data[48] = 0x32                                  // ql[0]=0x32: lo=2, hi=3
			},
			check: func(t *testing.T, result []float32) {
				t.Helper()
				if math.Abs(float64(result[0]-2.0)) > 1e-6 {
					t.Errorf("result[0] = %v, want 2.0 (d1=1.0, lo_nibble=2, no high bit)", result[0])
				}
				// result[1]: ql[1]=0, no high bit → 0.0 (NOT 3.0 — hi nibble is at result[32]).
				if math.Abs(float64(result[1])) > 1e-6 {
					t.Errorf("result[1] = %v, want 0.0 (ql[1]=0, lo pass)", result[1])
				}
				// result[32]: hi-nibble pass, d2=sc(j=1)=0 → 0.0 regardless of nibble value.
				if math.Abs(float64(result[32])) > 1e-6 {
					t.Errorf("result[32] = %v, want 0.0 (d2=0 because sc(j=1)=0)", result[32])
				}
			},
		},
		{
			name: "high_bits_set_for_first_two_elements",
			setup: func(data []byte) {
				// d=1.0, dmin=0.0.
				// q[0]=1 → sc(j=0)=1, m(j=0)=0; sc(j=1)=0.
				// d1=1.0, m1v=0.0; d2=0.0.
				// qh[0]=0x03 → bit0=1 (elem0 high), bit1=1 (elem1 high).
				// ql[0]=0x32: lo=2, hi=3; ql[1]=0.
				// elem0: global=0, qh[0] bit0=1 → q5=(2|16)=18 → result[0]=1.0*18=18.0
				// elem1: global=1, qh[0] bit1=1 → q5=(lo(ql[1])|16)=16 → result[1]=1.0*16=16.0
				// elem32: global=32, qh[4]=0 bit0=0 → q5=hi(ql[0])=3, d2=0 → result[32]=0.0
				binary.LittleEndian.PutUint16(data[0:2], 0x3C00) // d=1.0
				binary.LittleEndian.PutUint16(data[2:4], 0x0000) // dmin=0
				data[4] = 0x01                                   // q[0]: sc(j=0)=1
				data[16] = 0x03                                  // qh[0]: bit0=1 (elem0), bit1=1 (elem1)
				data[48] = 0x32                                  // ql[0]=0x32: lo=2, hi=3
			},
			check: func(t *testing.T, result []float32) {
				t.Helper()
				if math.Abs(float64(result[0]-18.0)) > 1e-6 {
					t.Errorf("result[0] = %v, want 18.0 (lo=2, high_bit=1 → q=18)", result[0])
				}
				// elem1: ql[1]=0 lo nibble=0, high_bit from qh[0] bit1=1 → q5=0|16=16.
				if math.Abs(float64(result[1]-16.0)) > 1e-6 {
					t.Errorf("result[1] = %v, want 16.0 (lo=0, high_bit=1 → q=16)", result[1])
				}
				// elem32 is hi-nibble pass, d2=0 → result[32]=0.0.
				if math.Abs(float64(result[32])) > 1e-6 {
					t.Errorf("result[32] = %v, want 0.0 (d2=0 because sc(j=1)=0)", result[32])
				}
			},
		},
		{
			name: "with_dmin_non_zero",
			setup: func(data []byte) {
				// d=1.0, dmin=1.0.
				// getScaleMinK4(j=0, q): sc=q[0]&63=1, m=q[4]&63=data[8]&63=8.
				// d1=1.0, m1v=1.0*8=8.0.
				// ql[0]=0x08 → lo_nibble=8.
				// result[0] = d1 * 8 - m1v = 1.0*8 - 8.0 = 0.0
				// result[1] = d1 * lo(ql[1]=0) - m1v = 0.0 - 8.0 = -8.0
				binary.LittleEndian.PutUint16(data[0:2], 0x3C00) // d=1.0
				binary.LittleEndian.PutUint16(data[2:4], 0x3C00) // dmin=1.0
				data[4] = 0x01                                   // q[0]: sc(j=0)=1
				data[4+4] = 0x08                                 // q[4]: m(j=0)=8
				data[48] = 0x08                                  // ql[0]: lo_nibble=8, hi_nibble=0
			},
			check: func(t *testing.T, result []float32) {
				t.Helper()
				if math.Abs(float64(result[0]-0.0)) > 1e-5 {
					t.Errorf("result[0] = %v, want 0.0 (d1*8 - m1v*8 = 0)", result[0])
				}
				if math.Abs(float64(result[1]+8.0)) > 1e-5 {
					t.Errorf("result[1] = %v, want -8.0 (d1*0 - m1v*8 = -8)", result[1])
				}
			},
		},
		{
			name: "returns_256_elements",
			setup: func(_ []byte) {
				// All-zero data — just verify element count.
			},
			check: func(t *testing.T, result []float32) {
				t.Helper()
				if len(result) != 256 {
					t.Errorf("expected 256 elements, got %d", len(result))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, 176)
			tt.setup(data)

			result, err := DequantizeBlock(data, GGMLTypeQ5_K)
			if err != nil {
				t.Fatalf("DequantizeBlock failed: %v", err)
			}
			tt.check(t, result)
		})
	}
}

// TestDequantizeQ6_K tests 6-bit K-quant (256 elements).
//
// Block layout (210 bytes):
//
//	[0:128]   ql[128]    (4 low bits per element, 2 elements per byte)
//	[128:192] qh[64]     (2 high bits per element, 4 elements per byte)
//	[192:208] scales[16] (signed int8, one per 16-element sub-block)
//	[208:210] d          (F16 super-scale)
//
// GGML output element ordering (matches ggml-quants.c dequantize_row_q6_K):
// Two passes of 128 elements each. Within each pass, inner loop l=0..31 produces:
//
//	y[l]:    d * sc[l/16]   * q1  where q1 = (ql[l]     lo nibble | qhLo << 4) - 32
//	y[l+32]: d * sc[l/16+2] * q2  where q2 = (ql[l+32]  lo nibble | qhLo << 4) - 32
//	y[l+64]: d * sc[l/16+4] * q3  where q3 = (ql[l]     hi nibble | qhHi << 4) - 32
//	y[l+96]: d * sc[l/16+6] * q4  where q4 = (ql[l+32]  hi nibble | qhHi << 4) - 32
//
// qhLo = (qh[l>>2] >> (2*(l&3)))   & 3
// qhHi = (qh[l>>2] >> (2*(l&3)+4)) & 3  — shift >7 gives 0 for l%4 ∈ {2,3}.
//
//nolint:gocognit // Table-driven test: multiple subtests with inline closures inflate gocognit score.
func TestDequantizeQ6_K(t *testing.T) {
	tests := []struct {
		name  string
		setup func(data []byte)
		check func(t *testing.T, result []float32)
	}{
		{
			name: "all_zero_quants_positive_scale",
			// ql=0, qh=0 → q = (0|0<<4)-32 = -32; d=1.0, scales=+1 → result = -32.0
			setup: func(data []byte) {
				for i := 0; i < 16; i++ {
					data[192+i] = 0x01 // signed int8 scale = +1
				}
				binary.LittleEndian.PutUint16(data[208:210], 0x3C00) // d=1.0
			},
			check: func(t *testing.T, result []float32) {
				t.Helper()
				if len(result) != 256 {
					t.Fatalf("expected 256 elements, got %d", len(result))
				}
				expected := float32(-32.0)
				for i := 0; i < 256; i++ {
					if math.Abs(float64(result[i]-expected)) > 1e-5 {
						t.Errorf("result[%d] = %v, want %v", i, result[i], expected)
					}
				}
			},
		},
		{
			name: "max_quant_value",
			// ql=0xFF (all nibbles=0xF), qh=0xFF for every byte, scales=+1, d=1.0.
			//
			// With correct qh indexing (data[qhBase+l], one byte per l):
			//   qhByte = 0xFF for every l.
			//   q1: (0xFF>>0)&3 = 3 → q = (0xF | 3<<4) - 32 = 63-32 = 31 → 31.0
			//   q2: (0xFF>>2)&3 = 3 → q = 31 → 31.0
			//   q3: (0xFF>>4)&3 = 3 → q = 31 → 31.0
			//   q4: (0xFF>>6)&3 = 3 → q = 31 → 31.0
			//
			// All 256 elements = d * sc * 31 = 1.0 * 1 * 31 = 31.0.
			setup: func(data []byte) {
				for i := 0; i < 128; i++ {
					data[i] = 0xFF
				}
				for i := 0; i < 64; i++ {
					data[128+i] = 0xFF
				}
				for i := 0; i < 16; i++ {
					data[192+i] = 0x01
				}
				binary.LittleEndian.PutUint16(data[208:210], 0x3C00) // d=1.0
			},
			check: func(t *testing.T, result []float32) {
				t.Helper()
				want := float32(31.0)
				for i := 0; i < 256; i++ {
					if math.Abs(float64(result[i]-want)) > 1e-5 {
						t.Errorf("result[%d] = %v, want %v", i, result[i], want)
					}
				}
			},
		},
		{
			name: "negative_scale",
			// scale = -1 (int8 0xFF), d=1.0, ql=0, qh=0 → q=-32.
			// result = 1.0 * (-1) * (-32) = 32.0 for all elements.
			setup: func(data []byte) {
				for i := 0; i < 16; i++ {
					data[192+i] = 0xFF // int8(-1) stored as 0xFF
				}
				binary.LittleEndian.PutUint16(data[208:210], 0x3C00) // d=1.0
			},
			check: func(t *testing.T, result []float32) {
				t.Helper()
				expected := float32(32.0) // 1.0 * (-1) * (0 - 32) = 32.0
				for i := 0; i < 256; i++ {
					if math.Abs(float64(result[i]-expected)) > 1e-5 {
						t.Errorf("result[%d] = %v, want %v", i, result[i], expected)
					}
				}
			},
		},
		{
			name: "lo_nibble_goes_to_y0_not_y1",
			// Verifies GGML element ordering: ql[0] lo-nibble → y[0], ql[0] hi-nibble → y[64].
			// ql[0]=0x25 (lo=5, hi=2), all other ql=0, qh=0, scales[0]=2, d=1.0.
			//
			// GGML: l=0, n=0:
			//   y[0]  = d*sc[0]*q1 = 1.0*2*((5|0<<4)-32) = 1.0*2*(-27) = -54.0
			//   y[64] = d*sc[4]*q3 = 1.0*sc[4]*((2|0<<4)-32); sc[4]=data[196]=0 → y[64]=0.0
			//   y[1]  = l=1: ql[1]=0; q1=(0|0)-32=-32; d*sc[0]*-32 = 1.0*2*(-32) = -64.0
			//
			// (Old Born had hi-nibble at y[1]=-60.0, which was wrong.)
			setup: func(data []byte) {
				data[0] = 0x25                                       // ql[0]: lo nibble=5, hi nibble=2
				data[192] = 0x02                                     // scales[0] = +2
				binary.LittleEndian.PutUint16(data[208:210], 0x3C00) // d=1.0
			},
			check: func(t *testing.T, result []float32) {
				t.Helper()
				// y[0]: ql[0] lo nibble=5, qh=0 → q1=(5|0)-32=-27; sc[0]=2; result=-54.0
				if math.Abs(float64(result[0]+54.0)) > 1e-5 {
					t.Errorf("result[0] = %v, want -54.0 (ql[0]lo=5, sc[0]=2, d*sc*q=1*2*(5-32))", result[0])
				}
				// y[1]: ql[1]=0, qh=0 → q1=(0|0)-32=-32; sc[0]=2; result=-64.0
				if math.Abs(float64(result[1]+64.0)) > 1e-5 {
					t.Errorf("result[1] = %v, want -64.0 (ql[1]lo=0, sc[0]=2, d*sc*q=1*2*(0-32))", result[1])
				}
				// y[64]: ql[0] hi nibble=2, qhHi=0 → q3=(2|0)-32=-30; sc[4]=data[196]=0; result=0.0
				if math.Abs(float64(result[64])) > 1e-5 {
					t.Errorf("result[64] = %v, want 0.0 (ql[0]hi=2, sc[4]=0)", result[64])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, 210)
			tt.setup(data)

			result, err := DequantizeBlock(data, GGMLTypeQ6_K)
			if err != nil {
				t.Fatalf("DequantizeBlock failed: %v", err)
			}
			tt.check(t, result)
		})
	}
}

// TestDequantizeUnsupportedType tests error handling for unsupported types.
func TestDequantizeUnsupportedType(t *testing.T) {
	data := make([]byte, 100)

	// Test with Q2_K (not implemented yet).
	_, err := Dequantize(data, GGMLTypeQ2_K, 32)
	if err == nil {
		t.Error("expected error for unsupported type Q2_K, got nil")
	}

	// Test with IQ2_XXS (not implemented).
	_, err = Dequantize(data, GGMLTypeIQ2_XXS, 32)
	if err == nil {
		t.Error("expected error for unsupported type IQ2_XXS, got nil")
	}
}

// TestDequantizeInsufficientData tests error handling for insufficient data.
func TestDequantizeInsufficientData(t *testing.T) {
	// Q8_0 needs 34 bytes per block.
	data := make([]byte, 10) // Too small.

	_, err := Dequantize(data, GGMLTypeQ8_0, 32)
	if err == nil {
		t.Error("expected error for insufficient data, got nil")
	}
}

// TestDequantizeBlockQ8_1 tests Q8_1 dequantization.
//
// Q8_1 block layout: d (F16), s (F16), qs[32] (int8).
// The 's' field = sum(qs)*d is stored for GGML dot-product acceleration only
// and is NOT part of the element-wise dequantization formula.
//
// Reference: ggml-quants.c dequantize_row_q8_1 — formula is x[i] = d * qs[i].
func TestDequantizeBlockQ8_1(t *testing.T) {
	data := make([]byte, 36)

	// d = 0.1 in F16 (approx 0x2E66).
	binary.LittleEndian.PutUint16(data[0:2], 0x2E66)
	// s = sum(qs)*d precomputed field — set to a non-zero value to confirm it is not used.
	// If the formula incorrectly adds s*sum, this test will catch it.
	binary.LittleEndian.PutUint16(data[2:4], 0x2028) // ~0.008118

	// qs: 32 int8 values [1, 2, 3, ..., 32].
	for i := 0; i < 32; i++ {
		data[4+i] = byte(int8(i + 1))
	}

	result, err := DequantizeBlock(data, GGMLTypeQ8_1)
	if err != nil {
		t.Fatalf("DequantizeBlock failed: %v", err)
	}
	if len(result) != 32 {
		t.Fatalf("expected 32 elements, got %d", len(result))
	}

	// Correct formula: x[i] = d * qs[i].
	d := Float16ToFloat32(0x2E66)

	for i := 0; i < 32; i++ {
		expected := d * float32(i+1)
		if math.Abs(float64(result[i]-expected)) > 1e-4 {
			t.Errorf("result[%d] = %v, want %v (d*%d)", i, result[i], expected, i+1)
		}
	}
}

// TestDequantizeBlockQ8_1_NegativeValues tests Q8_1 with negative int8 values
// to confirm signed reinterpretation is correct.
func TestDequantizeBlockQ8_1_NegativeValues(t *testing.T) {
	data := make([]byte, 36)

	// d = 1.0 in F16.
	binary.LittleEndian.PutUint16(data[0:2], 0x3C00)
	// s field irrelevant for dequant.
	binary.LittleEndian.PutUint16(data[2:4], 0x0000)

	// qs: [-16, -8, 0, 8, 16, 0, ...].
	// Using a slice of named constants expressed as hex to avoid compile-time
	// "constant overflows byte" errors while keeping intent clear.
	// 0xF0 = byte(-16), 0xF8 = byte(-8), 0x00 = 0, 0x08 = 8, 0x10 = 16.
	data[4] = 0xF0 // int8(-16)
	data[5] = 0xF8 // int8(-8)
	data[6] = 0x00 // int8(0)
	data[7] = 0x08 // int8(8)
	data[8] = 0x10 // int8(16)

	result, err := DequantizeBlock(data, GGMLTypeQ8_1)
	if err != nil {
		t.Fatalf("DequantizeBlock failed: %v", err)
	}

	tests := []struct {
		idx  int
		want float32
	}{
		{0, -16.0},
		{1, -8.0},
		{2, 0.0},
		{3, 8.0},
		{4, 16.0},
	}
	for _, tt := range tests {
		if math.Abs(float64(result[tt.idx]-tt.want)) > 1e-6 {
			t.Errorf("result[%d] = %v, want %v", tt.idx, result[tt.idx], tt.want)
		}
	}
}

// TestDequantizeMultipleBlocks tests Dequantize with multiple blocks.
func TestDequantizeMultipleBlocks(t *testing.T) {
	// Create data for 2 Q8_0 blocks (64 elements total).
	data := make([]byte, 68) // 2 * 34 bytes.

	// Block 1: d=1.0, qs=[0, 1, 2, ..., 31].
	binary.LittleEndian.PutUint16(data[0:2], 0x3C00)
	for i := 0; i < 32; i++ {
		data[2+i] = byte(int8(i))
	}

	// Block 2: d=2.0, qs=[0, 1, 2, ..., 31].
	binary.LittleEndian.PutUint16(data[34:36], 0x4000)
	for i := 0; i < 32; i++ {
		data[36+i] = byte(int8(i))
	}

	result, err := Dequantize(data, GGMLTypeQ8_0, 64)
	if err != nil {
		t.Fatalf("Dequantize failed: %v", err)
	}

	if len(result) != 64 {
		t.Fatalf("expected 64 elements, got %d", len(result))
	}

	// Check first block: d=1.0, result[0] = 1.0 * 0 = 0.
	if result[0] != 0.0 {
		t.Errorf("result[0] = %v, want 0.0", result[0])
	}

	// Check second block: d=2.0, result[32] = 2.0 * 0 = 0.
	if result[32] != 0.0 {
		t.Errorf("result[32] = %v, want 0.0", result[32])
	}

	// Check second block element: result[33] = 2.0 * 1 = 2.0.
	if result[33] != 2.0 {
		t.Errorf("result[33] = %v, want 2.0", result[33])
	}
}

// TestDequantizeQ5_1 tests 5-bit quantization with offset.
func TestDequantizeQ5_1(t *testing.T) {
	// Create Q5_1 block: d=0.5, m=1.0, qh=0, qs=0.
	data := make([]byte, 24)

	// d = 0.5 in F16.
	binary.LittleEndian.PutUint16(data[0:2], 0x3800)
	// m = 1.0 in F16.
	binary.LittleEndian.PutUint16(data[2:4], 0x3C00)

	// qh = 0 (no high bits).
	binary.LittleEndian.PutUint32(data[4:8], 0)

	// qs = 0 (all zeros).
	for i := 0; i < 16; i++ {
		data[8+i] = 0
	}

	result, err := DequantizeBlock(data, GGMLTypeQ5_1)
	if err != nil {
		t.Fatalf("DequantizeBlock failed: %v", err)
	}

	if len(result) != 32 {
		t.Fatalf("expected 32 elements, got %d", len(result))
	}

	// Formula: d * q + m = 0.5 * 0 + 1.0 = 1.0.
	expected := float32(1.0)
	for i := 0; i < 32; i++ {
		if math.Abs(float64(result[i]-expected)) > 1e-6 {
			t.Errorf("result[%d] = %v, want %v", i, result[i], expected)
		}
	}
}
