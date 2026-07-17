//go:build !wasm

// Package operators implements ONNX operator mapping to Born tensor operations.
//
// The package provides a registry of operator handlers that convert ONNX nodes
// to equivalent Born operations. Each handler validates inputs and attributes,
// then delegates to the appropriate backend operation.
//
// Supported operator tiers:
//   - Tier 1 (Essential): Basic math, activations, convolutions - for ResNet, MobileNet
//   - Tier 2 (Extended): Advanced ops for transformers and attention models
package operators
