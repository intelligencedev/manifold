package serialization

import (
	"crypto/sha256"
	"io"
)

// ComputeChecksum computes SHA-256 checksum of data.
func ComputeChecksum(data []byte) [32]byte {
	return sha256.Sum256(data)
}

// ComputeChecksumReader computes SHA-256 checksum from an io.Reader.
// This is useful for computing checksums of large files without loading them entirely into memory.
func ComputeChecksumReader(r io.Reader) ([32]byte, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return [32]byte{}, err
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

// ValidateChecksum compares computed checksum against stored checksum.
// Returns ErrChecksumMismatch if they don't match.
func ValidateChecksum(computed, stored [32]byte) error {
	if computed != stored {
		return ErrChecksumMismatch
	}
	return nil
}
