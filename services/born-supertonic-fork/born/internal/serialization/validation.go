package serialization

import (
	"fmt"
	"sort"
	"strings"
)

// Validation limits for security and resource protection.
const (
	MaxHeaderSize    = 100 * 1024 * 1024 // 100MB - maximum header size
	MaxTensorCount   = 100_000           // Maximum number of tensors in a file
	MaxTensorNameLen = 4096              // Maximum tensor name length
	MaxMetadataSize  = 10 * 1024 * 1024  // 10MB - maximum metadata size
)

// ValidationError type strings.
const (
	errTypeTooManyTensors = "too_many_tensors"
	errTypeOutOfBounds    = "out_of_bounds"
	errTypeOffsetOverlap  = "offset_overlap"
	errTypeInvalidName    = "invalid_name"
)

// ValidationLevel controls the strictness of validation.
type ValidationLevel int

const (
	// ValidationStrict performs all validation checks (default, recommended for production).
	ValidationStrict ValidationLevel = iota
	// ValidationNormal performs basic validation checks only.
	ValidationNormal
	// ValidationNone skips validation (dangerous! Use only with trusted input).
	ValidationNone
)

// ValidateTensorOffsets checks for overlapping tensor offsets and out-of-bounds access.
// This is critical for security - malformed files could cause memory corruption or data leakage.
func ValidateTensorOffsets(tensors []TensorMeta, dataSize int64) error {
	if len(tensors) > MaxTensorCount {
		return &ValidationError{
			Type:    errTypeTooManyTensors,
			Details: fmt.Sprintf("got %d, max %d", len(tensors), MaxTensorCount),
		}
	}

	// Sort tensors by offset for efficient overlap detection.
	sorted := make([]TensorMeta, len(tensors))
	copy(sorted, tensors)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Offset < sorted[j].Offset
	})

	for i, t := range sorted {
		// Check for negative values (potential integer overflow attacks).
		if t.Offset < 0 || t.Size < 0 {
			return &ValidationError{
				Type:    "negative_offset",
				Tensor:  t.Name,
				Details: fmt.Sprintf("offset=%d, size=%d (negative values not allowed)", t.Offset, t.Size),
			}
		}

		// Check bounds - prevent reading beyond file.
		if t.Offset+t.Size > dataSize {
			return &ValidationError{
				Type:    errTypeOutOfBounds,
				Tensor:  t.Name,
				Details: fmt.Sprintf("offset %d + size %d > data_size %d", t.Offset, t.Size, dataSize),
			}
		}

		// Check for overlap with next tensor (data leakage prevention).
		if i < len(sorted)-1 {
			next := sorted[i+1]
			if t.Offset+t.Size > next.Offset {
				return &ValidationError{
					Type:    errTypeOffsetOverlap,
					Tensor:  t.Name,
					Tensor2: next.Name,
					Details: fmt.Sprintf("regions [%d-%d] and [%d-%d] overlap",
						t.Offset, t.Offset+t.Size, next.Offset, next.Offset+next.Size),
				}
			}
		}
	}

	return nil
}

// ValidateTensorName checks tensor names for path traversal attacks and malicious patterns.
func ValidateTensorName(name string) error {
	if len(name) > MaxTensorNameLen {
		return &ValidationError{
			Type:    "name_too_long",
			Tensor:  name,
			Details: fmt.Sprintf("length %d > max %d", len(name), MaxTensorNameLen),
		}
	}

	// Path traversal prevention - critical for security.
	if strings.Contains(name, "..") {
		return &ValidationError{
			Type:    errTypeInvalidName,
			Tensor:  name,
			Details: "contains '..' (path traversal attempt)",
		}
	}

	// Prevent absolute paths and directory separators.
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return &ValidationError{
			Type:    errTypeInvalidName,
			Tensor:  name,
			Details: "contains path separator (/ or \\)",
		}
	}

	// Prevent null bytes (can bypass length checks in some contexts).
	if strings.Contains(name, "\x00") {
		return &ValidationError{
			Type:    errTypeInvalidName,
			Tensor:  name,
			Details: "contains null byte",
		}
	}

	return nil
}

// ValidateHeader performs comprehensive header validation.
func ValidateHeader(h *Header, dataSize int64, level ValidationLevel) error {
	if level == ValidationNone {
		return nil
	}

	// Validate tensor count (DoS prevention).
	if len(h.Tensors) > MaxTensorCount {
		return &ValidationError{
			Type:    errTypeTooManyTensors,
			Details: fmt.Sprintf("got %d, max %d", len(h.Tensors), MaxTensorCount),
		}
	}

	// Validate all tensor names.
	for _, t := range h.Tensors {
		if err := ValidateTensorName(t.Name); err != nil {
			return err
		}
	}

	// Validate offsets (only in strict mode - performance-intensive).
	if level == ValidationStrict {
		if err := ValidateTensorOffsets(h.Tensors, dataSize); err != nil {
			return err
		}
	}

	return nil
}
