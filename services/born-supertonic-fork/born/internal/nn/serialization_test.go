package nn

import (
	"bytes"
	"os"
	"testing"

	"github.com/born-ml/born/backend/cpu"
	"github.com/born-ml/born/internal/serialization"
	"github.com/born-ml/born/internal/tensor"
)

// TestBornFormatRoundTrip tests save → load round-trip for a simple Linear module.
func TestBornFormatRoundTrip(t *testing.T) {
	backend := cpu.New()

	// Create a simple Linear layer
	model := NewLinear(784, 128, backend)

	// Get initial predictions
	input, err := tensor.FromSlice(make([]float32, 784), tensor.Shape{1, 784}, backend)
	if err != nil {
		t.Fatal(err)
	}
	pred1 := model.Forward(input)

	// Save model
	tmpFile := t.TempDir() + "/model.born"
	if err := Save(model, tmpFile, "Linear", nil); err != nil {
		t.Fatalf("Failed to save model: %v", err)
	}

	// Create new model with same architecture
	model2 := NewLinear(784, 128, backend)

	// Load into new model
	if _, err := Load(tmpFile, backend, model2); err != nil {
		t.Fatalf("Failed to load model: %v", err)
	}

	// Get predictions from loaded model
	pred2 := model2.Forward(input)

	// Predictions should be identical
	pred1Data := pred1.Data()
	pred2Data := pred2.Data()
	if len(pred1Data) != len(pred2Data) {
		t.Fatalf("Prediction length mismatch: %d != %d", len(pred1Data), len(pred2Data))
	}

	for i := range pred1Data {
		if pred1Data[i] != pred2Data[i] {
			t.Errorf("Prediction mismatch at index %d: %.6f != %.6f", i, pred1Data[i], pred2Data[i])
		}
	}
}

// TestBornFormatSequential tests save → load for a Sequential model.
func TestBornFormatSequential(t *testing.T) {
	backend := cpu.New()

	// Create a sequential model
	model := NewSequential(
		NewLinear(784, 128, backend),
		NewReLU[*cpu.Backend](),
		NewLinear(128, 10, backend),
	)

	// Get initial predictions
	input, err := tensor.FromSlice(make([]float32, 784), tensor.Shape{1, 784}, backend)
	if err != nil {
		t.Fatal(err)
	}
	pred1 := model.Forward(input)

	// Save model
	tmpFile := t.TempDir() + "/sequential.born"
	if err := Save(model, tmpFile, "Sequential", nil); err != nil {
		t.Fatalf("Failed to save model: %v", err)
	}

	// Create new model with same architecture
	model2 := NewSequential(
		NewLinear(784, 128, backend),
		NewReLU[*cpu.Backend](),
		NewLinear(128, 10, backend),
	)

	// Load into new model
	if _, err := Load(tmpFile, backend, model2); err != nil {
		t.Fatalf("Failed to load model: %v", err)
	}

	// Get predictions from loaded model
	pred2 := model2.Forward(input)

	// Predictions should be identical
	pred1Data := pred1.Data()
	pred2Data := pred2.Data()
	if len(pred1Data) != len(pred2Data) {
		t.Fatalf("Prediction length mismatch: %d != %d", len(pred1Data), len(pred2Data))
	}

	for i := range pred1Data {
		if pred1Data[i] != pred2Data[i] {
			t.Errorf("Prediction mismatch at index %d: %.6f != %.6f", i, pred1Data[i], pred2Data[i])
		}
	}
}

// TestBornFormatWithMetadata tests metadata preservation.
func TestBornFormatWithMetadata(t *testing.T) {
	backend := cpu.New()

	// Create a simple Linear layer
	model := NewLinear(10, 5, backend)

	// Save with metadata
	tmpFile := t.TempDir() + "/model_with_metadata.born"
	metadata := map[string]string{
		"version":     "1.0.0",
		"author":      "test",
		"description": "test model",
	}
	if err := Save(model, tmpFile, "Linear", metadata); err != nil {
		t.Fatalf("Failed to save model: %v", err)
	}

	// Read and verify metadata
	reader, err := serialization.NewBornReader(tmpFile)
	if err != nil {
		t.Fatalf("Failed to open reader: %v", err)
	}
	defer reader.Close()

	loadedMetadata := reader.Metadata()
	for key, expectedValue := range metadata {
		if actualValue, ok := loadedMetadata[key]; !ok {
			t.Errorf("Metadata key %s missing", key)
		} else if actualValue != expectedValue {
			t.Errorf("Metadata %s mismatch: expected %s, got %s", key, expectedValue, actualValue)
		}
	}
}

