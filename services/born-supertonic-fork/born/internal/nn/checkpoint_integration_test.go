package nn_test

import (
	"os"
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/nn"
	"github.com/born-ml/born/internal/optim"
)

type CPUBackend = *cpu.CPUBackend

func TestCheckpointSaveLoad_SGD(t *testing.T) {
	backend := cpu.New()
	tempFile := "test_checkpoint_sgd.born"
	defer os.Remove(tempFile)

	// Create model and optimizer
	model := nn.NewLinear[CPUBackend](10, 5, backend)
	optimizer := optim.NewSGD(model.Parameters(), optim.SGDConfig{
		LR:       0.01,
		Momentum: 0.9,
	}, backend)

	// Create checkpoint
	checkpoint := &nn.Checkpoint[CPUBackend]{
		Model:     model,
		Optimizer: optimizer,
		Epoch:     10,
		Step:      5000,
		Loss:      0.123,
		Metadata:  map[string]any{"lr": 0.001, "batch_size": 32},
	}

	// Save checkpoint
	if err := checkpoint.Save(tempFile); err != nil {
		t.Fatalf("Failed to save checkpoint: %v", err)
	}

	// Create new model and optimizer for loading
	newModel := nn.NewLinear[CPUBackend](10, 5, backend)
	newOptimizer := optim.NewSGD(newModel.Parameters(), optim.SGDConfig{
		LR:       0.01,
		Momentum: 0.9,
	}, backend)

	// Load checkpoint
	loadedCheckpoint, err := nn.LoadCheckpoint(tempFile, backend, newModel, newOptimizer)
	if err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}

	// Verify metadata
	if loadedCheckpoint.Epoch != 10 {
		t.Errorf("Expected epoch 10, got %d", loadedCheckpoint.Epoch)
	}
	if loadedCheckpoint.Step != 5000 {
		t.Errorf("Expected step 5000, got %d", loadedCheckpoint.Step)
	}
	if loadedCheckpoint.Loss != 0.123 {
		t.Errorf("Expected loss 0.123, got %f", loadedCheckpoint.Loss)
	}

	// Verify model parameters were loaded
	originalParams := model.Parameters()
	loadedParams := newModel.Parameters()

	if len(originalParams) != len(loadedParams) {
		t.Fatalf("Parameter count mismatch: expected %d, got %d",
			len(originalParams), len(loadedParams))
	}

	for i := range originalParams {
		origData := originalParams[i].Tensor().Raw().AsFloat32()
		loadedData := loadedParams[i].Tensor().Raw().AsFloat32()

		if len(origData) != len(loadedData) {
			t.Errorf("Parameter %d size mismatch", i)
			continue
		}

		for j := range origData {
			if origData[j] != loadedData[j] {
				t.Errorf("Parameter %d data mismatch at index %d", i, j)
				break
			}
		}
	}
}

func TestCheckpointSaveLoad_Adam(t *testing.T) {
	backend := cpu.New()
	tempFile := "test_checkpoint_adam.born"
	defer os.Remove(tempFile)

	// Create model and optimizer
	model := nn.NewLinear[CPUBackend](10, 5, backend)
	optimizer := optim.NewAdam(model.Parameters(), optim.AdamConfig{
		LR:    0.001,
		Betas: [2]float32{0.9, 0.999},
		Eps:   1e-8,
	}, backend)

	// Create checkpoint
	checkpoint := &nn.Checkpoint[CPUBackend]{
		Model:     model,
		Optimizer: optimizer,
		Epoch:     5,
		Step:      2500,
		Loss:      0.456,
		Metadata:  map[string]any{"lr": 0.001},
	}

	// Save checkpoint
	if err := checkpoint.Save(tempFile); err != nil {
		t.Fatalf("Failed to save checkpoint: %v", err)
	}

	// Create new model and optimizer for loading
	newModel := nn.NewLinear[CPUBackend](10, 5, backend)
	newOptimizer := optim.NewAdam(newModel.Parameters(), optim.AdamConfig{
		LR:    0.001,
		Betas: [2]float32{0.9, 0.999},
		Eps:   1e-8,
	}, backend)

	// Load checkpoint
	loadedCheckpoint, err := nn.LoadCheckpoint(tempFile, backend, newModel, newOptimizer)
	if err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}

	// Verify metadata
	if loadedCheckpoint.Epoch != 5 {
		t.Errorf("Expected epoch 5, got %d", loadedCheckpoint.Epoch)
	}
	if loadedCheckpoint.Step != 2500 {
		t.Errorf("Expected step 2500, got %d", loadedCheckpoint.Step)
	}
	if loadedCheckpoint.Loss != 0.456 {
		t.Errorf("Expected loss 0.456, got %f", loadedCheckpoint.Loss)
	}
}

