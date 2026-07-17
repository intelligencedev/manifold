//go:build !wasm

package operators

import (
	"fmt"
	"math"

	"github.com/born-ml/born/internal/tensor"
)

// registerReduceOps registers ONNX reduction operators.
func (r *Registry) registerReduceOps() {
	r.Register("ReduceMean", handleReduceMean)
	r.Register("ReduceMax", handleReduceMax)
	r.Register("ReduceMin", handleReduceMin)
}

type reduceKind int

const (
	reduceMean reduceKind = iota
	reduceMax
	reduceMin
	reduceSum
)

func handleReduceMean(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	return handleReduce(node, inputs, reduceMean)
}

func handleReduceMax(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	return handleReduce(node, inputs, reduceMax)
}

func handleReduceMin(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	return handleReduce(node, inputs, reduceMin)
}

// handleReduce implements ONNX Reduce* (opset 13/18 semantics).
//
// axes may be supplied as a second int64 tensor input (opset 18) or as an
// "axes" attribute (older opset). keepdims and noop_with_empty_axes are
// attributes. With empty axes: noop_with_empty_axes=1 returns the input
// unchanged, otherwise all axes are reduced.
//
// TODO: extend beyond float32 (float64, int32, int64) when callers need it.
func handleReduce(node *Node, inputs []*tensor.RawTensor, kind reduceKind) ([]*tensor.RawTensor, error) {
	if len(inputs) < 1 || inputs[0] == nil {
		return nil, fmt.Errorf("reduce: missing data input")
	}
	data := inputs[0]
	if data.DType() != tensor.Float32 {
		return nil, fmt.Errorf("reduce: only float32 supported, got %s", data.DType())
	}
	if data.NumElements() == 0 {
		return nil, fmt.Errorf("reduce: empty input tensor")
	}
	if len(inputs) >= 2 && inputs[1] != nil && inputs[1].DType() != tensor.Int64 {
		return nil, fmt.Errorf("reduce: axes must be int64, got %s", inputs[1].DType())
	}
	keepdims := GetAttrInt(node, "keepdims", 1) != 0
	noop := GetAttrInt(node, "noop_with_empty_axes", 0) != 0

	axes := reduceAxes(node, inputs)
	if len(axes) == 0 {
		if noop {
			return []*tensor.RawTensor{data.Clone()}, nil
		}
		for i := range data.Shape() {
			axes = append(axes, i)
		}
	}

	out, err := reduceFloat32(data, axes, keepdims, kind)
	if err != nil {
		return nil, err
	}
	return []*tensor.RawTensor{out}, nil
}

// reduceAxes reads the axes either from the second input (opset 18) or the
// "axes" attribute (older opset). The caller guarantees a present second
// input is int64.
func reduceAxes(node *Node, inputs []*tensor.RawTensor) []int {
	var axes []int
	if len(inputs) >= 2 && inputs[1] != nil {
		for _, a := range inputs[1].AsInt64() {
			axes = append(axes, int(a))
		}
		return axes
	}
	for _, a := range GetAttrInts(node, "axes") {
		axes = append(axes, int(a))
	}
	return axes
}

// reduceFloat32 reduces a float32 tensor over the given axes (supports
// negative axes and multiple axes) in a single pass.
//
// The flat input is walked in row-major order with an odometer coordinate
// counter rather than per-element index division, and the result is
// accumulated directly into the output buffer (no separate accumulator +
// copy). Squeezing reduced dims only removes size-1 axes, so the squeezed
// output shares the accumulator's element order.
func reduceFloat32(in *tensor.RawTensor, axes []int, keepdims bool, kind reduceKind) (*tensor.RawTensor, error) {
	inShape := in.Shape()
	rank := len(inShape)
	reduced, err := markReducedAxes(axes, rank)
	if err != nil {
		return nil, err
	}

	// outStrides is computed from the ones-replaced shape (outFull), but the
	// same strides correctly index the squeezed (no-keepdims) buffer too.
	// Invariant: a reduced axis is size 1 in outFull, so it contributes a
	// factor of 1 to every stride. For each non-reduced axis the resulting
	// stride therefore equals the product of the non-reduced trailing dims,
	// which is exactly that axis's stride in the squeezed shape. Removing the
	// size-1 axes only drops the always-zero coordinate terms, so oi lands on
	// the same element either way. Do not "simplify" by squeezing outFull
	// before ComputeStrides; the two layouts must share these strides.
	outFull := reducedToOne(inShape, reduced)
	outStrides := tensor.Shape(outFull).ComputeStrides()

	finalShape := tensor.Shape(outFull)
	if !keepdims {
		finalShape = squeezeReduced(inShape, reduced)
	}
	out, err := tensor.NewRaw(finalShape, tensor.Float32, tensor.CPU)
	if err != nil {
		return nil, fmt.Errorf("reduce: %w", err)
	}
	acc := out.AsFloat32()
	initReduceAcc(acc, kind)

	inData := in.AsFloat32()
	coords := make([]int, rank)
	for _, v := range inData {
		oi := 0
		for d := range rank {
			if !reduced[d] {
				oi += coords[d] * outStrides[d]
			}
		}
		reduceStep(acc, oi, v, kind)
		for d := rank - 1; d >= 0; d-- {
			coords[d]++
			if coords[d] < inShape[d] {
				break
			}
			coords[d] = 0
		}
	}

	if kind == reduceMean {
		if c := productReduced(inShape, reduced); c > 0 {
			inv := float32(1) / float32(c)
			for i := range acc {
				acc[i] *= inv
			}
		}
	}
	return out, nil
}

func markReducedAxes(axes []int, rank int) ([]bool, error) {
	reduced := make([]bool, rank)
	for _, a := range axes {
		if a < 0 {
			a += rank
		}
		if a < 0 || a >= rank {
			return nil, fmt.Errorf("reduce: axis out of range for rank %d", rank)
		}
		reduced[a] = true
	}
	return reduced, nil
}

// reducedToOne returns inShape with reduced dimensions set to size 1.
func reducedToOne(inShape []int, reduced []bool) []int {
	out := make([]int, len(inShape))
	for i, d := range inShape {
		if reduced[i] {
			out[i] = 1
		} else {
			out[i] = d
		}
	}
	return out
}

// squeezeReduced returns inShape with reduced dimensions removed.
func squeezeReduced(inShape []int, reduced []bool) []int {
	out := make([]int, 0, len(inShape))
	for i, d := range inShape {
		if !reduced[i] {
			out = append(out, d)
		}
	}
	return out
}

func initReduceAcc(acc []float32, kind reduceKind) {
	switch kind {
	case reduceMax:
		for i := range acc {
			acc[i] = float32(math.Inf(-1))
		}
	case reduceMin:
		for i := range acc {
			acc[i] = float32(math.Inf(1))
		}
	case reduceMean:
		// zero value is the correct sum identity
	}
}

func reduceStep(acc []float32, oi int, v float32, kind reduceKind) {
	switch kind {
	case reduceMean, reduceSum:
		acc[oi] += v
	case reduceMax:
		if v > acc[oi] {
			acc[oi] = v
		}
	case reduceMin:
		if v < acc[oi] {
			acc[oi] = v
		}
	}
}

func productReduced(inShape []int, reduced []bool) int {
	p := 1
	for i, d := range inShape {
		if reduced[i] {
			p *= d
		}
	}
	return p
}
