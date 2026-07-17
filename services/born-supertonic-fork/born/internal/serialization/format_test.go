package serialization

import (
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

// TestDtypeRoundTrip verifies that every supported dtype survives a
// dtypeToString -> stringToDtype round-trip unchanged. A silent mismatch here
// would mislabel tensor data on write and misread it on load.
func TestDtypeRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		dtype tensor.DataType
		str   string
	}{
		{"float32", tensor.Float32, DTypeFloat32},
		{"float64", tensor.Float64, DTypeFloat64},
		{"int32", tensor.Int32, DTypeInt32},
		{"int64", tensor.Int64, DTypeInt64},
		{"uint8", tensor.Uint8, DTypeUint8},
		{"bool", tensor.Bool, DTypeBool},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dtypeToString(tt.dtype); got != tt.str {
				t.Errorf("dtypeToString(%v) = %q, want %q", tt.dtype, got, tt.str)
			}
			got, ok := stringToDtype(tt.str)
			if !ok {
				t.Fatalf("stringToDtype(%q) returned ok=false", tt.str)
			}
			if got != tt.dtype {
				t.Errorf("stringToDtype(%q) = %v, want %v", tt.str, got, tt.dtype)
			}
		})
	}
}

// TestDtypeToStringUnknown verifies the default arm names an unrecognized dtype.
func TestDtypeToStringUnknown(t *testing.T) {
	if got := dtypeToString(tensor.DataType(99)); got != "unknown" {
		t.Errorf("dtypeToString(unknown) = %q, want %q", got, "unknown")
	}
}

// TestStringToDtypeUnknown verifies an unrecognized string is rejected rather
// than mapped to a zero-value dtype. float16 is a real type Born does not carry.
func TestStringToDtypeUnknown(t *testing.T) {
	if _, ok := stringToDtype("float16"); ok {
		t.Error("stringToDtype(\"float16\") = ok true, want false")
	}
}
