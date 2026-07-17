package cpu

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

// ReLU applies ReLU activation: max(0, x).
func (cpu *CPUBackend) ReLU(x *tensor.RawTensor) *tensor.RawTensor {
	result, err := tensor.NewRaw(x.Shape(), x.DType(), cpu.device)
	if err != nil {
		panic(fmt.Sprintf("relu: %v", err))
	}

	switch x.DType() {
	case tensor.Float32:
		src := x.AsFloat32()
		dst := result.AsFloat32()
		if simdReluFloat32 != nil {
			simdReluFloat32(dst, src)
		} else {
			reluScalar(dst, src)
		}
	case tensor.Float64:
		src := x.AsFloat64()
		dst := result.AsFloat64()
		if simdReluFloat64 != nil {
			simdReluFloat64(dst, src)
		} else {
			reluScalar(dst, src)
		}
	default:
		panic(fmt.Sprintf("relu: unsupported dtype %s", x.DType()))
	}

	return result
}

// reluScalar computes dst[i] = max(src[i], 0) in a scalar loop.
func reluScalar[T float32 | float64](dst, src []T) {
	for i, v := range src {
		if v > 0 {
			dst[i] = v
		} else {
			dst[i] = 0
		}
	}
}
