//go:build !wasm

package operators

import (
	"fmt"
	"math"

	"github.com/born-ml/born/internal/tensor"
)

// registerResizeOps registers the ONNX Resize operator.
func (r *Registry) registerResizeOps() {
	r.Register("Resize", handleResize)
}

type resizeMode int

const (
	resizeNearest resizeMode = iota
	resizeLinear
)

type coordTransformMode int

const (
	coordHalfPixel coordTransformMode = iota
	coordAsymmetric
	coordAlignCorners
	coordPytorchHalfPixel
)

type nearestMode int

const (
	nearestRoundPreferFloor nearestMode = iota
	nearestRoundPreferCeil
	nearestFloor
	nearestCeil
)

// resizeParams holds the resolved attributes and computed dimensions for Resize.
type resizeParams struct {
	mode      resizeMode
	coordMode coordTransformMode
	nearMode  nearestMode
	scaleH    float64
	scaleW    float64
	outH      int
	outW      int
}

// handleResize implements ONNX Resize for 4D NCHW float32 tensors.
//
// Supported modes: nearest, linear (bilinear).
// Supported coordinate transforms: half_pixel, asymmetric, align_corners, pytorch_half_pixel.
// Supported inputs: scales OR sizes (exclusive).
//
// Unsupported features return a clear error: cubic mode, tf_crop_and_resize,
// antialias, axes, keep_aspect_ratio_policy.
func handleResize(_ *Context, node *Node, inputs []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
	if len(inputs) < 1 || inputs[0] == nil {
		return nil, fmt.Errorf("resize: missing input tensor")
	}
	x := inputs[0]
	if x.DType() != tensor.Float32 {
		return nil, fmt.Errorf("resize: only float32 supported, got %s", x.DType())
	}
	shape := x.Shape()
	if len(shape) != 4 {
		return nil, fmt.Errorf("resize: expected 4D NCHW input, got %dD", len(shape))
	}

	p, err := parseResizeParams(node, inputs, shape)
	if err != nil {
		return nil, err
	}

	n, c, h, w := shape[0], shape[1], shape[2], shape[3]
	out, err := tensor.NewRaw(tensor.Shape{n, c, p.outH, p.outW}, tensor.Float32, tensor.CPU)
	if err != nil {
		return nil, fmt.Errorf("resize: %w", err)
	}

	src := x.AsFloat32()
	dst := out.AsFloat32()

	switch p.mode {
	case resizeNearest:
		resizeNearestForward(src, dst, n, c, h, w, p)
	case resizeLinear:
		resizeLinearForward(src, dst, n, c, h, w, p)
	}

	return []*tensor.RawTensor{out}, nil
}

func parseResizeParams(node *Node, inputs []*tensor.RawTensor, shape tensor.Shape) (resizeParams, error) {
	var p resizeParams

	if err := checkUnsupportedResizeAttrs(node); err != nil {
		return p, err
	}

	var err error
	if p.mode, err = parseResizeMode(node); err != nil {
		return p, err
	}
	if p.coordMode, err = parseCoordTransformMode(node); err != nil {
		return p, err
	}
	if p.nearMode, err = parseNearestMode(node); err != nil {
		return p, err
	}

	if err := resolveResizeOutputDims(&p, inputs, shape[2], shape[3]); err != nil {
		return p, err
	}
	return p, nil
}

func parseResizeMode(node *Node) (resizeMode, error) {
	switch s := GetAttrString(node, "mode", "nearest"); s {
	case "nearest":
		return resizeNearest, nil
	case "linear":
		return resizeLinear, nil
	default:
		return 0, fmt.Errorf("resize: mode=%q not supported (supported: nearest, linear)", s)
	}
}

