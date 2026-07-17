package nn

import (
	"math"

	"github.com/born-ml/born/internal/tensor"
)

// CrossEntropyLoss computes cross-entropy loss for multi-class classification.
//
// This implementation uses the LogSoftmax + NLLLoss decomposition for
// numerical stability, following modern best practices (PyTorch, Burn 2025).
//
// Mathematical Formulation:
//
//	Loss = -log_probs[target]
//	where log_probs = LogSoftmax(logits)
//
// Gradient (Backward):
//
//	∂L/∂logits = Softmax(logits) - y_one_hot
//
// Usage:
//
//	criterion := nn.NewCrossEntropyLoss[Backend](backend)
//	logits := model.Forward(input)  // [batch_size, num_classes]
//	loss := criterion.Forward(logits, targets)  // targets: [batch_size] (class indices)
//
// Key Properties:
//   - Expects raw logits (unnormalized scores) as input
//   - Uses log-sum-exp trick for numerical stability
//   - Prevents overflow when logits > 88 (float32 limit)
//   - Prevents underflow when all logits are very negative
//
// References:
//   - "Adam: A Method for Stochastic Optimization" (Kingma & Ba, 2014)
//   - PyTorch CrossEntropyLoss documentation
//   - Burn framework loss implementations
type CrossEntropyLoss[B tensor.Backend] struct {
	backend B
}

// NewCrossEntropyLoss creates a new cross-entropy loss function.
func NewCrossEntropyLoss[B tensor.Backend](backend B) *CrossEntropyLoss[B] {
	return &CrossEntropyLoss[B]{
		backend: backend,
	}
}

// Forward computes cross-entropy loss.
//
// Parameters:
//   - logits: Model predictions (unnormalized scores) with shape [batch_size, num_classes]
//   - targets: Ground truth class indices with shape [batch_size] (values in range [0, num_classes-1])
//
// Returns:
//   - Scalar loss value (mean over batch)
//
// When using an autodiff-aware backend, this operation is recorded on the tape
// for proper gradient computation during backward pass.
func (c *CrossEntropyLoss[B]) Forward(
	logits *tensor.Tensor[float32, B],
	targets *tensor.Tensor[int32, B],
) *tensor.Tensor[float32, B] {
	// Check if backend supports CrossEntropy (autodiff-aware)
	type CrossEntropyBackend interface {
		CrossEntropy(logits, targets *tensor.RawTensor) *tensor.RawTensor
	}

	if adBackend, ok := any(c.backend).(CrossEntropyBackend); ok {
		// Use autodiff-aware version that records on tape
		resultRaw := adBackend.CrossEntropy(logits.Raw(), targets.Raw())
		return tensor.New[float32, B](resultRaw, c.backend)
	}

	// Fallback to manual computation for non-autodiff backends
	shape := logits.Shape()
	if len(shape) != 2 {
		panic("CrossEntropyLoss: logits must be 2D [batch_size, num_classes]")
	}

	batchSize := shape[0]
	numClasses := shape[1]

	targetsData := targets.Raw().AsInt32()
	if len(targetsData) != batchSize {
		panic("CrossEntropyLoss: targets must have shape [batch_size]")
	}

	logitsData := logits.Raw().AsFloat32()

	// Compute loss for each sample in batch
	totalLoss := float32(0.0)

	for b := 0; b < batchSize; b++ {
		// Extract logits for this sample
		sampleLogits := logitsData[b*numClasses : (b+1)*numClasses]

		// Compute LogSoftmax using log-sum-exp trick
		logProbs := logSoftmax(sampleLogits)

		// Negative log-likelihood loss: -log_probs[target]
		target := int(targetsData[b])
		if target < 0 || target >= numClasses {
			panic("CrossEntropyLoss: target index out of bounds")
		}

		loss := -logProbs[target]
		totalLoss += loss
	}

	// Average over batch
	meanLoss := totalLoss / float32(batchSize)

	// Return scalar loss
	lossRaw, err := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, c.backend.Device())
	if err != nil {
		panic(err)
	}
	lossRaw.AsFloat32()[0] = meanLoss

	return tensor.New[float32, B](lossRaw, c.backend)
}

// Parameters returns an empty slice (loss functions have no trainable parameters).
func (c *CrossEntropyLoss[B]) Parameters() []*Parameter[B] {
	return nil
}

