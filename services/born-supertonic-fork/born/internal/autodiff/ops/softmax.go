package ops

import (
	"math"

	"github.com/born-ml/born/internal/tensor"
)

// SoftmaxOp represents the softmax operation along a specified dimension.
//
// Forward (for each slice along dim):
//
//	softmax(x)_i = exp(x_i - max(x)) / Σ_j exp(x_j - max(x))
//
// The max-shifting ensures numerical stability (prevents overflow).
//
// Backward:
//
//	The Jacobian of softmax is:
//	∂softmax_i/∂x_j = softmax_i * (δ_ij - softmax_j)
//
//	Chain rule gives:
//	∂L/∂x_j = Σ_i (∂L/∂softmax_i) * softmax_i * (δ_ij - softmax_j)
//	        = softmax_j * (∂L/∂softmax_j - Σ_i (∂L/∂softmax_i * softmax_i))
//
//	Simplified formula:
//	∂L/∂x = y * (upstream_grad - sum(y * upstream_grad, dim=axis, keepdim=True))
//
// Supports:
//   - N-dimensional tensors (2D, 3D, 4D, etc.)
//   - Softmax applied along any dimension (positive or negative indexing)
type SoftmaxOp struct {
	input  *tensor.RawTensor
	output *tensor.RawTensor // Cached softmax output for backward pass
	dim    int               // Dimension along which softmax was applied
}

// NewSoftmaxOp creates a new softmax operation.
func NewSoftmaxOp(input, output *tensor.RawTensor, dim int) *SoftmaxOp {
	return &SoftmaxOp{
		input:  input,
		output: output,
		dim:    dim,
	}
}

// Inputs returns the input tensors.
func (op *SoftmaxOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor.
func (op *SoftmaxOp) Output() *tensor.RawTensor {
	return op.output
}

// Backward computes the gradient with respect to input.
//
// Uses the simplified formula:
//
//	∂L/∂x = y * (upstream_grad - sum(y * upstream_grad, dim=axis, keepdim=True))
//
// Where:
//   - y is the softmax output (op.output)
//   - upstream_grad is the gradient from the next layer
//   - sum is performed along the same dimension as softmax (op.dim)
//
// This formula works for N-dimensional tensors (2D, 3D, 4D, etc.).
func (op *SoftmaxOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	shape := op.output.Shape()
	dim := op.dim

	// Normalize negative dimension
	if dim < 0 {
		dim = len(shape) + dim
	}

	// Validate dimension
	if dim < 0 || dim >= len(shape) {
		panic("SoftmaxOp: invalid dimension for backward pass")
	}

	// Step 1: y * upstream_grad (element-wise)
	yTimesGrad := backend.Mul(op.output, outputGrad)

	// Step 2: sum(y * upstream_grad, dim=axis, keepdim=True)
	sumYG := backend.SumDim(yTimesGrad, dim, true)

	// Step 3: upstream_grad - sum(...)
	diff := backend.Sub(outputGrad, sumYG)

	// Step 4: y * diff (element-wise)
	inputGrad := backend.Mul(op.output, diff)

	return []*tensor.RawTensor{inputGrad}
}

// LogSoftmaxOp represents the log-softmax operation.
//
// Forward:
//
//	log_softmax(x)_i = x_i - max(x) - log(Σ_j exp(x_j - max(x)))
//
// This is more numerically stable than computing softmax then log.
//
// Backward:
//
//	∂L/∂x_j = ∂L/∂log_softmax_j - softmax_j * Σ_i ∂L/∂log_softmax_i
//
// Note: We need to cache both log_softmax (output) and softmax for backward.
type LogSoftmaxOp struct {
	input       *tensor.RawTensor
	output      *tensor.RawTensor // log_softmax output
	softmaxData []float32         // Cached softmax for backward (float32 only for now)
}

// NewLogSoftmaxOp creates a new log-softmax operation.
//
// Parameters:
//   - input: Input logits
//   - output: Log-softmax output
//   - softmaxData: Pre-computed softmax (needed for backward)
func NewLogSoftmaxOp(input, output *tensor.RawTensor, softmaxData []float32) *LogSoftmaxOp {
	return &LogSoftmaxOp{
		input:       input,
		output:      output,
		softmaxData: softmaxData,
	}
}

// Inputs returns the input tensors.
func (op *LogSoftmaxOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.input}
}

