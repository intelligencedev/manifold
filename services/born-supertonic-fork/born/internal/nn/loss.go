package nn

import (
	"github.com/born-ml/born/internal/tensor"
)

// MSELoss computes Mean Squared Error loss.
//
// Loss = mean((predictions - targets)²)
//
// MSE is commonly used for regression tasks where the goal is to predict
// continuous values.
//
// Example:
//
//	mse := nn.NewMSELoss[Backend]()
//	predictions := model.Forward(input)
//	loss := mse.Forward(predictions, targets)
type MSELoss[B tensor.Backend] struct {
	backend B
}

// NewMSELoss creates a new MSE loss function.
func NewMSELoss[B tensor.Backend](backend B) *MSELoss[B] {
	return &MSELoss[B]{
		backend: backend,
	}
}

// Forward computes the MSE loss.
//
// Loss = mean((predictions - targets)²)
//
// Parameters:
//   - predictions: Model predictions with shape [batch_size, ...]
//   - targets: Ground truth targets with same shape as predictions
//
// Returns a scalar loss value (shape [1] or []).
func (m *MSELoss[B]) Forward(predictions, targets *tensor.Tensor[float32, B]) *tensor.Tensor[float32, B] {
	// Validate shapes match
	if !predictions.Shape().Equal(targets.Shape()) {
		panic("MSELoss: predictions and targets must have the same shape")
	}

	// Compute difference: (predictions - targets)
	diff := predictions.Sub(targets)

	// Square: (predictions - targets)²
	squared := diff.Mul(diff)

	// Mean: sum / num_elements
	data := squared.Raw().AsFloat32()
	var sum float32
	for _, v := range data {
		sum += v
	}
	mean := sum / float32(len(data))

	// Return scalar loss
	lossRaw, err := tensor.NewRaw(tensor.Shape{1}, tensor.Float32, m.backend.Device())
	if err != nil {
		panic(err)
	}
	lossRaw.AsFloat32()[0] = mean

	return tensor.New[float32, B](lossRaw, m.backend)
}

// Parameters returns an empty slice (loss functions have no trainable parameters).
func (m *MSELoss[B]) Parameters() []*Parameter[B] {
	return nil
}

// NOTE: CrossEntropyLoss has been moved to cross_entropy.go
// See internal/nn/cross_entropy.go for the full implementation with numerical stability.
