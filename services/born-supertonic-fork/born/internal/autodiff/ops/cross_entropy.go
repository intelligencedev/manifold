package ops

import (
	"math"

	"github.com/born-ml/born/internal/tensor"
)

// buildOneHotIdentity creates a float identity matrix of shape [n, n] in the given dtype.
// Row i is the i-th standard basis vector (one-hot for class i).
// Only n scalar writes touch CPU; the logits/targets batch data never crosses the bus.
func buildOneHotIdentity(n int, dtype tensor.DataType, device tensor.Device) *tensor.RawTensor {
	identity, err := tensor.NewRaw(tensor.Shape{n, n}, dtype, device)
	if err != nil {
		panic(err)
	}

	switch dtype {
	case tensor.Float32:
		data := identity.AsFloat32()
		for i := 0; i < n; i++ {
			data[i*n+i] = 1.0
		}
	case tensor.Float64:
		data := identity.AsFloat64()
		for i := 0; i < n; i++ {
			data[i*n+i] = 1.0
		}
	default:
		panic("buildOneHotIdentity: only float32 and float64 are supported")
	}

	return identity
}

// CrossEntropyOp represents the cross-entropy loss operation.
//
// Forward:
//
//	Loss = mean(-log_softmax(logits)[targets])
//
// Where log_softmax uses the log-sum-exp trick for numerical stability:
//
//	log_softmax(z) = z - (max(z) + log(Σ exp(z - max(z))))
//
// Backward:
//
//	∂L/∂logits = (softmax(logits) - y_one_hot) / batch_size
//
// This elegant gradient formula is the key reason why softmax + cross-entropy
// are often fused together in modern frameworks (PyTorch, TensorFlow, Burn).
//
// Assumptions:
//   - Logits shape: [batch_size, num_classes] (2D)
//   - Targets shape: [batch_size] (1D, class indices)
//   - Output: scalar loss (mean over batch)
type CrossEntropyOp struct {
	logits  *tensor.RawTensor // Input logits [batch_size, num_classes]
	targets *tensor.RawTensor // Target class indices [batch_size]
	output  *tensor.RawTensor // Scalar loss output
}

// NewCrossEntropyOp creates a new cross-entropy operation.
func NewCrossEntropyOp(logits, targets, output *tensor.RawTensor) *CrossEntropyOp {
	return &CrossEntropyOp{
		logits:  logits,
		targets: targets,
		output:  output,
	}
}

// Inputs returns the input tensors.
func (op *CrossEntropyOp) Inputs() []*tensor.RawTensor {
	return []*tensor.RawTensor{op.logits}
}

// Output returns the output tensor.
func (op *CrossEntropyOp) Output() *tensor.RawTensor {
	return op.output
}

// Backward computes the gradient with respect to logits using backend ops only.
//
// Gradient formula:
//
//	∂L/∂logits[b,i] = (softmax(logits[b])[i] - y_one_hot[b,i]) / batch_size * outputGrad
//
// Where y_one_hot[b,i] = 1 if i == targets[b], else 0.
//
// Implementation strategy (no GPU→CPU readback for batch data):
//
//  1. softmax = backend.Softmax(logits, -1)         — [batch, classes], stays on device
//  2. identity = float identity matrix [classes, classes]  — only numClasses scalar writes
//  3. oneHot = backend.Embedding(identity, targets)  — [batch, classes], no targets readback
//  4. diff = backend.Sub(softmax, oneHot)            — [batch, classes]
//  5. scaled = backend.DivScalar(diff, float(batch)) — divide by batchSize
//  6. gradScale = outputGrad.AsFloat32/64()[0]       — single scalar readback (upstream grad)
//  7. gradInput = backend.MulScalar(scaled, gradScale)
//
// The identity matrix approach for one-hot encodes each target class i as identity[i],
// which is the i-th standard basis vector. backend.Embedding looks up rows by index,
// so Embedding(identity, targets) = one_hot(targets) without any CPU readback of targets.
//
// Note: outputGrad is a scalar loss gradient [1]; reading one float is acceptable here.
func (op *CrossEntropyOp) Backward(outputGrad *tensor.RawTensor, backend tensor.Backend) []*tensor.RawTensor {
	logitsShape := op.logits.Shape()
	if len(logitsShape) != 2 {
		panic("CrossEntropyOp: backward only supports 2D logits [batch_size, num_classes]")
	}

	batchSize := logitsShape[0]
	numClasses := logitsShape[1]
	dtype := op.logits.DType()

	// Step 1: softmax(logits) — [batch, classes], computed on backend (GPU stays on GPU).
	softmaxProbs := backend.Softmax(op.logits, -1)

	// Step 2: Build identity matrix [classes, classes] in logits dtype.
	// Only numClasses scalar 1.0 values are written on CPU; no batch data crosses the bus.
	identity := buildOneHotIdentity(numClasses, dtype, op.logits.Device())

	// Step 3: one_hot = Embedding(identity, targets) — [batch, classes].
	// targets remain on device; Embedding reads their values internally via AsInt32()
	// only to index into the identity table (O(numClasses) rows, not O(batch*classes)).
	oneHot := backend.Embedding(identity, op.targets)

	// Step 4: diff = softmax - one_hot — [batch, classes].
	// softmaxProbs is a fresh tensor from Softmax, safe as the first arg (CPU in-place optimizes
	// the 'a' operand, so using a fresh tensor avoids aliasing issues with stored op.logits).
	diff := backend.Sub(softmaxProbs, oneHot)

	// Step 5: scale by 1/batchSize (typed scalar to satisfy backend type assertion).
	var batchScale any
	switch dtype {
	case tensor.Float32:
		batchScale = float32(1.0) / float32(batchSize)
	default: // Float64
		batchScale = 1.0 / float64(batchSize)
	}

	scaled := backend.MulScalar(diff, batchScale)

	// Step 6: extract upstream gradient scalar — shape [1], single element readback.
	// This is acceptable: we are reading one float (the scalar loss gradient), not batch data.
	var gradScale any
	switch dtype {
	case tensor.Float32:
		gradScale = outputGrad.AsFloat32()[0]
	default: // Float64
		gradScale = outputGrad.AsFloat64()[0]
	}

	// Step 7: chain rule — multiply by upstream gradient.
	gradInput := backend.MulScalar(scaled, gradScale)

	return []*tensor.RawTensor{gradInput}
}

