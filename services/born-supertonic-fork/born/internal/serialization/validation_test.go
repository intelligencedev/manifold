package serialization

import (
	"errors"
	"strings"
	"testing"
)

// TestValidateTensorOffsets_NoOverlap verifies that valid tensors pass validation.
func TestValidateTensorOffsets_NoOverlap(t *testing.T) {
	tensors := []TensorMeta{
		{Name: "tensor1", Offset: 0, Size: 100},
		{Name: "tensor2", Offset: 100, Size: 200},
		{Name: "tensor3", Offset: 300, Size: 150},
	}
	dataSize := int64(500)

	err := ValidateTensorOffsets(tensors, dataSize)
	if err != nil {
		t.Errorf("Expected no error for valid tensors, got: %v", err)
	}
}

// TestValidateTensorOffsets_Overlap detects overlapping tensor regions.
func TestValidateTensorOffsets_Overlap(t *testing.T) {
	tests := []struct {
		name     string
		tensors  []TensorMeta
		dataSize int64
		wantErr  bool
	}{
		{
			name: "complete overlap",
			tensors: []TensorMeta{
				{Name: "tensor1", Offset: 0, Size: 100},
				{Name: "tensor2", Offset: 50, Size: 100}, // Overlaps with tensor1
			},
			dataSize: 200,
			wantErr:  true,
		},
		{
			name: "partial overlap at boundary",
			tensors: []TensorMeta{
				{Name: "tensor1", Offset: 0, Size: 100},
				{Name: "tensor2", Offset: 99, Size: 100}, // Overlaps by 1 byte
			},
			dataSize: 200,
			wantErr:  true,
		},
		{
			name: "exact boundary (no overlap)",
			tensors: []TensorMeta{
				{Name: "tensor1", Offset: 0, Size: 100},
				{Name: "tensor2", Offset: 100, Size: 100}, // Starts exactly where tensor1 ends
			},
			dataSize: 200,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTensorOffsets(tt.tensors, tt.dataSize)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTensorOffsets() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				var validationErr *ValidationError
				if !errors.As(err, &validationErr) {
					t.Errorf("Expected ValidationError, got %T", err)
				}
				if validationErr != nil && validationErr.Type != "offset_overlap" && tt.wantErr {
					t.Errorf("Expected offset_overlap error, got %s", validationErr.Type)
				}
			}
		})
	}
}

// TestValidateTensorOffsets_OutOfBounds detects tensors extending beyond data section.
func TestValidateTensorOffsets_OutOfBounds(t *testing.T) {
	tests := []struct {
		name     string
		tensors  []TensorMeta
		dataSize int64
		wantErr  bool
	}{
		{
			name: "tensor extends beyond data",
			tensors: []TensorMeta{
				{Name: "tensor1", Offset: 0, Size: 100},
				{Name: "tensor2", Offset: 100, Size: 200}, // Ends at 300, but dataSize is 250
			},
			dataSize: 250,
			wantErr:  true,
		},
		{
			name: "large offset beyond data",
			tensors: []TensorMeta{
				{Name: "tensor1", Offset: 1000, Size: 100}, // Starts beyond dataSize
			},
			dataSize: 500,
			wantErr:  true,
		},
		{
			name: "tensor fits exactly",
			tensors: []TensorMeta{
				{Name: "tensor1", Offset: 0, Size: 500},
			},
			dataSize: 500,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTensorOffsets(tt.tensors, tt.dataSize)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTensorOffsets() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.wantErr {
				var validationErr *ValidationError
				if !errors.As(err, &validationErr) {
					t.Errorf("Expected ValidationError, got %T", err)
				}
				if validationErr != nil && validationErr.Type != "out_of_bounds" {
					t.Errorf("Expected out_of_bounds error, got %s", validationErr.Type)
				}
			}
		})
	}
}

