package nn

import (
	"math"

	"github.com/born-ml/born/internal/tensor"
)

// Xavier (Glorot) initialization for weights.
//
// Initializes weights with values drawn from a uniform distribution:
// U(-sqrt(6/(fan_in + fan_out)), sqrt(6/(fan_in + fan_out)))
//
// This initialization helps maintain variance of activations across layers.
//
// Parameters:
//   - fanIn: Number of input units
//   - fanOut: Number of output units
//   - shape: Shape of the weight tensor
//   - backend: Backend to use for tensor creation
//
// Returns a tensor initialized with Xavier distribution.
func Xavier[B tensor.Backend](fanIn, fanOut int, shape tensor.Shape, backend B) *tensor.Tensor[float32, B] {
	// Xavier/Glorot bound: sqrt(6 / (fan_in + fan_out))
	bound := math.Sqrt(6.0 / float64(fanIn+fanOut))

	// Create tensor with random values in [-bound, bound]
	t, err := tensor.NewRaw(shape, tensor.Float32, backend.Device())
	if err != nil {
		panic(err)
	}

	data := t.AsFloat32()
	for i := range data {
		data[i] = float32((randFloat64()*2.0 - 1.0) * bound)
	}

	return tensor.New[float32, B](t, backend)
}

// Zeros creates a tensor filled with zeros.
//
// This is commonly used for bias initialization.
//
// Parameters:
//   - shape: Shape of the tensor
//   - backend: Backend to use for tensor creation
//
// Returns a zero-filled tensor.
func Zeros[B tensor.Backend](shape tensor.Shape, backend B) *tensor.Tensor[float32, B] {
	return tensor.Zeros[float32](shape, backend)
}

// Ones creates a tensor filled with ones.
//
// Parameters:
//   - shape: Shape of the tensor
//   - backend: Backend to use for tensor creation
//
// Returns a tensor filled with ones.
func Ones[B tensor.Backend](shape tensor.Shape, backend B) *tensor.Tensor[float32, B] {
	return tensor.Ones[float32](shape, backend)
}

// Randn creates a tensor with random values from standard normal distribution.
//
// Values are drawn from N(0, 1).
//
// Parameters:
//   - shape: Shape of the tensor
//   - backend: Backend to use for tensor creation
//
// Returns a tensor with random normal values.
func Randn[B tensor.Backend](shape tensor.Shape, backend B) *tensor.Tensor[float32, B] {
	return tensor.Randn[float32](shape, backend)
}
