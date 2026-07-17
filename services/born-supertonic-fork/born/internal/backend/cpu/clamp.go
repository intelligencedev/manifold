package cpu

import (
	"math"

	"github.com/born-ml/born/internal/tensor"
)

// Clamp restricts tensor values element-wise to [minBound, maxBound].
// If minBound > maxBound, all values are set to maxBound.
// Supported dtypes: int32, int64, float32, float64.
// Panics if bounds are nil, if bounds and tensor dtype do not match, or if bounds are NaN (for floats).
func (cpu *CPUBackend) Clamp(input *tensor.RawTensor, minBound, maxBound any) *tensor.RawTensor {
	if minBound == nil || maxBound == nil {
		panic("clamp: min and max bounds cannot be nil")
	}

	var result *tensor.RawTensor
	// when minBound > maxBound set all values to maxBound
	// otherwise, perform normal clamp operation
	switch input.DType() {
	case tensor.Int32:
		castedMin := tensor.CheckScalarDType[int32](minBound)
		castedMax := tensor.CheckScalarDType[int32](maxBound)
		if castedMin > castedMax {
			return tensor.Full(input.Shape(), castedMax, cpu).Raw()
		}
		result = clampGeneric(input, castedMin, castedMax, cpu)
	case tensor.Int64:
		castedMin := tensor.CheckScalarDType[int64](minBound)
		castedMax := tensor.CheckScalarDType[int64](maxBound)
		if castedMin > castedMax {
			return tensor.Full(input.Shape(), castedMax, cpu).Raw()
		}
		result = clampGeneric(input, castedMin, castedMax, cpu)
	case tensor.Float32:
		castedMin := tensor.CheckScalarDType[float32](minBound)
		castedMax := tensor.CheckScalarDType[float32](maxBound)
		checkNaN(castedMin)
		checkNaN(castedMax)
		if castedMin > castedMax {
			return tensor.Full(input.Shape(), castedMax, cpu).Raw()
		}
		result = clampGeneric(input, castedMin, castedMax, cpu)
	case tensor.Float64:
		castedMin := tensor.CheckScalarDType[float64](minBound)
		castedMax := tensor.CheckScalarDType[float64](maxBound)
		checkNaN(castedMin)
		checkNaN(castedMax)
		if castedMin > castedMax {
			return tensor.Full(input.Shape(), castedMax, cpu).Raw()
		}
		result = clampGeneric(input, castedMin, castedMax, cpu)
	default:
		panic("clamp: unsupported dtype (only int32/int64/float32/float64 supported)")
	}

	return result
}

// checkNaN checks if the value is NaN and panics if it is.
func checkNaN[T float32 | float64](v T) {
	if math.IsNaN(float64(v)) {
		panic("clamp: bounds cannot be NaN")
	}
}

// clampGeneric performs the clamp operation for a specific data type T.
func clampGeneric[T int32 | int64 | float32 | float64](input *tensor.RawTensor, minBound, maxBound T, cpu *CPUBackend) *tensor.RawTensor {
	minValues := tensor.Full(input.Shape(), minBound, cpu).Raw()
	maxValues := tensor.Full(input.Shape(), maxBound, cpu).Raw()

	// Clamp to min: select max(input, minValues)
	minMask := cpu.LowerEqual(input, minValues) // 1 where input <= min, else 0
	clampedMin := cpu.Where(minMask, minValues, input)

	// Clamp to max: select min(clampedMin, maxValues)
	maxMask := cpu.GreaterEqual(clampedMin, maxValues) // 1 where clampedMin >= max, else 0
	clamped := cpu.Where(maxMask, maxValues, clampedMin)

	return clamped
}
