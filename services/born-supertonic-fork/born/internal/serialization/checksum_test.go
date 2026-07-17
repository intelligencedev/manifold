package serialization

import (
	"bytes"
	"errors"
	"testing"
)

// TestComputeChecksum verifies SHA-256 checksum computation.
func TestComputeChecksum(t *testing.T) {
	data := []byte("test data")
	checksum1 := ComputeChecksum(data)
	checksum2 := ComputeChecksum(data)

	// Same data should produce same checksum
	if checksum1 != checksum2 {
		t.Error("Checksums should match for identical data")
	}

	// Different data should produce different checksum
	differentData := []byte("different data")
	checksum3 := ComputeChecksum(differentData)
	if checksum1 == checksum3 {
		t.Error("Checksums should differ for different data")
	}

	// Checksum should be 32 bytes (SHA-256)
	if len(checksum1) != 32 {
		t.Errorf("Expected checksum length 32, got %d", len(checksum1))
	}
}

// TestComputeChecksumReader verifies checksum computation from reader.
func TestComputeChecksumReader(t *testing.T) {
	data := []byte("test data for reader")
	reader := bytes.NewReader(data)

	checksum, err := ComputeChecksumReader(reader)
	if err != nil {
		t.Fatalf("ComputeChecksumReader failed: %v", err)
	}

	// Should match direct computation
	expected := ComputeChecksum(data)
	if checksum != expected {
		t.Error("Reader checksum should match direct checksum")
	}
}

// TestValidateChecksum verifies checksum validation.
func TestValidateChecksum(t *testing.T) {
	data := []byte("test data")
	checksum := ComputeChecksum(data)

	// Valid checksum should pass
	if err := ValidateChecksum(checksum, checksum); err != nil {
		t.Errorf("Expected no error for matching checksums, got: %v", err)
	}

	// Invalid checksum should fail
	wrongChecksum := [32]byte{1, 2, 3, 4, 5, 6, 7, 8}
	err := ValidateChecksum(checksum, wrongChecksum)
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Errorf("Expected ErrChecksumMismatch, got: %v", err)
	}
}

// TestKnownVectorSHA256 verifies SHA-256 produces correct known vectors.
func TestKnownVectorSHA256(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string // hex representation
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:     "hello world",
			input:    "hello world",
			expected: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checksum := ComputeChecksum([]byte(tt.input))
			got := hexEncode(checksum[:])
			if got != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, got)
			}
		})
	}
}

// hexEncode encodes bytes to hex string.
func hexEncode(b []byte) string {
	const hexChars = "0123456789abcdef"
	result := make([]byte, len(b)*2)
	for i, v := range b {
		result[i*2] = hexChars[v>>4]
		result[i*2+1] = hexChars[v&0x0f]
	}
	return string(result)
}
