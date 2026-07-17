package cpu

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// Scalar operations - element-wise operations with a scalar value.

// MulScalar multiplies each element of the tensor by a scalar value.
func (cpu *CPUBackend) MulScalar(x *tensor.RawTensor, scalar any) *tensor.RawTensor {
	result, err := tensor.NewRaw(x.Shape(), x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("mulScalar: failed to create result tensor: %v", err))
	}

	switch x.DType() {
	case tensor.Float32:
		mulScalarFloat32(result, x, scalar.(float32))
	case tensor.Float64:
		mulScalarFloat64(result, x, scalar.(float64))
	case tensor.Int32:
		mulScalarInt32(result, x, scalar.(int32))
	case tensor.Int64:
		mulScalarInt64(result, x, scalar.(int64))
	default:
		panic(fmt.Sprintf("mulScalar: unsupported dtype %v", x.DType()))
	}

	return result
}

// AddScalar adds a scalar value to each element of the tensor.
func (cpu *CPUBackend) AddScalar(x *tensor.RawTensor, scalar any) *tensor.RawTensor {
	result, err := tensor.NewRaw(x.Shape(), x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("addScalar: failed to create result tensor: %v", err))
	}

	switch x.DType() {
	case tensor.Float32:
		addScalarFloat32(result, x, scalar.(float32))
	case tensor.Float64:
		addScalarFloat64(result, x, scalar.(float64))
	case tensor.Int32:
		addScalarInt32(result, x, scalar.(int32))
	case tensor.Int64:
		addScalarInt64(result, x, scalar.(int64))
	default:
		panic(fmt.Sprintf("addScalar: unsupported dtype %v", x.DType()))
	}

	return result
}

// SubScalar subtracts a scalar value from each element of the tensor.
func (cpu *CPUBackend) SubScalar(x *tensor.RawTensor, scalar any) *tensor.RawTensor {
	result, err := tensor.NewRaw(x.Shape(), x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("subScalar: failed to create result tensor: %v", err))
	}

	switch x.DType() {
	case tensor.Float32:
		subScalarFloat32(result, x, scalar.(float32))
	case tensor.Float64:
		subScalarFloat64(result, x, scalar.(float64))
	case tensor.Int32:
		subScalarInt32(result, x, scalar.(int32))
	case tensor.Int64:
		subScalarInt64(result, x, scalar.(int64))
	default:
		panic(fmt.Sprintf("subScalar: unsupported dtype %v", x.DType()))
	}

	return result
}

// DivScalar divides each element of the tensor by a scalar value.
func (cpu *CPUBackend) DivScalar(x *tensor.RawTensor, scalar any) *tensor.RawTensor {
	result, err := tensor.NewRaw(x.Shape(), x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("divScalar: failed to create result tensor: %v", err))
	}

	switch x.DType() {
	case tensor.Float32:
		divScalarFloat32(result, x, scalar.(float32))
	case tensor.Float64:
		divScalarFloat64(result, x, scalar.(float64))
	case tensor.Int32:
		divScalarInt32(result, x, scalar.(int32))
	case tensor.Int64:
		divScalarInt64(result, x, scalar.(int64))
	default:
		panic(fmt.Sprintf("divScalar: unsupported dtype %v", x.DType()))
	}

	return result
}

// ============================================================================
// Float32 implementations
// ============================================================================

func mulScalarFloat32(result, x *tensor.RawTensor, scalar float32) {
	xData := x.AsFloat32()
	resultData := result.AsFloat32()

	for i := range resultData {
		resultData[i] = xData[i] * scalar
	}
}

func addScalarFloat32(result, x *tensor.RawTensor, scalar float32) {
	xData := x.AsFloat32()
	resultData := result.AsFloat32()

	for i := range resultData {
		resultData[i] = xData[i] + scalar
	}
}

func subScalarFloat32(result, x *tensor.RawTensor, scalar float32) {
	xData := x.AsFloat32()
	resultData := result.AsFloat32()

	for i := range resultData {
		resultData[i] = xData[i] - scalar
	}
}

func divScalarFloat32(result, x *tensor.RawTensor, scalar float32) {
	xData := x.AsFloat32()
	resultData := result.AsFloat32()

	for i := range resultData {
		resultData[i] = xData[i] / scalar
	}
}

// ============================================================================
// Float64 implementations
// ============================================================================

func mulScalarFloat64(result, x *tensor.RawTensor, scalar float64) {
	xData := x.AsFloat64()
	resultData := result.AsFloat64()

	for i := range resultData {
		resultData[i] = xData[i] * scalar
	}
}

func addScalarFloat64(result, x *tensor.RawTensor, scalar float64) {
	xData := x.AsFloat64()
	resultData := result.AsFloat64()

	for i := range resultData {
		resultData[i] = xData[i] + scalar
	}
}

func subScalarFloat64(result, x *tensor.RawTensor, scalar float64) {
	xData := x.AsFloat64()
	resultData := result.AsFloat64()

	for i := range resultData {
		resultData[i] = xData[i] - scalar
	}
}

func divScalarFloat64(result, x *tensor.RawTensor, scalar float64) {
	xData := x.AsFloat64()
	resultData := result.AsFloat64()

	for i := range resultData {
		resultData[i] = xData[i] / scalar
	}
}

// ============================================================================
// Int32 implementations
// ============================================================================

func mulScalarInt32(result, x *tensor.RawTensor, scalar int32) {
	xData := x.AsInt32()
	resultData := result.AsInt32()

	for i := range resultData {
		resultData[i] = xData[i] * scalar
	}
}

func addScalarInt32(result, x *tensor.RawTensor, scalar int32) {
	xData := x.AsInt32()
	resultData := result.AsInt32()

	for i := range resultData {
		resultData[i] = xData[i] + scalar
	}
}

func subScalarInt32(result, x *tensor.RawTensor, scalar int32) {
	xData := x.AsInt32()
	resultData := result.AsInt32()

	for i := range resultData {
		resultData[i] = xData[i] - scalar
	}
}

func divScalarInt32(result, x *tensor.RawTensor, scalar int32) {
	xData := x.AsInt32()
	resultData := result.AsInt32()

	for i := range resultData {
		resultData[i] = xData[i] / scalar
	}
}

// ============================================================================
// Int64 implementations
// ============================================================================

func mulScalarInt64(result, x *tensor.RawTensor, scalar int64) {
	xData := x.AsInt64()
	resultData := result.AsInt64()

	for i := range resultData {
		resultData[i] = xData[i] * scalar
	}
}

func addScalarInt64(result, x *tensor.RawTensor, scalar int64) {
	xData := x.AsInt64()
	resultData := result.AsInt64()

	for i := range resultData {
		resultData[i] = xData[i] + scalar
	}
}

func subScalarInt64(result, x *tensor.RawTensor, scalar int64) {
	xData := x.AsInt64()
	resultData := result.AsInt64()

	for i := range resultData {
		resultData[i] = xData[i] - scalar
	}
}

func divScalarInt64(result, x *tensor.RawTensor, scalar int64) {
	xData := x.AsInt64()
	resultData := result.AsInt64()

	for i := range resultData {
		resultData[i] = xData[i] / scalar
	}
}
