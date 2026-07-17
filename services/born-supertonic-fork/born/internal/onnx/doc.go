// Package onnx provides ONNX model import/export functionality.
//
// ONNX (Open Neural Network Exchange) is an open format for representing deep learning models.
// This package implements a hand-written protobuf parser for .onnx files without external dependencies.
//
// Key components:
//   - ModelProto: Top-level ONNX model structure with metadata and graph
//   - GraphProto: Computation graph with nodes, inputs, outputs, and initializers
//   - NodeProto: Single operation in the graph (e.g., Conv, MatMul, Relu)
//   - TensorProto: Weight/initializer tensor with data and shape
//   - ValueInfoProto: Input/output tensor type information
//
// Supported data types:
//   - float32, float64 (primary ML types)
//   - int8, int16, int32, int64 (integer types)
//   - uint8, uint16, uint32, uint64 (unsigned types)
//   - bool (boolean type)
//
// Example usage:
//
//	// Parse ONNX file
//	model, err := onnx.ParseFile("resnet50.onnx")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Inspect model
//	fmt.Printf("Model: %s (version %d)\n", model.ProducerName, model.ModelVersion)
//	fmt.Printf("Graph: %s with %d nodes\n", model.Graph.Name, len(model.Graph.Nodes))
//
//	// Iterate over operations
//	for _, node := range model.Graph.Nodes {
//	    fmt.Printf("Op: %s (type: %s)\n", node.Name, node.OpType)
//	}
package onnx
