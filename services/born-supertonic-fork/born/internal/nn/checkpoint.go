package nn

import (
	"fmt"
	"time"

	"github.com/born-ml/born/internal/serialization"
	"github.com/born-ml/born/internal/tensor"
)

// OptimizerState represents an optimizer that can save/load its state.
//
// This interface is used by checkpoints to serialize optimizer state
// without creating import cycles. Optimizers from the optim package
// implement this interface.
type OptimizerState interface {
	// StateDict returns the optimizer state for serialization.
	StateDict() map[string]*tensor.RawTensor

	// LoadStateDict loads optimizer state from serialization.
	LoadStateDict(stateDict map[string]*tensor.RawTensor) error

	// GetLR returns the current learning rate.
	GetLR() float32
}

// Checkpoint represents a complete training state snapshot.
//
// A checkpoint includes:
//   - Model parameters (weights and biases)
//   - Optimizer state (momentum buffers, Adam moments, etc.)
//   - Training metadata (epoch, step, loss)
//   - Custom metadata
//
// Checkpoints enable training to be resumed from a specific point,
// which is essential for:
//   - Long-running training jobs that might be interrupted
//   - Hyperparameter tuning and experimentation
//   - Model ensembling and averaging
//
// Example:
//
//	checkpoint := &nn.Checkpoint[cpu.Backend]{
//	    Model:     model,
//	    Optimizer: optimizer,
//	    Epoch:     10,
//	    Step:      5000,
//	    Loss:      0.123,
//	    Metadata:  map[string]any{"lr": 0.001, "batch_size": 32},
//	}
//	err := checkpoint.Save("checkpoint_epoch_10.born")
//
// To resume training:
//
//	checkpoint, err := nn.LoadCheckpoint[cpu.Backend]("checkpoint.born", backend, model, optimizer)
//	startEpoch := checkpoint.Epoch + 1
//
// Type parameter B must satisfy the tensor.Backend interface.
type Checkpoint[B tensor.Backend] struct {
	Model     Module[B]      // The neural network model
	Optimizer OptimizerState // The optimizer with its state
	Epoch     int            // Training epoch number
	Step      int64          // Training step number
	Loss      float64        // Loss value at this checkpoint
	Metadata  map[string]any // Additional training metadata
	CreatedAt time.Time      // When the checkpoint was created
}