// TestValidateTensorOffsets_NegativeValues detects negative offsets or sizes.
func TestValidateTensorOffsets_NegativeValues(t *testing.T) {
	tests := []struct {
		name     string
		tensors  []TensorMeta
		dataSize int64
	}{
		{
			name: "negative offset",
			tensors: []TensorMeta{
				{Name: "tensor1", Offset: -100, Size: 100},
			},
			dataSize: 500,
		},
		{
			name: "negative size",
			tensors: []TensorMeta{
				{Name: "tensor1", Offset: 0, Size: -100},
			},
			dataSize: 500,
		},
		{
			name: "both negative",
			tensors: []TensorMeta{
				{Name: "tensor1", Offset: -100, Size: -100},
			},
			dataSize: 500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTensorOffsets(tt.tensors, tt.dataSize)
			if err == nil {
				t.Errorf("Expected error for negative values, got nil")
			}
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Errorf("Expected ValidationError, got %T", err)
			}
			if validationErr != nil && validationErr.Type != "negative_offset" {
				t.Errorf("Expected negative_offset error, got %s", validationErr.Type)
			}
		})
	}
}

// TestValidateTensorOffsets_TooManyTensors prevents DoS via excessive tensor count.
func TestValidateTensorOffsets_TooManyTensors(t *testing.T) {
	// Create MaxTensorCount + 1 tensors
	tensors := make([]TensorMeta, MaxTensorCount+1)
	for i := range tensors {
		tensors[i] = TensorMeta{
			Name:   "tensor",
			Offset: int64(i * 100),
			Size:   100,
		}
	}
	dataSize := int64((MaxTensorCount + 1) * 100)

	err := ValidateTensorOffsets(tensors, dataSize)
	if err == nil {
		t.Errorf("Expected error for too many tensors, got nil")
	}
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Errorf("Expected ValidationError, got %T", err)
	}
	if validationErr != nil && validationErr.Type != "too_many_tensors" {
		t.Errorf("Expected too_many_tensors error, got %s", validationErr.Type)
	}
}

// TestValidateTensorName_PathTraversal prevents directory traversal attacks.
func TestValidateTensorName_PathTraversal(t *testing.T) {
	badNames := []string{
		"../../../etc/passwd",
		"..\\..\\windows\\system32",
		"tensor/../secret",
		"layer/0/weight",
		"model\\layer\\weight",
		"tensor\x00hidden",                      // Null byte injection
		strings.Repeat("a", MaxTensorNameLen+1), // Too long
	}

	for _, name := range badNames {
		t.Run(name, func(t *testing.T) {
			err := ValidateTensorName(name)
			if err == nil {
				t.Errorf("Expected error for malicious name %q, got nil", name)
			}
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Errorf("Expected ValidationError, got %T", err)
			}
			if validationErr != nil {
				// Should be one of: invalid_name, name_too_long
				if validationErr.Type != "invalid_name" && validationErr.Type != "name_too_long" {
					t.Errorf("Expected invalid_name or name_too_long error, got %s", validationErr.Type)
				}
			}
		})
	}
}

// TestValidateTensorName_ValidNames ensures valid names are accepted.
func TestValidateTensorName_ValidNames(t *testing.T) {
	validNames := []string{
		"tensor",
		"layer.0.weight",
		"model_layer_0_bias",
		"embedding-matrix",
		"output:logits",
		"UPPERCASE",
		"with_numbers_123",
	}

	for _, name := range validNames {
		t.Run(name, func(t *testing.T) {
			err := ValidateTensorName(name)
			if err != nil {
				t.Errorf("Expected no error for valid name %q, got: %v", name, err)
			}
		})
	}
}