func parseCoordTransformMode(node *Node) (coordTransformMode, error) {
	switch s := GetAttrString(node, "coordinate_transformation_mode", "half_pixel"); s {
	case "half_pixel":
		return coordHalfPixel, nil
	case "asymmetric":
		return coordAsymmetric, nil
	case "align_corners":
		return coordAlignCorners, nil
	case "pytorch_half_pixel":
		return coordPytorchHalfPixel, nil
	default:
		return 0, fmt.Errorf("resize: coordinate_transformation_mode=%q not supported (supported: half_pixel, asymmetric, align_corners, pytorch_half_pixel)", s)
	}
}

func parseNearestMode(node *Node) (nearestMode, error) {
	switch s := GetAttrString(node, "nearest_mode", "round_prefer_floor"); s {
	case "round_prefer_floor":
		return nearestRoundPreferFloor, nil
	case "round_prefer_ceil":
		return nearestRoundPreferCeil, nil
	case "floor":
		return nearestFloor, nil
	case "ceil":
		return nearestCeil, nil
	default:
		return 0, fmt.Errorf("resize: nearest_mode=%q not supported", s)
	}
}

func resolveResizeOutputDims(p *resizeParams, inputs []*tensor.RawTensor, h, w int) error {
	if len(inputs) >= 4 && inputs[3] != nil && len(inputs[3].AsInt64()) > 0 {
		return resolveFromSizes(p, inputs[3].AsInt64(), h, w)
	}
	if len(inputs) >= 3 && inputs[2] != nil && len(inputs[2].AsFloat32()) > 0 {
		return resolveFromScales(p, inputs[2].AsFloat32(), h, w)
	}
	return fmt.Errorf("resize: either scales or sizes input must be provided (non-empty)")
}

func resolveFromSizes(p *resizeParams, sizes []int64, h, w int) error {
	if len(sizes) != 4 {
		return fmt.Errorf("resize: sizes must have 4 entries (NCHW), got %d", len(sizes))
	}
	p.outH = int(sizes[2])
	p.outW = int(sizes[3])
	if p.outH <= 0 || p.outW <= 0 {
		return fmt.Errorf("resize: invalid output sizes %dx%d", p.outH, p.outW)
	}
	p.scaleH = float64(p.outH) / float64(h)
	p.scaleW = float64(p.outW) / float64(w)
	return nil
}

func resolveFromScales(p *resizeParams, scales []float32, h, w int) error {
	if len(scales) != 4 {
		return fmt.Errorf("resize: scales must have 4 entries (NCHW), got %d", len(scales))
	}
	if scales[0] != 1.0 || scales[1] != 1.0 {
		return fmt.Errorf("resize: batch/channel scales must be 1.0, got [%.2f, %.2f, ...]", scales[0], scales[1])
	}
	p.scaleH = float64(scales[2])
	p.scaleW = float64(scales[3])
	p.outH = int(math.Floor(float64(h) * p.scaleH))
	p.outW = int(math.Floor(float64(w) * p.scaleW))
	if p.outH <= 0 || p.outW <= 0 {
		return fmt.Errorf("resize: invalid computed output dims %dx%d from scales [%.4f, %.4f]", p.outH, p.outW, p.scaleH, p.scaleW)
	}
	return nil
}

func checkUnsupportedResizeAttrs(node *Node) error {
	if GetAttrInt(node, "antialias", 0) != 0 {
		return fmt.Errorf("resize: antialias=1 not supported")
	}
	if GetAttrInt(node, "exclude_outside", 0) != 0 {
		return fmt.Errorf("resize: exclude_outside=1 not supported")
	}
	if axes := GetAttrInts(node, "axes"); len(axes) > 0 {
		return fmt.Errorf("resize: axes attribute not supported (resize all spatial dims)")
	}
	if karp := GetAttrString(node, "keep_aspect_ratio_policy", "stretch"); karp != "stretch" {
		return fmt.Errorf("resize: keep_aspect_ratio_policy=%q not supported", karp)
	}
	return nil
}

