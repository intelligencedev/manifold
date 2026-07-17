// Package tensor provides the core tensor types and operations for Born ML framework.
package tensor

import "fmt"

// DType is a constraint for supported tensor data types.
// It uses Go generics to ensure compile-time type safety.
type DType interface {
	~float32 | ~float64 | ~int32 | ~int64 | ~uint8 | ~bool
}

// DataType represents runtime type information for tensors.
type DataType int

// Supported data types for tensors.
const (
	Float32 DataType = iota
	Float64
	Int32
	Int64
	Uint8
	Bool
)

// Size returns the byte size of the data type.
func (dt DataType) Size() int {
	switch dt {
	case Float32, Int32:
		return 4
	case Float64, Int64:
		return 8
	case Uint8, Bool:
		return 1
	default:
		panic("unknown data type")
	}
}

// String returns a human-readable name for the data type.
func (dt DataType) String() string {
	switch dt {
	case Float32:
		return "float32"
	case Float64:
		return "float64"
	case Int32:
		return "int32"
	case Int64:
		return "int64"
	case Uint8:
		return "uint8"
	case Bool:
		return "bool"
	default:
		return "unknown"
	}
}

// inferDataType infers DataType from a generic type T.
func inferDataType[T DType](dummy T) DataType {
	switch any(dummy).(type) {
	case float32:
		return Float32
	case float64:
		return Float64
	case int32:
		return Int32
	case int64:
		return Int64
	case uint8:
		return Uint8
	case bool:
		return Bool
	default:
		panic("unsupported type")
	}
}

// CheckScalarDType checks if the value matches the expected type.
//
// Panics if they do not match.
func CheckScalarDType[T DType](val any) T {
	casted, ok := val.(T)
	if !ok {
		panic(fmt.Sprintf("tensor: expected %T, got %T", new(T), val))
	}

	return casted
}
