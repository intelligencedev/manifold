package gguf

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Dequantize преобразует quantized данные в float32.
// Поддерживает: F32, F16, Q4_0, Q4_1, Q5_0, Q5_1, Q8_0, Q8_1, Q4_K, Q5_K, Q6_K.
func Dequantize(data []byte, dtype GGMLType, numElements int) ([]float32, error) {
	trait := dtype.Trait()

	// Validate input.
	if trait.TypeSize == 0 {
		return nil, fmt.Errorf("unsupported type: %s", dtype)
	}

	expectedBytes := dtype.RowSize(numElements)
	if len(data) < expectedBytes {
		return nil, fmt.Errorf("insufficient data: need %d bytes, got %d", expectedBytes, len(data))
	}

	// Handle non-quantized types.
	if !trait.Quantized {
		return dequantizeUnquantized(data, dtype, numElements)
	}

	// Handle quantized types block by block.
	result := make([]float32, numElements)
	numBlocks := (numElements + trait.BlockSize - 1) / trait.BlockSize

	offset := 0
	elemIdx := 0

	for i := 0; i < numBlocks; i++ {
		blockData := data[offset : offset+trait.TypeSize]
		block, err := DequantizeBlock(blockData, dtype)
		if err != nil {
			return nil, fmt.Errorf("dequantize block %d: %w", i, err)
		}

		// Copy block values to result.
		for j := 0; j < len(block) && elemIdx < numElements; j++ {
			result[elemIdx] = block[j]
			elemIdx++
		}

		offset += trait.TypeSize
	}

	return result, nil
}

// DequantizeBlock дequантизирует один блок данных.
// Используется для streaming дequантизации больших тензоров.
func DequantizeBlock(data []byte, dtype GGMLType) ([]float32, error) {
	switch dtype {
	case GGMLTypeQ4_0:
		return dequantizeBlockQ4_0(data)
	case GGMLTypeQ4_1:
		return dequantizeBlockQ4_1(data)
	case GGMLTypeQ5_0:
		return dequantizeBlockQ5_0(data)
	case GGMLTypeQ5_1:
		return dequantizeBlockQ5_1(data)
	case GGMLTypeQ8_0:
		return dequantizeBlockQ8_0(data)
	case GGMLTypeQ8_1:
		return dequantizeBlockQ8_1(data)
	case GGMLTypeQ4_K:
		return dequantizeBlockQ4_K(data)
	case GGMLTypeQ5_K:
		return dequantizeBlockQ5_K(data)
	case GGMLTypeQ6_K:
		return dequantizeBlockQ6_K(data)
	default:
		return nil, fmt.Errorf("unsupported quantized type: %s", dtype)
	}
}