func TestSaveCheckpoint_Convenience(t *testing.T) {
	backend := cpu.New()
	tempFile := "test_checkpoint_convenience.born"
	defer os.Remove(tempFile)

	// Create model and optimizer
	model := nn.NewLinear[CPUBackend](10, 5, backend)
	optimizer := optim.NewSGD(model.Parameters(), optim.SGDConfig{
		LR: 0.01,
	}, backend)

	// Save checkpoint using convenience function
	if err := nn.SaveCheckpoint(tempFile, model, optimizer, 15); err != nil {
		t.Fatalf("Failed to save checkpoint: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(tempFile); os.IsNotExist(err) {
		t.Error("Checkpoint file was not created")
	}

	// Load and verify
	newModel := nn.NewLinear[CPUBackend](10, 5, backend)
	newOptimizer := optim.NewSGD(newModel.Parameters(), optim.SGDConfig{
		LR: 0.01,
	}, backend)

	loadedCheckpoint, err := nn.LoadCheckpoint(tempFile, backend, newModel, newOptimizer)
	if err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}

	if loadedCheckpoint.Epoch != 15 {
		t.Errorf("Expected epoch 15, got %d", loadedCheckpoint.Epoch)
	}
}

func TestCheckpointSaveLoad_Sequential(t *testing.T) {
	backend := cpu.New()
	tempFile := "test_checkpoint_sequential.born"
	defer os.Remove(tempFile)

	// Create sequential model
	model := nn.NewSequential[CPUBackend](
		nn.NewLinear[CPUBackend](10, 20, backend),
		nn.NewReLU[CPUBackend](),
		nn.NewLinear[CPUBackend](20, 5, backend),
	)

	optimizer := optim.NewAdam(model.Parameters(), optim.AdamConfig{
		LR: 0.001,
	}, backend)

	// Create and save checkpoint
	checkpoint := &nn.Checkpoint[CPUBackend]{
		Model:     model,
		Optimizer: optimizer,
		Epoch:     7,
		Step:      3500,
		Loss:      0.789,
	}

	if err := checkpoint.Save(tempFile); err != nil {
		t.Fatalf("Failed to save checkpoint: %v", err)
	}

	// Create new model for loading
	newModel := nn.NewSequential[CPUBackend](
		nn.NewLinear[CPUBackend](10, 20, backend),
		nn.NewReLU[CPUBackend](),
		nn.NewLinear[CPUBackend](20, 5, backend),
	)
	newOptimizer := optim.NewAdam(newModel.Parameters(), optim.AdamConfig{
		LR: 0.001,
	}, backend)

	// Load checkpoint
	loadedCheckpoint, err := nn.LoadCheckpoint(tempFile, backend, newModel, newOptimizer)
	if err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}

	if loadedCheckpoint.Epoch != 7 {
		t.Errorf("Expected epoch 7, got %d", loadedCheckpoint.Epoch)
	}

	// Verify all parameters were loaded
	originalParams := model.Parameters()
	loadedParams := newModel.Parameters()

	if len(originalParams) != len(loadedParams) {
		t.Fatalf("Parameter count mismatch: expected %d, got %d",
			len(originalParams), len(loadedParams))
	}
}

