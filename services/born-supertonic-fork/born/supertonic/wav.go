package supertonic

import (
	"encoding/binary"
	"math"
)

// FloatToPCM16 converts a float32 waveform to little-endian signed 16-bit PCM,
// matching helper.mjs writeWavFile (clamp to [-1,1], scale by 32767, floor).
func FloatToPCM16(wav []float32) []byte {
	out := make([]byte, len(wav)*2)
	for i, v := range wav {
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		s := int16(math.Floor(float64(v) * 32767.0))
		binary.LittleEndian.PutUint16(out[i*2:], uint16(s))
	}
	return out
}

// WAVBytes wraps PCM16 bytes in a minimal mono WAV container.
func WAVBytes(pcm []byte, sampleRate int) []byte {
	const channels, bits = 1, 16
	byteRate := sampleRate * channels * bits / 8
	blockAlign := channels * bits / 8
	dataSize := len(pcm)
	buf := make([]byte, 44+dataSize)
	copy(buf[0:], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:], uint32(36+dataSize))
	copy(buf[8:], "WAVE")
	copy(buf[12:], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:], 16)
	binary.LittleEndian.PutUint16(buf[20:], 1)
	binary.LittleEndian.PutUint16(buf[22:], channels)
	binary.LittleEndian.PutUint32(buf[24:], uint32(sampleRate))
	binary.LittleEndian.PutUint32(buf[28:], uint32(byteRate))
	binary.LittleEndian.PutUint16(buf[32:], uint16(blockAlign))
	binary.LittleEndian.PutUint16(buf[34:], bits)
	copy(buf[36:], "data")
	binary.LittleEndian.PutUint32(buf[40:], uint32(dataSize))
	copy(buf[44:], pcm)
	return buf
}
