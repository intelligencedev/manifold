package documents

import "math/bits"

// Distance returns the Hamming distance between two 64-bit hashes.
func Distance(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}