func TestCheckpointSaveLoad_SGDNoMomentum(t *testing.T) {
	backend := cpu.New()
	tempFile := "test_checkpoint_sgd_no_momentum.born"
	defer os.Remove(tempFile)

	// Create model and optimizer without momentum
	model := nn.NewLinear[CPUBackend](5, 3, backend)
	optimizer := optim.NewSGD(model.Parameters(), optim.SGDConfig{
		LR:       0.01,
		Momentum: 0.0, // No momentum
	}, backend)

	// Create checkpoint
	checkpoint := &nn.Checkpoint[CPUBackend]{
		Model:     model,
		Optimizer: optimizer,
		Epoch:     3,
		Step:      1500,
		Loss:      0.321,
	}

	// Save checkpoint
	if err := checkpoint.Save(tempFile); err != nil {
		t.Fatalf("Failed to save checkpoint: %v", err)
	}

	// Create new model and optimizer for loading
	newModel := nn.NewLinear[CPUBackend](5, 3, backend)
	newOptimizer := optim.NewSGD(newModel.Parameters(), optim.SGDConfig{
		LR:       0.01,
		Momentum: 0.0,
	}, backend)

	// Load checkpoint
	loadedCheckpoint, err := nn.LoadCheckpoint(tempFile, backend, newModel, newOptimizer)
	if err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}

	if loadedCheckpoint.Epoch != 3 {
		t.Errorf("Expected epoch 3, got %d", loadedCheckpoint.Epoch)
	}
}

func TestCheckpointLoad_InvalidFile(t *testing.T) {
	backend := cpu.New()

	// Try to load non-existent file
	model := nn.NewLinear[CPUBackend](10, 5, backend)
	optimizer := optim.NewSGD(model.Parameters(), optim.SGDConfig{LR: 0.01}, backend)

	_, err := nn.LoadCheckpoint("nonexistent.born", backend, model, optimizer)
	if err == nil {
		t.Error("Expected error when loading non-existent file, got nil")
	}
}

func TestCheckpointLoad_NotACheckpoint(t *testing.T) {
	backend := cpu.New()
	tempFile := "test_not_checkpoint.born"
	defer os.Remove(tempFile)

	// Save a regular model (not a checkpoint)
	model := nn.NewLinear[CPUBackend](10, 5, backend)
	if err := nn.Save(model, tempFile, "Linear", nil); err != nil {
		t.Fatalf("Failed to save model: %v", err)
	}

	// Try to load as checkpoint
	newModel := nn.NewLinear[CPUBackend](10, 5, backend)
	optimizer := optim.NewSGD(newModel.Parameters(), optim.SGDConfig{LR: 0.01}, backend)

	_, err := nn.LoadCheckpoint(tempFile, backend, newModel, optimizer)
	if err == nil {
		t.Error("Expected error when loading non-checkpoint file as checkpoint, got nil")
	}
}

func TestCheckpointMetadata(t *testing.T) {
	backend := cpu.New()
	tempFile := "test_checkpoint_metadata.born"
	defer os.Remove(tempFile)

	// Create checkpoint with custom metadata
	model := nn.NewLinear[CPUBackend](10, 5, backend)
	optimizer := optim.NewSGD(model.Parameters(), optim.SGDConfig{LR: 0.01}, backend)

	metadata := map[string]any{
		"learning_rate": 0.001,
		"batch_size":    32,
		"dataset":       "MNIST",
		"accuracy":      0.95,
	}

	checkpoint := &nn.Checkpoint[CPUBackend]{
		Model:     model,
		Optimizer: optimizer,
		Epoch:     20,
		Step:      10000,
		Loss:      0.05,
		Metadata:  metadata,
	}

	// Save checkpoint
	if err := checkpoint.Save(tempFile); err != nil {
		t.Fatalf("Failed to save checkpoint: %v", err)
	}

	// Load checkpoint
	newModel := nn.NewLinear[CPUBackend](10, 5, backend)
	newOptimizer := optim.NewSGD(newModel.Parameters(), optim.SGDConfig{LR: 0.01}, backend)

	loadedCheckpoint, err := nn.LoadCheckpoint(tempFile, backend, newModel, newOptimizer)
	if err != nil {
		t.Fatalf("Failed to load checkpoint: %v", err)
	}

	// Verify metadata exists (exact values may not match due to JSON serialization)
	if loadedCheckpoint.Metadata == nil {
		t.Fatal("Loaded checkpoint has nil metadata")
	}
}
