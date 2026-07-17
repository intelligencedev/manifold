//go:build !wasm

package operators

import (
	"fmt"
	"math"

	"github.com/born-ml/born/internal/tensor"
)

// registerPoolOps registers ONNX 2D pooling operators.
func (r *Registry) registerPoolOps() {
	r.Register("MaxPool", handleMaxPool)
	r.Register("AveragePool", handleAveragePool)
}

type poolKind int

const (
	poolMax poolKind = iota
	poolAvg
)

// poolParams holds the resolved attributes for a 2D pool.
type poolParams struct {
	kh, kw, sh, sw         int
	padT, padL, padB, padR int
	countIncludePad        bool
}

func handleMaxPool(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	// ONNX MaxPool has an optional second output (Indices, consumed by
	// MaxUnpool). This implementation produces only the pooled values, so a
	// model that wires up the Indices output must fail loudly here rather than
	// silently losing it and breaking the downstream MaxUnpool.
	if len(node.Outputs) > 1 && node.Outputs[1] != "" {
		return nil, fmt.Errorf("pool: MaxPool Indices output not supported")
	}
	return handlePool(node, inputs, poolMax)
}

func handleAveragePool(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	return handlePool(node, inputs, poolAvg)
}

// handlePool implements ONNX MaxPool / AveragePool for 4D NCHW float32 input
// with non-square kernels, strides, and explicit pads. AveragePool honors
// count_include_pad; the defaults (floor output, count_include_pad=0) match
// onnxruntime.
//
// Unsupported attributes are rejected with an error rather than silently
// mispooled: ceil_mode=1, auto_pad=SAME_UPPER/SAME_LOWER, and dilations>1.
//
// This is a CPU reference implementation and intentionally does not delegate
// to ctx.Backend.MaxPool2D: the backend pooling kernels assume a square
// window and equal strides, whereas ONNX permits non-square kernels and
// asymmetric strides/pads. The handler therefore takes no Context. Switch to
// the backend path only once it supports the general (non-square) case.
func handlePool(node *Node, inputs []*tensor.RawTensor, kind poolKind) ([]*tensor.RawTensor, error) {
	if len(inputs) < 1 || inputs[0] == nil {
		return nil, fmt.Errorf("pool: missing input")
	}
	x := inputs[0]
	if x.DType() != tensor.Float32 {
		return nil, fmt.Errorf("pool: only float32 supported, got %s", x.DType())
	}
	shape := x.Shape()
	if len(shape) != 4 {
		return nil, fmt.Errorf("pool: expected 4D NCHW input, got %dD", len(shape))
	}
	p, err := parsePoolParams(node, kind)
	if err != nil {
		return nil, err
	}

	n, c, h, w := shape[0], shape[1], shape[2], shape[3]
	outH := poolOutDim(h, p.padT, p.padB, p.kh, p.sh)
	outW := poolOutDim(w, p.padL, p.padR, p.kw, p.sw)
	if outH <= 0 || outW <= 0 {
		return nil, fmt.Errorf("pool: invalid output dims %dx%d (kernel/stride/pad)", outH, outW)
	}

	out, err := tensor.NewRaw(tensor.Shape{n, c, outH, outW}, tensor.Float32, tensor.CPU)
	if err != nil {
		return nil, fmt.Errorf("pool: %w", err)
	}
	poolForward(x.AsFloat32(), out.AsFloat32(), n, c, h, w, outH, outW, p, kind)
	return []*tensor.RawTensor{out}, nil
}