// logSoftmax computes log(softmax(z)) in numerically stable way.
//
// Formula:
//
//	LogSoftmax(z)[i] = z[i] - LogSumExp(z)
//	                 = z[i] - (max(z) + log(Σ exp(z - max(z))))
//
// The log-sum-exp trick prevents overflow by subtracting max(z) before exponentiating.
func logSoftmax(z []float32) []float32 {
	n := len(z)
	result := make([]float32, n)

	// Find maximum for numerical stability
	maxZ := z[0]
	for i := 1; i < n; i++ {
		if z[i] > maxZ {
			maxZ = z[i]
		}
	}

	// Compute sum of exp(z - max)
	sumExp := float32(0.0)
	for i := 0; i < n; i++ {
		sumExp += float32(math.Exp(float64(z[i] - maxZ)))
	}

	// LogSumExp = max + log(sum_exp)
	logSumExp := maxZ + float32(math.Log(float64(sumExp)))

	// LogSoftmax = z - LogSumExp
	for i := 0; i < n; i++ {
		result[i] = z[i] - logSumExp
	}

	return result
}

// softmax computes softmax(z) = exp(LogSoftmax(z)).
//
// This is used for backward pass gradient computation.
//
// Formula:
//
//	Softmax(z)[i] = exp(z[i]) / Σ exp(z[j])
//	              = exp(z[i] - LogSumExp(z))
func softmax(z []float32) []float32 {
	logProbs := logSoftmax(z)
	result := make([]float32, len(logProbs))
	for i, lp := range logProbs {
		result[i] = float32(math.Exp(float64(lp)))
	}
	return result
}

// CrossEntropyBackward computes gradient of CrossEntropyLoss w.r.t. logits.
//
// This function provides manual backward pass for CrossEntropyLoss.
// It will be integrated with autodiff in Phase 2.
//
// Gradient Formula:
//
//	∂L/∂logits[i] = softmax(logits)[i] - y_one_hot[i]
//	              = probs[i] - (1 if i==target else 0)
//
// For single class target:
//
//	∂L/∂logits[i] = probs[i]         if i ≠ target
//	∂L/∂logits[i] = probs[i] - 1     if i = target
//
// Parameters:
//   - logits: [batch_size, num_classes]
//   - targets: [batch_size] (class indices)
//
// Returns:
//   - grads: [batch_size, num_classes] gradient tensor
//
// Note: Gradients are automatically averaged over batch size.
func CrossEntropyBackward[B tensor.Backend](
	logits *tensor.Tensor[float32, B],
	targets *tensor.Tensor[int32, B],
	backend B,
) *tensor.Tensor[float32, B] {
	shape := logits.Shape()
	batchSize := shape[0]
	numClasses := shape[1]

	logitsData := logits.Raw().AsFloat32()
	targetsData := targets.Raw().AsInt32()

	// Create gradient tensor
	gradRaw, err := tensor.NewRaw(shape, tensor.Float32, backend.Device())
	if err != nil {
		panic(err)
	}
	gradData := gradRaw.AsFloat32()

	// Compute gradient for each sample
	for b := 0; b < batchSize; b++ {
		// Extract logits for this sample
		sampleLogits := logitsData[b*numClasses : (b+1)*numClasses]

		// Compute softmax probabilities
		probs := softmax(sampleLogits)

		// Gradient = softmax(z) - y_one_hot
		target := int(targetsData[b])
		for i := 0; i < numClasses; i++ {
			grad := probs[i]
			if i == target {
				grad -= 1.0
			}

			// Average over batch
			gradData[b*numClasses+i] = grad / float32(batchSize)
		}
	}

	return tensor.New[float32, B](gradRaw, backend)
}

// argmax returns the index of the maximum value in the slice.
//
// This is used for computing classification accuracy.
func argmax(z []float32) int {
	maxIdx := 0
	maxVal := z[0]
	for i := 1; i < len(z); i++ {
		if z[i] > maxVal {
			maxVal = z[i]
			maxIdx = i
		}
	}
	return maxIdx
}

// Accuracy computes classification accuracy for a batch.
//
// Parameters:
//   - logits: Model predictions [batch_size, num_classes]
//   - targets: Ground truth class indices [batch_size]
//
// Returns:
//   - Accuracy as a float between 0 and 1.
func Accuracy[B tensor.Backend](
	logits *tensor.Tensor[float32, B],
	targets *tensor.Tensor[int32, B],
) float32 {
	shape := logits.Shape()
	batchSize := shape[0]
	numClasses := shape[1]

	logitsData := logits.Raw().AsFloat32()
	targetsData := targets.Raw().AsInt32()

	correct := 0
	for b := 0; b < batchSize; b++ {
		sampleLogits := logitsData[b*numClasses : (b+1)*numClasses]
		predicted := argmax(sampleLogits)
		target := int(targetsData[b])

		if predicted == target {
			correct++
		}
	}

	return float32(correct) / float32(batchSize)
}