// TestBornFormatInvalidFile tests error handling for invalid files.
func TestBornFormatInvalidFile(t *testing.T) {
	tmpFile := t.TempDir() + "/invalid.born"

	// Write invalid magic bytes
	if err := os.WriteFile(tmpFile, []byte("XXXX"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Try to read - should fail
	if _, err := serialization.NewBornReader(tmpFile); err == nil {
		t.Error("Expected error for invalid magic bytes, got nil")
	}
}

// TestBornFormatMissingParameter tests error handling for missing parameters.
func TestBornFormatMissingParameter(t *testing.T) {
	backend := cpu.New()

	// Create a model and save it
	model := NewLinear(10, 5, backend)
	tmpFile := t.TempDir() + "/model.born"
	if err := Save(model, tmpFile, "Linear", nil); err != nil {
		t.Fatal(err)
	}

	// Read state dict
	reader, err := serialization.NewBornReader(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	stateDict, err := reader.ReadStateDict(backend)
	if err != nil {
		t.Fatal(err)
	}
	reader.Close()

	// Remove weight parameter
	delete(stateDict, "weight")

	// Try to load - should fail
	model2 := NewLinear(10, 5, backend)
	if err := model2.LoadStateDict(stateDict); err == nil {
		t.Error("Expected error for missing parameter, got nil")
	}
}

// TestBornFormatShapeMismatch tests error handling for shape mismatches.
func TestBornFormatShapeMismatch(t *testing.T) {
	backend := cpu.New()

	// Create and save a 10→5 model
	model := NewLinear(10, 5, backend)
	tmpFile := t.TempDir() + "/model.born"
	if err := Save(model, tmpFile, "Linear", nil); err != nil {
		t.Fatal(err)
	}

	// Try to load into a 20→5 model - should fail
	model2 := NewLinear(20, 5, backend)
	if _, err := Load(tmpFile, backend, model2); err == nil {
		t.Error("Expected error for shape mismatch, got nil")
	}
}

// TestBornWriterCloseIdempotent tests that closing writer multiple times is safe.
func TestBornWriterCloseIdempotent(t *testing.T) {
	tmpFile := t.TempDir() + "/close_test.born"
	writer, err := serialization.NewBornWriter(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	// Close multiple times should not panic
	if err := writer.Close(); err != nil {
		t.Errorf("First close failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Errorf("Second close failed: %v", err)
	}
}

// TestBornReaderCloseIdempotent tests that closing reader multiple times is safe.
func TestBornReaderCloseIdempotent(t *testing.T) {
	backend := cpu.New()
	model := NewLinear(10, 5, backend)
	tmpFile := t.TempDir() + "/close_test.born"
	if err := Save(model, tmpFile, "Linear", nil); err != nil {
		t.Fatal(err)
	}

	reader, err := serialization.NewBornReader(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	// Close multiple times should not panic
	if err := reader.Close(); err != nil {
		t.Errorf("First close failed: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Errorf("Second close failed: %v", err)
	}
}

// TestBornFormatTensorNames tests reading tensor names from file.
func TestBornFormatTensorNames(t *testing.T) {
	backend := cpu.New()

	// Create sequential model with known structure
	model := NewSequential(
		NewLinear(10, 5, backend), // 0.weight, 0.bias
		NewReLU[*cpu.Backend](),   // no parameters
		NewLinear(5, 3, backend),  // 2.weight, 2.bias
	)

	tmpFile := t.TempDir() + "/tensor_names.born"
	if err := Save(model, tmpFile, "Sequential", nil); err != nil {
		t.Fatal(err)
	}

	// Read and verify tensor names
	reader, err := serialization.NewBornReader(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	names := reader.TensorNames()
	expectedNames := []string{"0.weight", "0.bias", "2.weight", "2.bias"}

	if len(names) != len(expectedNames) {
		t.Fatalf("Expected %d tensor names, got %d", len(expectedNames), len(names))
	}

	// Check all expected names are present
	nameSet := make(map[string]bool)
	for _, name := range names {
		nameSet[name] = true
	}

	for _, expected := range expectedNames {
		if !nameSet[expected] {
			t.Errorf("Expected tensor name %s not found", expected)
		}
	}
}

// TestBornFormatHeaderInfo tests reading header information.
func TestBornFormatHeaderInfo(t *testing.T) {
	backend := cpu.New()
	model := NewLinear(10, 5, backend)

	tmpFile := t.TempDir() + "/header_test.born"
	metadata := map[string]string{"version": "1.0"}
	if err := Save(model, tmpFile, "Linear", metadata); err != nil {
		t.Fatal(err)
	}

	reader, err := serialization.NewBornReader(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	header := reader.Header()

	// Check format version
	if header.FormatVersion != serialization.FormatVersion {
		t.Errorf("Format version mismatch: expected %d, got %d", serialization.FormatVersion, header.FormatVersion)
	}

	// Check model type
	if header.ModelType != "Linear" {
		t.Errorf("Model type mismatch: expected Linear, got %s", header.ModelType)
	}

	// Check Born version
	if header.BornVersion == "" {
		t.Error("Born version is empty")
	}

	// Check created_at is set
	if header.CreatedAt.IsZero() {
		t.Error("CreatedAt timestamp is zero")
	}
}

// TestBornFormatWriteToReader tests serialization.WriteTo/serialization.ReadFrom functions.
func TestBornFormatWriteToReader(t *testing.T) {
	backend := cpu.New()

	// Create model
	model := NewLinear(10, 5, backend)
	stateDict := model.StateDict()

	// Write to buffer
	var buf bytes.Buffer
	if err := serialization.WriteTo(&buf, stateDict, "Linear", nil); err != nil {
		t.Fatalf("serialization.WriteTo failed: %v", err)
	}

	// Read from buffer
	loadedStateDict, header, err := serialization.ReadFrom(&buf, backend)
	if err != nil {
		t.Fatalf("serialization.ReadFrom failed: %v", err)
	}

	// Verify header
	if header.ModelType != "Linear" {
		t.Errorf("Model type mismatch: expected Linear, got %s", header.ModelType)
	}

	// Verify state dict
	if len(loadedStateDict) != len(stateDict) {
		t.Fatalf("StateDict length mismatch: expected %d, got %d", len(stateDict), len(loadedStateDict))
	}

	// Verify tensors match
	for name, originalRaw := range stateDict {
		loadedRaw, ok := loadedStateDict[name]
		if !ok {
			t.Errorf("Missing tensor %s in loaded state dict", name)
			continue
		}

		// Compare shapes
		if !originalRaw.Shape().Equal(loadedRaw.Shape()) {
			t.Errorf("Shape mismatch for %s: expected %v, got %v", name, originalRaw.Shape(), loadedRaw.Shape())
		}

		// Compare dtypes
		if originalRaw.DType() != loadedRaw.DType() {
			t.Errorf("DType mismatch for %s: expected %v, got %v", name, originalRaw.DType(), loadedRaw.DType())
		}

		// Compare data
		originalData := originalRaw.AsFloat32()
		loadedData := loadedRaw.AsFloat32()
		if len(originalData) != len(loadedData) {
			t.Errorf("Data length mismatch for %s: expected %d, got %d", name, len(originalData), len(loadedData))
			continue
		}

		for i := range originalData {
			if originalData[i] != loadedData[i] {
				t.Errorf("Data mismatch for %s at index %d: %.6f != %.6f", name, i, originalData[i], loadedData[i])
			}
		}
	}
}
