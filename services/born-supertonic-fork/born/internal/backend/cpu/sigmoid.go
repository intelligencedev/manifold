package cpu

import (
	"fmt"
	"math"

	"github.com/born-ml/born/internal/tensor"
)

// Sigmoid applies sigmoid activation: 1 / (1 + exp(-x)).
func (cpu *CPUBackend) Sigmoid(x *tensor.RawTensor) *tensor.RawTensor {
	result, err := tensor.NewRaw(x.Shape(), x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("sigmoid: %v", err))
	}

	switch x.DType() {
	case tensor.Float32:
		src := x.AsFloat32()
		dst := result.AsFloat32()
		if simdSigmoidFloat32 != nil {
			simdSigmoidFloat32(dst, src)
		} else {
			sigmoidScalar(dst, src)
		}
	case tensor.Float64:
		src := x.AsFloat64()
		dst := result.AsFloat64()
		if simdSigmoidFloat64 != nil {
			simdSigmoidFloat64(dst, src)
		} else {
			sigmoidScalar(dst, src)
		}
	default:
		panic(fmt.Sprintf("sigmoid: unsupported dtype %s", x.DType()))
	}

	return result
}

// sigmoidScalar computes dst[i] = 1 / (1 + exp(-src[i])) in a scalar loop.
func sigmoidScalar[T float32 | float64](dst, src []T) {
	for i, v := range src {
		dst[i] = T(1.0 / (1.0 + math.Exp(float64(-v))))
	}
}