// dequantizeUnquantized handles non-quantized types (F32, F16, etc).
func dequantizeUnquantized(data []byte, dtype GGMLType, numElements int) ([]float32, error) {
	result := make([]float32, numElements)

	switch dtype {
	case GGMLTypeF32:
		for i := 0; i < numElements; i++ {
			result[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
		}

	case GGMLTypeF16:
		for i := 0; i < numElements; i++ {
			h := binary.LittleEndian.Uint16(data[i*2:])
			result[i] = Float16ToFloat32(h)
		}

	case GGMLTypeI8:
		for i := 0; i < numElements; i++ {
			result[i] = float32(int8(data[i])) //nolint:gosec // G115: intentional byte-to-signed reinterpretation for GGML I8 format
		}

	case GGMLTypeI16:
		for i := 0; i < numElements; i++ {
			result[i] = float32(int16(binary.LittleEndian.Uint16(data[i*2:]))) //nolint:gosec // G115: integer overflow conversion uint16 -> int16
		}

	case GGMLTypeI32:
		for i := 0; i < numElements; i++ {
			result[i] = float32(int32(binary.LittleEndian.Uint32(data[i*4:]))) //nolint:gosec // G115: integer overflow conversion uint32 -> int32
		}

	default:
		return nil, fmt.Errorf("unsupported unquantized type: %s", dtype)
	}

	return result, nil
}

// Float16ToFloat32 конвертирует half precision (IEEE 754) в float32.
func Float16ToFloat32(h uint16) float32 {
	// Extract sign, exponent, and mantissa.
	sign := (h >> 15) & 0x1
	exp := (h >> 10) & 0x1F
	mant := h & 0x3FF

	var result uint32

	switch exp {
	case 0:
		if mant == 0 {
			// Zero.
			result = uint32(sign) << 31
		} else {
			// Subnormal F16: value = (-1)^sign * 2^(-14) * (mant / 1024).
			// Convert directly without normalization to avoid uint16 underflow.
			f := float64(mant) / 1024.0 * math.Pow(2, -14)
			if sign == 1 {
				f = -f
			}
			return float32(f)
		}
	case 0x1F:
		// Inf or NaN.
		result = (uint32(sign) << 31) | 0x7F800000 | (uint32(mant) << 13)
	default:
		// Normal number.
		result = (uint32(sign) << 31) | (uint32(exp+127-15) << 23) | (uint32(mant) << 13)
	}

	return math.Float32frombits(result)
}

// Q4_0: 32 elements per block, 4 bits per element.
// Structure: half d (2 bytes), uint8_t qs[16] (16 bytes).
// Formula: x[i] = d * (q[i] - 8) where q[i] is 4-bit value.
func dequantizeBlockQ4_0(data []byte) ([]float32, error) {
	if len(data) < 18 {
		return nil, fmt.Errorf("insufficient data for Q4_0 block: need 18 bytes, got %d", len(data))
	}

	// Read scale factor (half precision).
	d := Float16ToFloat32(binary.LittleEndian.Uint16(data[0:2]))

	// Read quantized values (4 bits each, 2 per byte).
	result := make([]float32, 32)
	for i := 0; i < 16; i++ {
		qByte := data[2+i]

		// Low 4 bits.
		q0 := qByte & 0x0F
		result[i*2] = d * (float32(q0) - 8.0)

		// High 4 bits.
		q1 := qByte >> 4
		result[i*2+1] = d * (float32(q1) - 8.0)
	}

	return result, nil
}

// Q4_1: 32 elements per block, 4 bits per element with offset.
// Structure: half d (2 bytes), half m (2 bytes), uint8_t qs[16] (16 bytes).
// Formula: x[i] = d * q[i] + m.
func dequantizeBlockQ4_1(data []byte) ([]float32, error) {
	if len(data) < 20 {
		return nil, fmt.Errorf("insufficient data for Q4_1 block: need 20 bytes, got %d", len(data))
	}

	d := Float16ToFloat32(binary.LittleEndian.Uint16(data[0:2]))
	m := Float16ToFloat32(binary.LittleEndian.Uint16(data[2:4]))

	result := make([]float32, 32)
	for i := 0; i < 16; i++ {
		qByte := data[4+i]

		q0 := qByte & 0x0F
		result[i*2] = d*float32(q0) + m

		q1 := qByte >> 4
		result[i*2+1] = d*float32(q1) + m
	}

	return result, nil
}

// Q5_0: 32 elements per block, 5 bits per element.
// Structure: half d (2 bytes), uint32_t qh (4 bytes), uint8_t qs[16] (16 bytes).
// qh contains high bits, qs contains low 4 bits.
// Formula: x[i] = d * (q[i] - 16) where q[i] is 5-bit value.
func dequantizeBlockQ5_0(data []byte) ([]float32, error) {
	if len(data) < 22 {
		return nil, fmt.Errorf("insufficient data for Q5_0 block: need 22 bytes, got %d", len(data))
	}

	d := Float16ToFloat32(binary.LittleEndian.Uint16(data[0:2]))
	qh := binary.LittleEndian.Uint32(data[2:6])

	result := make([]float32, 32)
	for i := 0; i < 16; i++ {
		qByte := data[6+i]

		// Reconstruct 5-bit values.
		q0Low := qByte & 0x0F
		q0High := (qh >> i) & 0x1
		q0 := q0Low | (uint8(q0High) << 4)
		result[i*2] = d * (float32(q0) - 16.0)

		q1Low := qByte >> 4
		q1High := (qh >> (i + 16)) & 0x1
		q1 := q1Low | (uint8(q1High) << 4)
		result[i*2+1] = d * (float32(q1) - 16.0)
	}

	return result, nil
}

// Q5_1: 32 elements per block, 5 bits per element with offset.
// Structure: half d (2 bytes), half m (2 bytes), uint32_t qh (4 bytes), uint8_t qs[16] (16 bytes).
// Formula: x[i] = d * q[i] + m.
func dequantizeBlockQ5_1(data []byte) ([]float32, error) {
	if len(data) < 24 {
		return nil, fmt.Errorf("insufficient data for Q5_1 block: need 24 bytes, got %d", len(data))
	}

	d := Float16ToFloat32(binary.LittleEndian.Uint16(data[0:2]))
	m := Float16ToFloat32(binary.LittleEndian.Uint16(data[2:4]))
	qh := binary.LittleEndian.Uint32(data[4:8])

	result := make([]float32, 32)
	for i := 0; i < 16; i++ {
		qByte := data[8+i]

		q0Low := qByte & 0x0F
		q0High := (qh >> i) & 0x1
		q0 := q0Low | (uint8(q0High) << 4)
		result[i*2] = d*float32(q0) + m

		q1Low := qByte >> 4
		q1High := (qh >> (i + 16)) & 0x1
		q1 := q1Low | (uint8(q1High) << 4)
		result[i*2+1] = d*float32(q1) + m
	}

	return result, nil
}

// Q8_0: 32 elements per block, 8 bits per element.
// Structure: half d (2 bytes), int8_t qs[32] (32 bytes).
// Formula: x[i] = d * q[i].
func dequantizeBlockQ8_0(data []byte) ([]float32, error) {
	if len(data) < 34 {
		return nil, fmt.Errorf("insufficient data for Q8_0 block: need 34 bytes, got %d", len(data))
	}

	d := Float16ToFloat32(binary.LittleEndian.Uint16(data[0:2]))

	result := make([]float32, 32)
	for i := 0; i < 32; i++ {
		q := int8(data[2+i]) //nolint:gosec // G115: intentional byte-to-signed reinterpretation for Q8_0 quantized weights
		result[i] = d * float32(q)
	}

	return result, nil
}

// Q8_1: 32 elements per block, 8 bits per element.
// Structure: half d (2 bytes), half s (2 bytes), int8_t qs[32] (32 bytes).
//
// NOTE: The 's' field (s = sum(qs) * d) is stored for optimized dot-product
// kernels inside GGML and is NOT used during standard dequantization.
// Reference: ggml-quants.c dequantize_row_q8_1 — formula is simply x[i] = d * qs[i].
//
// Formula: x[i] = d * q[i].
func dequantizeBlockQ8_1(data []byte) ([]float32, error) {
	if len(data) < 36 {
		return nil, fmt.Errorf("insufficient data for Q8_1 block: need 36 bytes, got %d", len(data))
	}

	d := Float16ToFloat32(binary.LittleEndian.Uint16(data[0:2]))

	// NOTE: s (data[2:4]) = sum(qs)*d is a precomputed dot-product accelerator;
	// it is intentionally not used in element-wise dequantization.

	result := make([]float32, 32)
	for i := 0; i < 32; i++ {
		q := int8(data[4+i]) //nolint:gosec // G115: intentional byte-to-signed reinterpretation for Q8_1 quantized weights
		result[i] = d * float32(q)
	}

	return result, nil
}

// getScaleMinK4 unpacks one scale/min pair from the 12-byte Q4_K/Q5_K scales array.
// This is a direct translation of the GGML get_scale_min_k4 function from ggml-quants.c.
//
// The 12-byte layout stores 8 pairs of 6-bit (scale, min) values:
//
//	q[0..3]:  low 6 bits = sc[0..3]; bits [6:7] = high 2 bits of sc[4..7]
//	q[4..7]:  low 6 bits = m[0..3];  bits [6:7] = high 2 bits of m[4..7]
//	q[8..11]: low nibble = low 4 bits of sc[4..7]; high nibble = low 4 bits of m[4..7]
func getScaleMinK4(j int, q []byte) (sc, m uint8) {
	if j < 4 {
		sc = q[j] & 63
		m = q[j+4] & 63
	} else {
		sc = (q[j+4] & 0xF) | ((q[j-4] >> 6) << 4)
		m = (q[j+4] >> 4) | ((q[j-0] >> 6) << 4)
	}
	return
}

// Q4_K: 256 elements per block, 4-bit K-quant with super-block scales.
// Structure:
//
//	half d (2 bytes) - super-block scale
//	half dmin (2 bytes) - super-block minimum
//	uint8_t scales[12] (12 bytes) - packed 6-bit scales/mins via get_scale_min_k4
//	uint8_t qs[128] (128 bytes) - 4-bit quantized values
//
// Element ordering (matches ggml-quants.c dequantize_row_q4_K exactly):
// 4 groups of 64 elements. Each group uses TWO scale pairs (is and is+1):
//   - First 32 elements: lo nibbles of qs[group*32..(group+1)*32-1], scaled by d1/m1
//   - Next  32 elements: hi nibbles of the same qs range, scaled by d2/m2
//
//nolint:revive // Function name matches GGML specification (Q4_K format).
func dequantizeBlockQ4_K(data []byte) ([]float32, error) {
	if len(data) < 144 {
		return nil, fmt.Errorf("insufficient data for Q4_K block: need 144 bytes, got %d", len(data))
	}

	d := float64(Float16ToFloat32(binary.LittleEndian.Uint16(data[0:2])))
	dmin := float64(Float16ToFloat32(binary.LittleEndian.Uint16(data[2:4])))

	// scales[12] at bytes 4..15, indexed as q[0..11] by getScaleMinK4.
	q := data[4:16]
	// qs[128] at bytes 16..143.
	qptr := data[16:]

	result := make([]float32, 256)
	y := 0
	is := 0

	// 4 outer groups of 64 elements, each group advancing qptr by 32 bytes.
	for j := 0; j < 4; j++ {
		sc0, m0 := getScaleMinK4(is+0, q)
		sc1, m1 := getScaleMinK4(is+1, q)
		d1 := d * float64(sc0)
		m1v := dmin * float64(m0)
		d2 := d * float64(sc1)
		m2v := dmin * float64(m1)

		qOffset := j * 32

		// First 32 outputs: lo nibbles, scaled by d1/m1.
		for l := 0; l < 32; l++ {
			result[y] = float32(d1*float64(qptr[qOffset+l]&0xF) - m1v)
			y++
		}
		// Next 32 outputs: hi nibbles, scaled by d2/m2.
		for l := 0; l < 32; l++ {
			result[y] = float32(d2*float64(qptr[qOffset+l]>>4) - m2v)
			y++
		}
		is += 2
	}

	return result, nil
}

// Q5_K: 256 elements per block, 5-bit K-quant.
// Structure:
//
//	half d (2 bytes)
//	half dmin (2 bytes)
//	uint8_t scales[12] (12 bytes) - packed 6-bit scales/mins via get_scale_min_k4
//	uint8_t qh[32] (32 bytes) - high bits, bit-packed: qh[elem/8] bit (elem%8)
//	uint8_t qs[128] (128 bytes) - low 4 bits
//
// Element ordering (matches ggml-quants.c dequantize_row_q5_K exactly):
// 4 groups of 64 elements. Each group uses TWO scale pairs (is and is+1):
//   - First 32 outputs: lo nibbles of qs[group*32..], high bit from qh[elem/8], scaled by d1/m1
//   - Next  32 outputs: hi nibbles of the same qs range, high bit from qh[elem/8], scaled by d2/m2
//
//nolint:revive // Function name matches GGML specification (Q5_K format).
func dequantizeBlockQ5_K(data []byte) ([]float32, error) {
	if len(data) < 176 {
		return nil, fmt.Errorf("insufficient data for Q5_K block: need 176 bytes, got %d", len(data))
	}

	d := float64(Float16ToFloat32(binary.LittleEndian.Uint16(data[0:2])))
	dmin := float64(Float16ToFloat32(binary.LittleEndian.Uint16(data[2:4])))

	// scales[12] at bytes 4..15 — same get_scale_min_k4 layout as Q4_K.
	q := data[4:16]
	// qh[32] at bytes 16..47: 1 bit per element, packed as qh[elem/8] bit (elem%8).
	qh := data[16:48]
	// qs[128] at bytes 48..175: lo 4 bits per element.
	ql := data[48:]

	result := make([]float32, 256)
	y := 0
	is := 0

	// 4 outer groups of 64 elements, each group advancing ql by 32 bytes.
	for j := 0; j < 4; j++ {
		sc0, m0 := getScaleMinK4(is+0, q)
		sc1, m1 := getScaleMinK4(is+1, q)
		d1 := d * float64(sc0)
		m1v := dmin * float64(m0)
		d2 := d * float64(sc1)
		m2v := dmin * float64(m1)

		qlOffset := j * 32

		// First 32 outputs: lo nibbles + high bit from qh, scaled by d1/m1.
		for l := 0; l < 32; l++ {
			elemGlobal := j*64 + l
			highBit := (qh[elemGlobal/8] >> uint(elemGlobal%8)) & 1
			q5 := (ql[qlOffset+l] & 0xF) | (highBit << 4)
			result[y] = float32(d1*float64(q5) - m1v)
			y++
		}
		// Next 32 outputs: hi nibbles + high bit from qh, scaled by d2/m2.
		for l := 0; l < 32; l++ {
			elemGlobal := j*64 + 32 + l
			highBit := (qh[elemGlobal/8] >> uint(elemGlobal%8)) & 1
			q5 := (ql[qlOffset+l] >> 4) | (highBit << 4)
			result[y] = float32(d2*float64(q5) - m2v)
			y++
		}
		is += 2
	}

	return result, nil
}

// Q6_K: 256 elements per block, 6-bit K-quant.
//
// Structure (210 bytes total):
//
//	uint8_t ql[128]   - low 4 bits of each quant (2 elements per byte)
//	uint8_t qh[64]    - high 2 bits of each quant (4 elements per byte)
//	int8_t  scales[16] - signed scales, one per 16-element sub-block
//	half    d          - super-block scale (F16), at byte 208
//
// Formula: x[i] = d * scales[sub] * (q[i] - 32)
// where q[i] = (ql_nibble[i] | (qh_bits[i] << 4)), range 0..63.
//
// Element layout matches ggml-quants.c dequantize_row_q6_K exactly:
// Two passes (n=0,128), each producing 128 outputs from 64 ql bytes and 32 qh bytes.
// Within each pass, inner loop l=0..31 produces 4 outputs per iteration:
//
//	y[l],    y[l+32]  — use ql[l] lo nibble and ql[l+32] lo nibble respectively
//	y[l+64], y[l+96]  — use ql[l] hi nibble and ql[l+32] hi nibble respectively
//
// The 4 outputs within one l share the SAME qh byte (qh[l>>2]) but use different
// 2-bit sub-fields of that byte (via bit-shift = 2*(l&3) for y[l]/y[l+32]).
// For y[l+64]/y[l+96], the shift is 2*(l&3)+4; in C/Go integer arithmetic this
// gives 0 for l=2,3 (shift 8,10 > 7) — identical to the GGML C reference.
//
// Scale indices: y[l] uses sc[l/16], y[l+32] uses sc[l/16+2],
//
//	y[l+64] uses sc[l/16+4], y[l+96] uses sc[l/16+6].
//
//nolint:revive // Function name matches GGML specification (Q6_K format).
func dequantizeBlockQ6_K(data []byte) ([]float32, error) {
	if len(data) < 210 {
		return nil, fmt.Errorf("insufficient data for Q6_K block: need 210 bytes, got %d", len(data))
	}

	d := Float16ToFloat32(binary.LittleEndian.Uint16(data[208:210]))
	result := make([]float32, 256)

	// Two passes of 128 elements each, matching the GGML reference loop structure.
	// Block byte layout: ql[0..127] | qh[128..191] | scales[192..207] | d[208..209]
	for n := 0; n < 256; n += 128 {
		qlBase := n / 2     // ql section byte offset: 0, then 64
		qhBase := 128 + n/4 // qh section starts at byte 128; pass offset: 0, then 32
		scBase := n / 16    // scales section starts at byte 192 (added below); offset: 0, then 8
		yBase := n

		for l := 0; l < 32; l++ {
			is := l / 16
			qhByte := int(data[qhBase+l])

			// Each of q1..q4 uses a DIFFERENT 2-bit field from qh:
			//   q1: bits [0:1], q2: bits [2:3], q3: bits [4:5], q4: bits [6:7]
			qLoA := int(data[qlBase+l]) & 0xF
			qLoB := int(data[qlBase+l+32]) & 0xF
			qHiA := int(data[qlBase+l]) >> 4
			qHiB := int(data[qlBase+l+32]) >> 4

			q1 := int8((qLoA | (((qhByte >> 0) & 3) << 4)) - 32) //nolint:gosec // G115: 6-bit range
			q2 := int8((qLoB | (((qhByte >> 2) & 3) << 4)) - 32) //nolint:gosec // G115: 6-bit range
			q3 := int8((qHiA | (((qhByte >> 4) & 3) << 4)) - 32) //nolint:gosec // G115: 6-bit range
			q4 := int8((qHiB | (((qhByte >> 6) & 3) << 4)) - 32) //nolint:gosec // G115: 6-bit range

			sc1 := float32(int8(data[192+scBase+is]))   //nolint:gosec // G115: signed scale byte
			sc2 := float32(int8(data[192+scBase+is+2])) //nolint:gosec // G115: signed scale byte
			sc3 := float32(int8(data[192+scBase+is+4])) //nolint:gosec // G115: signed scale byte
			sc4 := float32(int8(data[192+scBase+is+6])) //nolint:gosec // G115: signed scale byte

			result[yBase+l] = d * sc1 * float32(q1)
			result[yBase+l+32] = d * sc2 * float32(q2)
			result[yBase+l+64] = d * sc3 * float32(q3)
			result[yBase+l+96] = d * sc4 * float32(q4)
		}
	}

	return result, nil
}
