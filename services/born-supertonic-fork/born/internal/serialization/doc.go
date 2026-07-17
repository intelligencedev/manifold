// Package serialization provides native .born format for saving and loading Born ML models.
//
// The .born format is a simple, efficient binary format designed specifically for Born models:
//
//	Format Structure:
//	  [4 bytes: Magic "BORN"]
//	  [4 bytes: Version (uint32 LE)]
//	  [4 bytes: Flags (uint32 LE)]
//	  [8 bytes: Header Size (uint64 LE)]
//	  [Header: JSON metadata]
//	  [Tensor data: raw bytes, 64-byte aligned]
//
// The format supports:
//   - Multiple data types (float32, float64, int32, int64, uint8, bool)
//   - Arbitrary tensor shapes
//   - Metadata preservation
//   - Fast loading with memory mapping support (future)
//   - Optional compression (future)
//
// Example usage:
//
//	// Save a model
//	model := nn.NewLinear(784, 128, backend)
//	writer := serialization.NewBornWriter("model.born")
//	if err := writer.WriteModel(model); err != nil {
//	    log.Fatal(err)
//	}
//	writer.Close()
//
//	// Load a model
//	reader := serialization.NewBornReader("model.born")
//	stateDict, err := reader.ReadStateDict(backend)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	model.LoadStateDict(stateDict)
//	reader.Close()
package serialization