// Save saves the checkpoint to a .born file.
//
// This writes:
//   - Model parameters via Module.StateDict()
//   - Optimizer state via Optimizer.StateDict()
//   - Training metadata (epoch, step, loss)
//
// The resulting file can be loaded with LoadCheckpoint to resume training.
//
// Parameters:
//   - path: File path to write checkpoint to
//
// Returns an error if saving fails.
func (c *Checkpoint[B]) Save(path string) (err error) {
	// Get model state dict
	modelStateDict := c.Model.StateDict()

	// Get optimizer state dict
	optimizerStateDict := c.Optimizer.StateDict()

	// Combine model and optimizer state
	// Prefix optimizer state with "optimizer."
	combinedStateDict := make(map[string]*tensor.RawTensor)

	for name, raw := range modelStateDict {
		combinedStateDict[name] = raw
	}

	for name, raw := range optimizerStateDict {
		combinedStateDict["optimizer."+name] = raw
	}

	// Build checkpoint metadata
	checkpointMeta := &serialization.CheckpointMeta{
		IsCheckpoint:    true,
		Epoch:           c.Epoch,
		Step:            c.Step,
		Loss:            c.Loss,
		OptimizerType:   getOptimizerType(c.Optimizer),
		OptimizerConfig: getOptimizerConfig(c.Optimizer),
		TrainingMeta:    c.Metadata,
	}

	// Create writer
	writer, err := serialization.NewBornWriter(path)
	if err != nil {
		return fmt.Errorf("failed to create writer: %w", err)
	}
	defer func() {
		if closeErr := writer.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	// Prepare header
	header := serialization.Header{
		FormatVersion:  serialization.FormatVersion,
		BornVersion:    "0.5.4", // TODO: Get from build info
		ModelType:      "Checkpoint",
		CreatedAt:      time.Now().UTC(),
		Metadata:       make(map[string]string),
		CheckpointMeta: checkpointMeta,
	}

	// Write combined state dict with custom header
	if err := writer.WriteStateDictWithHeader(combinedStateDict, header); err != nil {
		return fmt.Errorf("failed to write checkpoint: %w", err)
	}

	return nil
}

// LoadCheckpoint loads a checkpoint from a .born file.
//
// This restores:
//   - Model parameters into the provided model
//   - Optimizer state into the provided optimizer
//   - Training metadata
//
// The model and optimizer must be pre-constructed with the same architecture
// and configuration as when the checkpoint was saved.
//
// Parameters:
//   - path: File path to read checkpoint from
//   - backend: Backend to use for tensor operations
//   - model: Pre-constructed model (will be loaded into)
//   - optimizer: Pre-constructed optimizer (will be loaded into)
//
// Returns the checkpoint with restored state, or an error if loading fails.
//
// Example:
//
//	// Create model and optimizer with same architecture
//	model := nn.NewLinear[cpu.Backend](10, 5, backend)
//	optimizer := optim.NewAdam(model.Parameters(), optim.AdamConfig{LR: 0.001}, backend)
//
//	// Load checkpoint
//	checkpoint, err := nn.LoadCheckpoint("checkpoint.born", backend, model, optimizer)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Resume training from checkpoint.Epoch + 1
//	for epoch := checkpoint.Epoch + 1; epoch < totalEpochs; epoch++ {
//	    // Training loop...
//	}
func LoadCheckpoint[B tensor.Backend](
	path string,
	backend B,
	model Module[B],
	optimizer OptimizerState,
) (checkpoint *Checkpoint[B], err error) {
	// Create reader
	reader, err := serialization.NewBornReader(path)
	if err != nil {
		return nil, fmt.Errorf("failed to create reader: %w", err)
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	// Read header
	header := reader.Header()

	// Verify this is a checkpoint file
	if header.CheckpointMeta == nil || !header.CheckpointMeta.IsCheckpoint {
		return nil, fmt.Errorf("file is not a checkpoint")
	}

	// Read state dictionary
	stateDict, err := reader.ReadStateDict(backend)
	if err != nil {
		return nil, fmt.Errorf("failed to read state dict: %w", err)
	}

	// Split model and optimizer state
	modelStateDict := make(map[string]*tensor.RawTensor)
	optimizerStateDict := make(map[string]*tensor.RawTensor)

	for name, raw := range stateDict {
		if len(name) > 10 && name[:10] == "optimizer." {
			// Optimizer state - remove prefix
			optimizerStateDict[name[10:]] = raw
		} else {
			// Model state
			modelStateDict[name] = raw
		}
	}

	// Load model state
	if err := model.LoadStateDict(modelStateDict); err != nil {
		return nil, fmt.Errorf("failed to load model state: %w", err)
	}

	// Load optimizer state
	if err := optimizer.LoadStateDict(optimizerStateDict); err != nil {
		return nil, fmt.Errorf("failed to load optimizer state: %w", err)
	}

	// Create checkpoint
	checkpoint = &Checkpoint[B]{
		Model:     model,
		Optimizer: optimizer,
		Epoch:     header.CheckpointMeta.Epoch,
		Step:      header.CheckpointMeta.Step,
		Loss:      header.CheckpointMeta.Loss,
		Metadata:  header.CheckpointMeta.TrainingMeta,
		CreatedAt: header.CreatedAt,
	}

	return checkpoint, nil
}

// SaveCheckpoint is a convenience function to save a checkpoint.
//
// This is equivalent to creating a Checkpoint struct and calling Save(),
// but with a simpler API for common use cases.
//
// Parameters:
//   - path: File path to write checkpoint to
//   - model: The model to save
//   - optimizer: The optimizer to save
//   - epoch: Current training epoch
//
// Returns an error if saving fails.
//
// Example:
//
//	err := nn.SaveCheckpoint("checkpoint.born", model, optimizer, epoch)
func SaveCheckpoint[B tensor.Backend](
	path string,
	model Module[B],
	optimizer OptimizerState,
	epoch int,
) error {
	checkpoint := &Checkpoint[B]{
		Model:     model,
		Optimizer: optimizer,
		Epoch:     epoch,
		Step:      0,
		Loss:      0.0,
		Metadata:  nil,
		CreatedAt: time.Now().UTC(),
	}
	return checkpoint.Save(path)
}

// getOptimizerType returns a string identifier for the optimizer type.
func getOptimizerType(_ OptimizerState) string {
	// We can't determine type without importing optim
	// So we'll just return a generic type
	return "Optimizer"
}

// getOptimizerConfig extracts optimizer configuration.
func getOptimizerConfig(opt OptimizerState) map[string]any {
	config := make(map[string]any)
	config["lr"] = opt.GetLR()
	return config
}