// TestValidateHeader_Strict tests strict validation mode.
func TestValidateHeader_Strict(t *testing.T) {
	tests := []struct {
		name     string
		header   Header
		dataSize int64
		wantErr  bool
	}{
		{
			name: "valid header",
			header: Header{
				Tensors: []TensorMeta{
					{Name: "tensor1", Offset: 0, Size: 100},
					{Name: "tensor2", Offset: 100, Size: 100},
				},
			},
			dataSize: 200,
			wantErr:  false,
		},
		{
			name: "overlapping tensors",
			header: Header{
				Tensors: []TensorMeta{
					{Name: "tensor1", Offset: 0, Size: 100},
					{Name: "tensor2", Offset: 50, Size: 100},
				},
			},
			dataSize: 200,
			wantErr:  true,
		},
		{
			name: "invalid tensor name",
			header: Header{
				Tensors: []TensorMeta{
					{Name: "../malicious", Offset: 0, Size: 100},
				},
			},
			dataSize: 100,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHeader(&tt.header, tt.dataSize, ValidationStrict)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHeader() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateHeader_Normal tests normal validation mode (skips offset checks).
func TestValidateHeader_Normal(t *testing.T) {
	// This should pass normal validation (only name checks)
	// but would fail strict validation (offset overlap)
	header := Header{
		Tensors: []TensorMeta{
			{Name: "tensor1", Offset: 0, Size: 100},
			{Name: "tensor2", Offset: 50, Size: 100}, // Overlaps
		},
	}
	dataSize := int64(200)

	// Normal mode: should pass (no offset validation)
	err := ValidateHeader(&header, dataSize, ValidationNormal)
	if err != nil {
		t.Errorf("Normal validation should pass, got error: %v", err)
	}

	// Strict mode: should fail (offset overlap)
	err = ValidateHeader(&header, dataSize, ValidationStrict)
	if err == nil {
		t.Errorf("Strict validation should fail on overlap")
	}
}

// TestValidateHeader_None tests disabled validation.
func TestValidateHeader_None(t *testing.T) {
	// Even completely invalid header should pass with ValidationNone
	header := Header{
		Tensors: []TensorMeta{
			{Name: "../../../etc/passwd", Offset: -1000, Size: -1000},
		},
	}
	dataSize := int64(100)

	err := ValidateHeader(&header, dataSize, ValidationNone)
	if err != nil {
		t.Errorf("ValidationNone should skip all checks, got error: %v", err)
	}
}

// TestValidationError_ErrorMessages verifies error message formatting.
func TestValidationError_ErrorMessages(t *testing.T) {
	tests := []struct {
		name     string
		err      *ValidationError
		expected string
	}{
		{
			name: "single tensor error",
			err: &ValidationError{
				Type:    "out_of_bounds",
				Tensor:  "layer1",
				Details: "offset 100 + size 200 > data_size 250",
			},
			expected: `out_of_bounds: tensor "layer1": offset 100 + size 200 > data_size 250`,
		},
		{
			name: "two tensor error (overlap)",
			err: &ValidationError{
				Type:    "offset_overlap",
				Tensor:  "tensor1",
				Tensor2: "tensor2",
				Details: "regions [0-100] and [50-150] overlap",
			},
			expected: `offset_overlap: tensors "tensor1" and "tensor2": regions [0-100] and [50-150] overlap`,
		},
		{
			name: "general error (no tensor)",
			err: &ValidationError{
				Type:    "too_many_tensors",
				Details: "got 100001, max 100000",
			},
			expected: "too_many_tensors: got 100001, max 100000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.err.Error()
			if actual != tt.expected {
				t.Errorf("Error message mismatch\nExpected: %s\nGot:      %s", tt.expected, actual)
			}
		})
	}
}

// FuzzValidateTensorName ensures name validation never panics on random input.
func FuzzValidateTensorName(f *testing.F) {
	// Seed with interesting test cases
	f.Add("normal_tensor_name")
	f.Add("../malicious")
	f.Add("path/to/tensor")
	f.Add(strings.Repeat("a", MaxTensorNameLen))
	f.Add("\x00null_byte")
	f.Add("..\\windows")

	f.Fuzz(func(_ *testing.T, name string) {
		// Should never panic - only return error or nil
		_ = ValidateTensorName(name)
	})
}

// FuzzValidateTensorOffsets ensures offset validation never panics.
func FuzzValidateTensorOffsets(f *testing.F) {
	// Seed with interesting test cases
	f.Add(int64(0), int64(100), int64(200))
	f.Add(int64(-100), int64(50), int64(1000))
	f.Add(int64(100), int64(-50), int64(1000))

	f.Fuzz(func(_ *testing.T, offset1, size1, dataSize int64) {
		tensors := []TensorMeta{
			{Name: "fuzz_tensor", Offset: offset1, Size: size1},
		}
		// Should never panic
		_ = ValidateTensorOffsets(tensors, dataSize)
	})
}
