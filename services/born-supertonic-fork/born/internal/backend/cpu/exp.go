package cpu

import (
	"fmt"
	"math"

	"github.com/born-ml/born/internal/tensor"
)

// Exp computes element-wise exponential: exp(x).
func (cpu *CPUBackend) Exp(x *tensor.RawTensor) *tensor.RawTensor {
	result, err := tensor.NewRaw(x.Shape(), x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("exp: %v", err))
	}

	switch x.DType() {
	case tensor.Float32:
		src := x.AsFloat32()
		dst := result.AsFloat32()
		if simdExpFloat32 != nil {
			simdExpFloat32(dst, src)
		} else {
			expScalar(dst, src)
		}
	case tensor.Float64:
		src := x.AsFloat64()
		dst := result.AsFloat64()
		if simdExpFloat64 != nil {
			simdExpFloat64(dst, src)
		} else {
			expScalar(dst, src)
		}
	default:
		panic(fmt.Sprintf("exp: unsupported dtype %s (only float32/float64 supported)", x.DType()))
	}

	return result
}

// expScalar computes dst[i] = exp(src[i]) using a scalar loop.
func expScalar[T float32 | float64](dst, src []T) {
	for i, v := range src {
		dst[i] = T(math.Exp(float64(v)))
	}
}