// CrossEntropyForward computes cross-entropy loss (helper function).
//
// This is a helper for use outside autodiff context.
// For autodiff support, use AutodiffBackend with CrossEntropyOp.
//
// Parameters:
//   - logits: [batch_size, num_classes]
//   - targets: [batch_size] (class indices)
//
// Returns:
//   - Scalar loss tensor (mean over batch)
func CrossEntropyForward(logits, targets *tensor.RawTensor, device tensor.Device) *tensor.RawTensor {
	logitsShape := logits.Shape()
	if len(logitsShape) != 2 {
		panic("CrossEntropyForward: logits must be 2D [batch_size, num_classes]")
	}

	targetsShape := targets.Shape()
	if len(targetsShape) != 1 {
		panic("CrossEntropyForward: targets must be 1D [batch_size]")
	}

	batchSize := logitsShape[0]
	numClasses := logitsShape[1]

	if targetsShape[0] != batchSize {
		panic("CrossEntropyForward: batch size mismatch between logits and targets")
	}

	// Create scalar output
	output, err := tensor.NewRaw(tensor.Shape{1}, logits.DType(), device)
	if err != nil {
		panic(err)
	}

	switch logits.DType() {
	case tensor.Float32:
		logitsData := logits.AsFloat32()
		targetsData := targets.AsInt32()

		totalLoss := float32(0.0)

		for b := 0; b < batchSize; b++ {
			sampleLogits := logitsData[b*numClasses : (b+1)*numClasses]
			logProbs := computeLogSoftmaxFloat32(sampleLogits)

			target := int(targetsData[b])
			if target < 0 || target >= numClasses {
				panic("CrossEntropyForward: target index out of bounds")
			}

			// Negative log-likelihood
			totalLoss += -logProbs[target]
		}

		// Average over batch
		output.AsFloat32()[0] = totalLoss / float32(batchSize)

	case tensor.Float64:
		logitsData := logits.AsFloat64()
		targetsData := targets.AsInt32()

		totalLoss := 0.0

		for b := 0; b < batchSize; b++ {
			sampleLogits := logitsData[b*numClasses : (b+1)*numClasses]
			logProbs := computeLogSoftmaxFloat64(sampleLogits)

			target := int(targetsData[b])
			if target < 0 || target >= numClasses {
				panic("CrossEntropyForward: target index out of bounds")
			}

			totalLoss += -logProbs[target]
		}

		output.AsFloat64()[0] = totalLoss / float64(batchSize)

	default:
		panic("CrossEntropyForward: only supports float32 and float64")
	}

	return output
}

// computeLogSoftmaxFloat32 computes log-softmax with numerical stability.
func computeLogSoftmaxFloat32(logits []float32) []float32 {
	n := len(logits)
	result := make([]float32, n)

	// Find max for numerical stability
	maxVal := logits[0]
	for i := 1; i < n; i++ {
		if logits[i] > maxVal {
			maxVal = logits[i]
		}
	}

	// Compute log-sum-exp: log(Σ exp(z - max))
	sumExp := float32(0.0)
	for i := 0; i < n; i++ {
		sumExp += float32(math.Exp(float64(logits[i] - maxVal)))
	}
	logSumExp := maxVal + float32(math.Log(float64(sumExp)))

	// log_softmax = z - log_sum_exp
	for i := 0; i < n; i++ {
		result[i] = logits[i] - logSumExp
	}

	return result
}

// computeLogSoftmaxFloat64 computes log-softmax with numerical stability.
func computeLogSoftmaxFloat64(logits []float64) []float64 {
	n := len(logits)
	result := make([]float64, n)

	// Find max for numerical stability
	maxVal := logits[0]
	for i := 1; i < n; i++ {
		if logits[i] > maxVal {
			maxVal = logits[i]
		}
	}

	// Compute log-sum-exp
	sumExp := 0.0
	for i := 0; i < n; i++ {
		sumExp += math.Exp(logits[i] - maxVal)
	}
	logSumExp := maxVal + math.Log(sumExp)

	// log_softmax = z - log_sum_exp
	for i := 0; i < n; i++ {
		result[i] = logits[i] - logSumExp
	}

	return result
}
