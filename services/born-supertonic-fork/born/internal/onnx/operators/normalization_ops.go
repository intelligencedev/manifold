//go:build !wasm

package operators

import (
	"fmt"

	"github.com/born-ml/born/internal/tensor"
)

func (r *Registry) registerNormalizationOps() {
	r.Register("LayerNormalization", handleLayerNormalization)
}

func handleLayerNormalization(ctx *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) < 2 {
		return nil, fmt.Errorf("layerNormalization requires at least 2 inputs (X, Scale), got %d", len(inputs))
	}

	X := inputs[0]
	Scale := inputs[1]

	epsilon := float32(GetAttrFloat(node, "epsilon", 1e-5))
	rank := len(X.Shape())
	axis := int(GetAttrInt(node, "axis", -1))
	if axis < 0 {
		axis = rank + axis
	}
	if axis < 0 || axis >= rank {
		return nil, fmt.Errorf("layerNormalization: axis %d out of range for tensor of rank %d", axis, rank)
	}

	mean := X.Clone()
	for i := axis; i < rank; i++ {
		mean = ctx.Backend.MeanDim(mean, i, true)
	}

	xCentered := ctx.Backend.Sub(X, mean)

	variance := ctx.Backend.Mul(xCentered, xCentered)
	for i := axis; i < rank; i++ {
		variance = ctx.Backend.MeanDim(variance, i, true)
	}

	variancePlusEps := ctx.Backend.AddScalar(variance, epsilon)
	invStdDev := ctx.Backend.Rsqrt(variancePlusEps)
	normalized := ctx.Backend.Mul(xCentered, invStdDev)

	output := ctx.Backend.Mul(normalized, Scale)
	if len(inputs) == 3 {
		B := inputs[2]
		output = ctx.Backend.Add(output, B)
	}
	return []*tensor.RawTensor{output}, nil
}