func parsePoolParams(node *Node, kind poolKind) (poolParams, error) {
	var p poolParams
	if err := checkUnsupportedPoolAttrs(node); err != nil {
		return p, err
	}

	kernel := GetAttrInts(node, "kernel_shape")
	if len(kernel) != 2 {
		return p, fmt.Errorf("pool: only 2D pooling supported, kernel_shape=%v", kernel)
	}
	p.kh, p.kw = int(kernel[0]), int(kernel[1])
	if p.kh <= 0 || p.kw <= 0 {
		return p, fmt.Errorf("pool: invalid kernel_shape %v", kernel)
	}

	p.sh, p.sw = 1, 1
	if s := GetAttrInts(node, "strides"); len(s) == 2 {
		p.sh, p.sw = int(s[0]), int(s[1])
	} else if len(s) != 0 {
		return p, fmt.Errorf("pool: strides must have 2 entries, got %v", s)
	}
	if p.sh <= 0 || p.sw <= 0 {
		return p, fmt.Errorf("pool: invalid strides %d,%d", p.sh, p.sw)
	}

	if pd := GetAttrInts(node, "pads"); len(pd) == 4 {
		p.padT, p.padL, p.padB, p.padR = int(pd[0]), int(pd[1]), int(pd[2]), int(pd[3])
	} else if len(pd) != 0 {
		return p, fmt.Errorf("pool: pads must have 4 entries for 2D, got %v", pd)
	}

	p.countIncludePad = kind == poolAvg && GetAttrInt(node, "count_include_pad", 0) != 0
	return p, nil
}

// checkUnsupportedPoolAttrs rejects pooling attributes this implementation does
// not model, so a model relying on them fails loudly instead of silently
// producing wrong results.
func checkUnsupportedPoolAttrs(node *Node) error {
	if ap := GetAttrString(node, "auto_pad", autoPadNotset); ap != autoPadNotset && ap != autoPadValid && ap != "" {
		return fmt.Errorf("pool: auto_pad=%q not supported (use explicit pads)", ap)
	}
	if GetAttrInt(node, "ceil_mode", 0) != 0 {
		return fmt.Errorf("pool: ceil_mode=1 not supported")
	}
	for _, d := range GetAttrInts(node, "dilations") {
		if d != 1 {
			return fmt.Errorf("pool: dilations>1 not supported")
		}
	}
	return nil
}

// poolOutDim computes one spatial output dimension (floor mode).
func poolOutDim(in, padBegin, padEnd, kernel, stride int) int {
	eff := in + padBegin + padEnd - kernel
	if eff < 0 {
		return 0
	}
	return eff/stride + 1
}

func poolForward(src, dst []float32, n, c, h, w, outH, outW int, p poolParams, kind poolKind) {
	// Select the window reducer once, outside the per-element loops.
	window := poolWindowMax
	if kind == poolAvg {
		window = poolWindowAvg
	}
	for ni := range n {
		for ci := range c {
			inBase := (ni*c + ci) * h * w
			outBase := (ni*c + ci) * outH * outW
			for oh := range outH {
				hStart := oh*p.sh - p.padT
				for ow := range outW {
					wStart := ow*p.sw - p.padL
					dst[outBase+oh*outW+ow] = window(src, inBase, hStart, wStart, h, w, p)
				}
			}
		}
	}
}

// poolWindowMax reduces one window with max. Out-of-bounds (padded) positions
// are skipped, so they never win the max.
func poolWindowMax(src []float32, inBase, hStart, wStart, h, w int, p poolParams) float32 {
	acc := float32(math.Inf(-1))
	for dh := range p.kh {
		ih := hStart + dh
		if ih < 0 || ih >= h {
			continue
		}
		row := inBase + ih*w
		for dw := range p.kw {
			iw := wStart + dw
			if iw < 0 || iw >= w {
				continue
			}
			if v := src[row+iw]; v > acc {
				acc = v
			}
		}
	}
	return acc
}

// poolWindowAvg reduces one window with mean. Padded positions are excluded
// from the denominator unless count_include_pad is set.
func poolWindowAvg(src []float32, inBase, hStart, wStart, h, w int, p poolParams) float32 {
	var sum float32
	count := 0
	for dh := range p.kh {
		ih := hStart + dh
		if ih < 0 || ih >= h {
			continue
		}
		row := inBase + ih*w
		for dw := range p.kw {
			iw := wStart + dw
			if iw < 0 || iw >= w {
				continue
			}
			sum += src[row+iw]
			count++
		}
	}
	denom := count
	if p.countIncludePad {
		denom = p.kh * p.kw
	}
	if denom > 0 {
		return sum / float32(denom)
	}
	return 0
}
