package serialization

import (
	"errors"
	"fmt"
)

// Common errors.
var (
	ErrChecksumMismatch   = errors.New("checksum mismatch: file may be corrupted")
	ErrOffsetOverlap      = errors.New("tensor offsets overlap")
	ErrOutOfBounds        = errors.New("tensor extends beyond data section")
	ErrNegativeOffset     = errors.New("negative offset or size")
	ErrTooManyTensors     = errors.New("too many tensors in file")
	ErrTensorNameTooLong  = errors.New("tensor name too long")
	ErrInvalidTensorName  = errors.New("invalid tensor name")
	ErrHeaderTooLarge     = errors.New("header exceeds maximum size")
	ErrInvalidMagic       = errors.New("invalid magic bytes")
	ErrUnsupportedVersion = errors.New("unsupported format version")
)

// ValidationError provides detailed information about validation failures.
type ValidationError struct {
	Type    string // Type of error (e.g., "offset_overlap", "out_of_bounds")
	Tensor  string // Primary tensor name involved
	Tensor2 string // Secondary tensor name (for overlap errors)
	Details string // Additional details
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	if e.Tensor2 != "" {
		return fmt.Sprintf("%s: tensors %q and %q: %s", e.Type, e.Tensor, e.Tensor2, e.Details)
	}
	if e.Tensor != "" {
		return fmt.Sprintf("%s: tensor %q: %s", e.Type, e.Tensor, e.Details)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Details)
}
