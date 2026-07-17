package tensor

import (
	"math"
)

// Zeros creates a tensor filled with zeros.
//
// Example:
//
//	backend := cpu.New()
//	t := tensor.Zeros[float32](Shape{3, 4}, backend)
func Zeros[T DType, B Backend](shape Shape, b B) *Tensor[T, B] {
	var dummy T
	dtype := inferDataType(dummy)

	raw, err := NewRaw(shape, dtype, b.Device())
	if err != nil {
		panic(err) // Shape validation should prevent this
	}

	// Data is already zero-initialized by make()
	return New[T, B](raw, b)
}

// Ones creates a tensor filled with ones.
//
// Example:
//
//	t := tensor.Ones[float64](Shape{2, 3}, backend)
func Ones[T DType, B Backend](shape Shape, b B) *Tensor[T, B] {
	t := Zeros[T, B](shape, b)
	data := t.Data()

	// Type-specific one value
	var dummy T
	var one any
	switch any(dummy).(type) {
	case float32:
		one = float32(1)
	case float64:
		one = float64(1)
	case int32:
		one = int32(1)
	case int64:
		one = int64(1)
	case uint8:
		one = uint8(1)
	case bool:
		one = true
	}

	for i := range data {
		data[i] = one.(T)
	}
	return t
}

// Full creates a tensor filled with a specific value.
//
// Example:
//
//	t := tensor.Full[float32](Shape{3, 3}, 3.14, backend)
func Full[T DType, B Backend](shape Shape, value T, b B) *Tensor[T, B] {
	t := Zeros[T, B](shape, b)
	data := t.Data()
	for i := range data {
		data[i] = value
	}
	return t
}

// Randn creates a tensor with random values from a normal distribution (mean=0, std=1).
// Uses Box-Muller transform for generating normal distribution.
// Only works with float types.
// Note: Uses math/rand (not crypto/rand) - appropriate for ML/statistical purposes.
//
// Example:
//
//	t := tensor.Randn[float32](Shape{100, 100}, backend)
func Randn[T DType, B Backend](shape Shape, b B) *Tensor[T, B] {
	t := Zeros[T, B](shape, b)
	data := t.Data()

	// Box-Muller transform for normal distribution (only for float types)
	var dummy T
	switch any(dummy).(type) {
	case float32:
		dataF32 := any(data).([]float32)
		for i := 0; i < len(dataF32); i += 2 {
			u1 := RandFloat64()
			u2 := RandFloat64()
			z0 := math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)
			z1 := math.Sqrt(-2.0*math.Log(u1)) * math.Sin(2.0*math.Pi*u2)
			dataF32[i] = float32(z0)
			if i+1 < len(dataF32) {
				dataF32[i+1] = float32(z1)
			}
		}
	case float64:
		dataF64 := any(data).([]float64)
		for i := 0; i < len(dataF64); i += 2 {
			u1 := RandFloat64()
			u2 := RandFloat64()
			z0 := math.Sqrt(-2.0*math.Log(u1)) * math.Cos(2.0*math.Pi*u2)
			z1 := math.Sqrt(-2.0*math.Log(u1)) * math.Sin(2.0*math.Pi*u2)
			dataF64[i] = z0
			if i+1 < len(dataF64) {
				dataF64[i+1] = z1
			}
		}
	default:
		panic("Randn only supports float32 and float64 types")
	}
	return t
}

// Rand creates a tensor with random values uniformly distributed in [0, 1).
// Only works with float types.
//
// Example:
//
//	t := tensor.Rand[float32](Shape{10, 10}, backend)
func Rand[T DType, B Backend](shape Shape, b B) *Tensor[T, B] {
	t := Zeros[T, B](shape, b)
	data := t.Data()

	var dummy T
	switch any(dummy).(type) {
	case float32:
		dataF32 := any(data).([]float32)
		for i := range dataF32 {
			dataF32[i] = float32(RandFloat64())
		}
	case float64:
		dataF64 := any(data).([]float64)
		for i := range dataF64 {
			dataF64[i] = RandFloat64()
		}
	default:
		panic("Rand only supports float32 and float64 types")
	}
	return t
}

// Arange creates a 1D tensor with values from start to end (exclusive).
// Only works with numeric types (not bool).
//
// Example:
//
//	t := tensor.Arange[int32](0, 10, backend) // [0, 1, 2, ..., 9]
//
//nolint:gocyclo,cyclop // Type-specific logic for each supported numeric type
func Arange[T DType, B Backend](start, end T, b B) *Tensor[T, B] {
	// Calculate number of elements based on type
	var numElements int
	switch any(start).(type) {
	case float32:
		numElements = int(any(end).(float32) - any(start).(float32))
	case float64:
		numElements = int(any(end).(float64) - any(start).(float64))
	case int32:
		numElements = int(any(end).(int32) - any(start).(int32))
	case int64:
		numElements = int(any(end).(int64) - any(start).(int64))
	case uint8:
		numElements = int(any(end).(uint8) - any(start).(uint8))
	default:
		panic("Arange not supported for this type")
	}

	if numElements <= 0 {
		panic("end must be greater than start")
	}

	t := Zeros[T, B](Shape{numElements}, b)
	data := t.Data()

	// Type-specific increment
	switch any(start).(type) {
	case float32:
		dataF32 := any(data).([]float32)
		startF32 := any(start).(float32)
		for i := range dataF32 {
			dataF32[i] = startF32 + float32(i)
		}
	case float64:
		dataF64 := any(data).([]float64)
		startF64 := any(start).(float64)
		for i := range dataF64 {
			dataF64[i] = startF64 + float64(i)
		}
	case int32:
		dataI32 := any(data).([]int32)
		startI32 := any(start).(int32)
		for i := range dataI32 {
			dataI32[i] = startI32 + int32(i)
		}
	case int64:
		dataI64 := any(data).([]int64)
		startI64 := any(start).(int64)
		for i := range dataI64 {
			dataI64[i] = startI64 + int64(i)
		}
	case uint8:
		dataU8 := any(data).([]uint8)
		startU8 := any(start).(uint8)
		for i := range dataU8 {
			dataU8[i] = startU8 + uint8(i)
		}
	}
	return t
}

// Eye creates a 2D identity matrix.
//
// Example:
//
//	t := tensor.Eye[float32](3, backend) // 3x3 identity matrix
func Eye[T DType, B Backend](n int, b B) *Tensor[T, B] {
	t := Zeros[T, B](Shape{n, n}, b)

	// Type-specific one value
	var dummy T
	var one any
	switch any(dummy).(type) {
	case float32:
		one = float32(1)
	case float64:
		one = float64(1)
	case int32:
		one = int32(1)
	case int64:
		one = int64(1)
	case uint8:
		one = uint8(1)
	case bool:
		one = true
	}

	for i := 0; i < n; i++ {
		t.Set(one.(T), i, i)
	}
	return t
}
