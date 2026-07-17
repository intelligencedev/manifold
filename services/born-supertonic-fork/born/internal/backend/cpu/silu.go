package cpu

import (
	"fmt"
	"math"

	"github.com/born-ml/born/internal/tensor"
)

// SiLU applies SiLU (Swish) activation: x * sigmoid(x).
func (cpu *CPUBackend) SiLU(x *tensor.RawTensor) *tensor.RawTensor {
	result, err := tensor.NewRaw(x.Shape(), x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("silu: %v", err))
	}

	switch x.DType() {
	case tensor.Float32:
		src := x.AsFloat32()
		dst := result.AsFloat32()
		if simdSiluFloat32 != nil {
			simdSiluFloat32(dst, src)
		} else {
			siluScalar(dst, src)
		}
	case tensor.Float64:
		src := x.AsFloat64()
		dst := result.AsFloat64()
		if simdSiluFloat64 != nil {
			simdSiluFloat64(dst, src)
		} else {
			siluScalar(dst, src)
		}
	default:
		panic(fmt.Sprintf("silu: unsupported dtype %s", x.DType()))
	}

	return result
}

// siluScalar computes dst[i] = src[i] * sigmoid(src[i]) in a scalar loop.
func siluScalar[T float32 | float64](dst, src []T) {
	for i, v := range src {
		sig := T(1.0 / (1.0 + math.Exp(float64(-v))))
		dst[i] = v * sig
	}
}