// coordTransform maps an output coordinate to an input coordinate.
func coordTransform(outIdx int, scale float64, inLen, outLen int, mode coordTransformMode) float64 {
	switch mode {
	case coordAsymmetric:
		return float64(outIdx) / scale
	case coordHalfPixel:
		return (float64(outIdx)+0.5)/scale - 0.5
	case coordAlignCorners:
		if outLen <= 1 {
			return 0
		}
		return float64(outIdx) * float64(inLen-1) / float64(outLen-1)
	case coordPytorchHalfPixel:
		if outLen <= 1 {
			return 0
		}
		return (float64(outIdx)+0.5)/scale - 0.5
	default:
		return float64(outIdx) / scale
	}
}

// nearestIdx rounds a float coordinate to the nearest integer index per the nearest_mode.
func nearestIdx(val float64, mode nearestMode) int {
	switch mode {
	case nearestFloor:
		return int(math.Floor(val))
	case nearestCeil:
		return int(math.Ceil(val))
	case nearestRoundPreferFloor:
		return int(roundPreferFloor(val))
	case nearestRoundPreferCeil:
		return int(roundPreferCeil(val))
	default:
		return int(math.Floor(val))
	}
}

func roundPreferFloor(v float64) float64 {
	// Round to nearest, ties go to floor (e.g., 0.5 → 0, 1.5 → 1).
	return math.Ceil(v - 0.5)
}

func roundPreferCeil(v float64) float64 {
	// Round to nearest, ties go to ceil (e.g., 0.5 → 1, 1.5 → 2).
	return math.Floor(v + 0.5)
}

// clampInt clamps v to [0, maxVal-1].
func clampInt(v, maxVal int) int {
	if v < 0 {
		return 0
	}
	if v >= maxVal {
		return maxVal - 1
	}
	return v
}

func resizeNearestForward(src, dst []float32, n, c, h, w int, p resizeParams) {
	for ni := range n {
		for ci := range c {
			inBase := (ni*c + ci) * h * w
			outBase := (ni*c + ci) * p.outH * p.outW
			for oh := range p.outH {
				origH := coordTransform(oh, p.scaleH, h, p.outH, p.coordMode)
				ih := clampInt(nearestIdx(origH, p.nearMode), h)
				for ow := range p.outW {
					origW := coordTransform(ow, p.scaleW, w, p.outW, p.coordMode)
					iw := clampInt(nearestIdx(origW, p.nearMode), w)
					dst[outBase+oh*p.outW+ow] = src[inBase+ih*w+iw]
				}
			}
		}
	}
}

func resizeLinearForward(src, dst []float32, n, c, h, w int, p resizeParams) {
	for ni := range n {
		for ci := range c {
			inBase := (ni*c + ci) * h * w
			outBase := (ni*c + ci) * p.outH * p.outW
			for oh := range p.outH {
				origH := coordTransform(oh, p.scaleH, h, p.outH, p.coordMode)
				ih0 := int(math.Floor(origH))
				ih1 := ih0 + 1
				dh := origH - float64(ih0)

				ih0 = clampInt(ih0, h)
				ih1 = clampInt(ih1, h)

				for ow := range p.outW {
					origW := coordTransform(ow, p.scaleW, w, p.outW, p.coordMode)
					iw0 := int(math.Floor(origW))
					iw1 := iw0 + 1
					dw := origW - float64(iw0)

					iw0 = clampInt(iw0, w)
					iw1 = clampInt(iw1, w)

					v00 := float64(src[inBase+ih0*w+iw0])
					v01 := float64(src[inBase+ih0*w+iw1])
					v10 := float64(src[inBase+ih1*w+iw0])
					v11 := float64(src[inBase+ih1*w+iw1])

					val := v00*(1-dh)*(1-dw) + v01*(1-dh)*dw + v10*dh*(1-dw) + v11*dh*dw
					dst[outBase+oh*p.outW+ow] = float32(val)
				}
			}
		}
	}
}
