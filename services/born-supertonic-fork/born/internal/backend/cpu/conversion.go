package cpu

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// Cast converts the tensor to a different data type.
func (cpu *CPUBackend) Cast(x *tensor.RawTensor, dtype tensor.DataType) *tensor.RawTensor {
	// No-op if same dtype
	if x.DType() == dtype {
		return x
	}

	result, err := tensor.NewRaw(x.Shape(), dtype, cpu.device)
	if err != nil {
		panic(fmt.Sprintf("cast: %v", err))
	}

	castImpl(result, x, dtype)

	return result
}

func castImpl(result, x *tensor.RawTensor, toDtype tensor.DataType) {
	fromDtype := x.DType()

	// Dispatch based on from/to types
	switch fromDtype {
	case tensor.Float32:
		castFromFloat32(result, x, toDtype)
	case tensor.Float64:
		castFromFloat64(result, x, toDtype)
	case tensor.Int32:
		castFromInt32(result, x, toDtype)
	case tensor.Int64:
		castFromInt64(result, x, toDtype)
	case tensor.Uint8:
		castFromUint8(result, x, toDtype)
	case tensor.Bool:
		castFromBool(result, x, toDtype)
	default:
		panic(fmt.Sprintf("cast: unsupported source dtype %v", fromDtype))
	}
}

// ============================================================================
// Cast from Float32
// ============================================================================

func castFromFloat32(result, x *tensor.RawTensor, toDtype tensor.DataType) {
	src := x.AsFloat32()

	switch toDtype {
	case tensor.Float64:
		dst := result.AsFloat64()
		for i, v := range src {
			dst[i] = float64(v)
		}
	case tensor.Int32:
		dst := result.AsInt32()
		for i, v := range src {
			dst[i] = int32(v)
		}
	case tensor.Int64:
		dst := result.AsInt64()
		for i, v := range src {
			dst[i] = int64(v)
		}
	case tensor.Uint8:
		dst := result.AsUint8()
		for i, v := range src {
			dst[i] = uint8(v)
		}
	case tensor.Bool:
		dst := result.AsBool()
		for i, v := range src {
			dst[i] = v != 0
		}
	default:
		panic(fmt.Sprintf("cast: unsupported target dtype %v from float32", toDtype))
	}
}

// ============================================================================
// Cast from Float64
// ============================================================================

func castFromFloat64(result, x *tensor.RawTensor, toDtype tensor.DataType) {
	src := x.AsFloat64()

	switch toDtype {
	case tensor.Float32:
		dst := result.AsFloat32()
		for i, v := range src {
			dst[i] = float32(v)
		}
	case tensor.Int32:
		dst := result.AsInt32()
		for i, v := range src {
			dst[i] = int32(v)
		}
	case tensor.Int64:
		dst := result.AsInt64()
		for i, v := range src {
			dst[i] = int64(v)
		}
	case tensor.Uint8:
		dst := result.AsUint8()
		for i, v := range src {
			dst[i] = uint8(v)
		}
	case tensor.Bool:
		dst := result.AsBool()
		for i, v := range src {
			dst[i] = v != 0
		}
	default:
		panic(fmt.Sprintf("cast: unsupported target dtype %v from float64", toDtype))
	}
}

// ============================================================================
// Cast from Int32
// ============================================================================

func castFromInt32(result, x *tensor.RawTensor, toDtype tensor.DataType) {
	src := x.AsInt32()

	switch toDtype {
	case tensor.Float32:
		dst := result.AsFloat32()
		for i, v := range src {
			dst[i] = float32(v)
		}
	case tensor.Float64:
		dst := result.AsFloat64()
		for i, v := range src {
			dst[i] = float64(v)
		}
	case tensor.Int64:
		dst := result.AsInt64()
		for i, v := range src {
			dst[i] = int64(v)
		}
	case tensor.Uint8:
		dst := result.AsUint8()
		for i, v := range src {
			dst[i] = uint8(v) //nolint:gosec // G115: intentional dtype cast, caller controls value range
		}
	case tensor.Bool:
		dst := result.AsBool()
		for i, v := range src {
			dst[i] = v != 0
		}
	default:
		panic(fmt.Sprintf("cast: unsupported target dtype %v from int32", toDtype))
	}
}

// ============================================================================
// Cast from Int64
// ============================================================================

func castFromInt64(result, x *tensor.RawTensor, toDtype tensor.DataType) {
	src := x.AsInt64()

	switch toDtype {
	case tensor.Float32:
		dst := result.AsFloat32()
		for i, v := range src {
			dst[i] = float32(v)
		}
	case tensor.Float64:
		dst := result.AsFloat64()
		for i, v := range src {
			dst[i] = float64(v)
		}
	case tensor.Int32:
		dst := result.AsInt32()
		for i, v := range src {
			dst[i] = int32(v) //nolint:gosec // G115: intentional dtype cast, caller controls value range
		}
	case tensor.Uint8:
		dst := result.AsUint8()
		for i, v := range src {
			dst[i] = uint8(v) //nolint:gosec // G115: intentional dtype cast, caller controls value range
		}
	case tensor.Bool:
		dst := result.AsBool()
		for i, v := range src {
			dst[i] = v != 0
		}
	default:
		panic(fmt.Sprintf("cast: unsupported target dtype %v from int64", toDtype))
	}
}

// ============================================================================
// Cast from Uint8
// ============================================================================

func castFromUint8(result, x *tensor.RawTensor, toDtype tensor.DataType) {
	src := x.AsUint8()

	switch toDtype {
	case tensor.Float32:
		dst := result.AsFloat32()
		for i, v := range src {
			dst[i] = float32(v)
		}
	case tensor.Float64:
		dst := result.AsFloat64()
		for i, v := range src {
			dst[i] = float64(v)
		}
	case tensor.Int32:
		dst := result.AsInt32()
		for i, v := range src {
			dst[i] = int32(v)
		}
	case tensor.Int64:
		dst := result.AsInt64()
		for i, v := range src {
			dst[i] = int64(v)
		}
	case tensor.Bool:
		dst := result.AsBool()
		for i, v := range src {
			dst[i] = v != 0
		}
	default:
		panic(fmt.Sprintf("cast: unsupported target dtype %v from uint8", toDtype))
	}
}

// ============================================================================
// Cast from Bool
// ============================================================================

//nolint:gocognit,gocyclo,cyclop // Type-specific casting logic for 5 target types (float32, float64, int32, int64, uint8)
func castFromBool(result, x *tensor.RawTensor, toDtype tensor.DataType) {
	src := x.AsBool()

	switch toDtype {
	case tensor.Float32:
		dst := result.AsFloat32()
		for i, v := range src {
			if v {
				dst[i] = 1.0
			} else {
				dst[i] = 0.0
			}
		}
	case tensor.Float64:
		dst := result.AsFloat64()
		for i, v := range src {
			if v {
				dst[i] = 1.0
			} else {
				dst[i] = 0.0
			}
		}
	case tensor.Int32:
		dst := result.AsInt32()
		for i, v := range src {
			if v {
				dst[i] = 1
			} else {
				dst[i] = 0
			}
		}
	case tensor.Int64:
		dst := result.AsInt64()
		for i, v := range src {
			if v {
				dst[i] = 1
			} else {
				dst[i] = 0
			}
		}
	case tensor.Uint8:
		dst := result.AsUint8()
		for i, v := range src {
			if v {
				dst[i] = 1
			} else {
				dst[i] = 0
			}
		}
	default:
		panic(fmt.Sprintf("cast: unsupported target dtype %v from bool", toDtype))
	}
}