// Output returns the output tensor.
func (op *LogSoftmaxOp) Output() *tensor.RawTensor {
	return op.output
}

// Backward computes gradient for log-softmax.
//
// Formula:
//
//	∂L/∂x[b,j] = ∂L/∂log_softmax[b,j] - softmax[b,j] * Σ_i ∂L/∂log_softmax[b,i]
func (op *LogSoftmaxOp) Backward(outputGrad *tensor.RawTensor, _ tensor.Backend) []*tensor.RawTensor {
	shape := op.input.Shape()
	if len(shape) != 2 {
		panic("LogSoftmaxOp: backward only supports 2D tensors")
	}

	batchSize := shape[0]
	numClasses := shape[1]

	inputGrad, err := tensor.NewRaw(shape, op.input.DType(), op.input.Device())
	if err != nil {
		panic(err)
	}

	switch op.input.DType() {
	case tensor.Float32:
		outGradData := outputGrad.AsFloat32()
		inGradData := inputGrad.AsFloat32()

		for b := 0; b < batchSize; b++ {
			// Sum gradient over classes: Σ_i ∂L/∂log_softmax[i]
			gradSum := float32(0.0)
			for j := 0; j < numClasses; j++ {
				idx := b*numClasses + j
				gradSum += outGradData[idx]
			}

			// Compute gradient
			for j := 0; j < numClasses; j++ {
				idx := b*numClasses + j
				inGradData[idx] = outGradData[idx] - op.softmaxData[idx]*gradSum
			}
		}

	default:
		panic("LogSoftmaxOp: backward only supports float32 for now")
	}

	return []*tensor.RawTensor{inputGrad}
}

// softmaxFloat32 computes softmax for float32 data.
func softmaxFloat32(inputData, outputData []float32, batchSize, numClasses int) {
	for b := 0; b < batchSize; b++ {
		// Find max for numerical stability
		offset := b * numClasses
		maxVal := inputData[offset]
		for j := 1; j < numClasses; j++ {
			if inputData[offset+j] > maxVal {
				maxVal = inputData[offset+j]
			}
		}

		// Compute exp and sum
		sumExp := float32(0.0)
		for j := 0; j < numClasses; j++ {
			idx := offset + j
			outputData[idx] = float32(math.Exp(float64(inputData[idx] - maxVal)))
			sumExp += outputData[idx]
		}

		// Normalize
		for j := 0; j < numClasses; j++ {
			outputData[offset+j] /= sumExp
		}
	}
}

// softmaxFloat64 computes softmax for float64 data.
func softmaxFloat64(inputData, outputData []float64, batchSize, numClasses int) {
	for b := 0; b < batchSize; b++ {
		offset := b * numClasses
		maxVal := inputData[offset]
		for j := 1; j < numClasses; j++ {
			if inputData[offset+j] > maxVal {
				maxVal = inputData[offset+j]
			}
		}

		sumExp := 0.0
		for j := 0; j < numClasses; j++ {
			idx := offset + j
			outputData[idx] = math.Exp(inputData[idx] - maxVal)
			sumExp += outputData[idx]
		}

		for j := 0; j < numClasses; j++ {
			outputData[offset+j] /= sumExp
		}
	}
}

// Softmax computes softmax along last dimension (helper function).
//
// This is a helper for use outside autodiff.
// For autodiff support, use backend.Softmax() which records SoftmaxOp.
func Softmax(input *tensor.RawTensor, device tensor.Device) *tensor.RawTensor {
	shape := input.Shape()
	if len(shape) != 2 {
		panic("Softmax: only supports 2D tensors [batch_size, num_classes]")
	}

	batchSize := shape[0]
	numClasses := shape[1]

	output, err := tensor.NewRaw(shape, input.DType(), device)
	if err != nil {
		panic(err)
	}

	switch input.DType() {
	case tensor.Float32:
		softmaxFloat32(input.AsFloat32(), output.AsFloat32(), batchSize, numClasses)

	case tensor.Float64:
		softmaxFloat64(input.AsFloat64(), output.AsFloat64(), batchSize, numClasses)

	default:
		panic("Softmax: only supports float32 and float64")
	}

	return output
}
