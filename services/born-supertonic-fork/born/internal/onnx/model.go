//go:build !wasm

package onnx

import (
	"fmt"
	"os"

	"github.com/born-ml/born/internal/onnx/operators"
	"github.com/born-ml/born/internal/tensor"
)

var bornTrace = os.Getenv("BORN_TRACE") != ""

// Model represents a loaded ONNX model ready for inference.
// It executes the computation graph using the provided backend.
type Model struct {
	proto        *ModelProto
	registry     *operators.Registry
	backend      tensor.Backend
	tensors      map[string]*tensor.RawTensor // Weights and intermediate tensors
	inputNames   []string
	outputNames  []string
	sortedNodes  []NodeProto
	opsetVersion int64
}

// InputNames returns the names of model inputs.
func (m *Model) InputNames() []string {
	return m.inputNames
}

// OutputNames returns the names of model outputs.
func (m *Model) OutputNames() []string {
	return m.outputNames
}

// OpsetVersion returns the ONNX opset version.
func (m *Model) OpsetVersion() int64 {
	return m.opsetVersion
}

// Metadata returns model metadata as key-value pairs.
func (m *Model) Metadata() map[string]string {
	meta := make(map[string]string)
	for _, prop := range m.proto.MetadataProps {
		meta[prop.Key] = prop.Value
	}
	meta["producer_name"] = m.proto.ProducerName
	meta["producer_version"] = m.proto.ProducerVersion
	meta["domain"] = m.proto.Domain
	return meta
}

// Forward runs inference with a single input tensor.
// For models with multiple inputs, use ForwardNamed.
func (m *Model) Forward(input *tensor.RawTensor) (*tensor.RawTensor, error) {
	if len(m.inputNames) != 1 {
		return nil, fmt.Errorf("model has %d inputs, use ForwardNamed", len(m.inputNames))
	}

	outputs, err := m.ForwardNamed(map[string]*tensor.RawTensor{
		m.inputNames[0]: input,
	})
	if err != nil {
		return nil, err
	}

	if len(m.outputNames) != 1 {
		return nil, fmt.Errorf("model has %d outputs, access via ForwardNamed result", len(m.outputNames))
	}

	return outputs[m.outputNames[0]], nil
}

// ForwardNamed runs inference with named inputs.
// Returns a map of output name to tensor.
//
//nolint:gocognit // ForwardNamed orchestrates the full inference pipeline.
func (m *Model) ForwardNamed(inputs map[string]*tensor.RawTensor) (map[string]*tensor.RawTensor, error) {
	// Copy weights and set inputs.
	//
	// put() protects every tensor placed in the map from in-place mutation:
	// operators use an inplace fast path when a buffer looks unique (refCount==1),
	// but a graph tensor held only by this map is *not* safe to mutate — later
	// nodes still read it. Bumping the refcount forces those ops to copy instead,
	// preventing buffer-aliasing corruption (e.g. an op overwriting a value that a
	// subsequent Unsqueeze/Slice still needs). The extra ref is never released:
	// buffers are Go-GC'd, so this only disables the inplace optimization.
	tensors := make(map[string]*tensor.RawTensor)
	put := func(name string, t *tensor.RawTensor) {
		if t != nil {
			t.ForceNonUnique()
		}
		tensors[name] = t
	}
	for name, t := range m.tensors {
		put(name, t)
	}
	for name, t := range inputs {
		put(name, t)
	}

	// Validate all inputs are provided
	for _, inputName := range m.inputNames {
		if _, ok := tensors[inputName]; !ok {
			return nil, fmt.Errorf("missing input: %s", inputName)
		}
	}

	// Execute nodes in topological order
	ctx := &operators.Context{Backend: m.backend}
	for nodeIdx := range m.sortedNodes {
		node := &m.sortedNodes[nodeIdx]
		// Gather inputs
		nodeInputs := make([]*tensor.RawTensor, len(node.Inputs))
		for i, inputName := range node.Inputs {
			if inputName == "" {
				// Optional input not provided
				nodeInputs[i] = nil
				continue
			}
			t, ok := tensors[inputName]
			if !ok {
				return nil, fmt.Errorf("node %s: missing input %s", node.Name, inputName)
			}
			nodeInputs[i] = t
		}

		// Execute operator
		opNode, err := nodeProtoToOperatorNode(node)
		if err != nil {
			return nil, fmt.Errorf("node %s (%s): %w", node.Name, node.OpType, err)
		}
		outputs, err := m.registry.Execute(ctx, opNode, nodeInputs)
		if err != nil {
			return nil, fmt.Errorf("node %s (%s): %w", node.Name, node.OpType, err)
		}

		// Store outputs (protected from later in-place mutation; see put()).
		for i, outputName := range node.Outputs {
			if i < len(outputs) {
				put(outputName, outputs[i])
			}
		}

		if bornTrace {
			for i, outputName := range node.Outputs {
				if i < len(outputs) && outputs[i] != nil {
					o := outputs[i]
					val := ""
					if o.DType() == tensor.Int64 && o.NumElements() <= 64 {
						val = fmt.Sprintf("%v", o.AsInt64())
					}
					fmt.Printf("TRACE %s\t%s\t%v\t%s\n", node.OpType, outputName, o.Shape(), val)
				}
			}
		}
	}

	// Gather final outputs
	result := make(map[string]*tensor.RawTensor)
	for _, outputName := range m.outputNames {
		t, ok := tensors[outputName]
		if !ok {
			return nil, fmt.Errorf("missing output: %s", outputName)
		}
		result[outputName] = t
	}

	return result, nil
}

// compile prepares the model for inference.
func (m *Model) compile() error {
	graph := m.proto.Graph
	if graph == nil {
		return fmt.Errorf("model has no graph")
	}

	// Load initializers (weights)
	m.tensors = make(map[string]*tensor.RawTensor)
	for i := range graph.Initializers {
		init := &graph.Initializers[i]
		t, err := tensorFromProto(init)
		if err != nil {
			return fmt.Errorf("failed to load initializer %s: %w", init.Name, err)
		}
		m.tensors[init.Name] = t
	}

	// Extract input/output names
	initNames := make(map[string]bool)
	for i := range graph.Initializers {
		initNames[graph.Initializers[i].Name] = true
	}

	// Inputs are graph inputs minus initializers
	for i := range graph.Inputs {
		if !initNames[graph.Inputs[i].Name] {
			m.inputNames = append(m.inputNames, graph.Inputs[i].Name)
		}
	}

	for i := range graph.Outputs {
		m.outputNames = append(m.outputNames, graph.Outputs[i].Name)
	}

	// Topological sort of nodes
	m.sortedNodes = topologicalSort(graph.Nodes)

	// Get opset version
	for _, opset := range m.proto.OpsetImport {
		if opset.Domain == "" || opset.Domain == "ai.onnx" {
			m.opsetVersion = opset.Version
			break
		}
	}

	return nil
}

// tensorFromProto converts TensorProto to RawTensor.
func tensorFromProto(proto *TensorProto) (*tensor.RawTensor, error) {
	// Get shape
	shape := make(tensor.Shape, len(proto.Dims))
	for i, dim := range proto.Dims {
		shape[i] = int(dim)
	}

	// Get dtype
	dtype := protoTypeToTensorType(proto.DataType)

	// Create tensor
	t, err := tensor.NewRaw(shape, dtype, tensor.CPU)
	if err != nil {
		return nil, err
	}

	// Copy data - check which data field is populated (mutually exclusive).
	//nolint:gocritic // ifElseChain: checking mutually exclusive data fields.
	if len(proto.RawData) > 0 {
		// Raw binary data - most common
		copy(t.Data(), proto.RawData)
	} else if len(proto.FloatData) > 0 {
		// Legacy float data
		dst := t.AsFloat32()
		copy(dst, proto.FloatData)
	} else if len(proto.Int32Data) > 0 {
		// Legacy int32 data
		dst := t.AsInt32()
		copy(dst, proto.Int32Data)
	} else if len(proto.Int64Data) > 0 {
		// Legacy int64 data
		dst := t.AsInt64()
		copy(dst, proto.Int64Data)
	}

	return t, nil
}

// protoTypeToTensorType converts ONNX data type to tensor.DataType.
func protoTypeToTensorType(onnxType int32) tensor.DataType {
	switch onnxType {
	case TensorProtoFloat:
		return tensor.Float32
	case TensorProtoDouble:
		return tensor.Float64
	case TensorProtoInt32:
		return tensor.Int32
	case TensorProtoInt64:
		return tensor.Int64
	case TensorProtoUint8:
		return tensor.Uint8
	case TensorProtoBool:
		return tensor.Bool
	default:
		return tensor.Float32 // Default fallback
	}
}

// nodeProtoToOperatorNode converts NodeProto to operators.Node.
func nodeProtoToOperatorNode(proto *NodeProto) (*operators.Node, error) {
	attrs := make([]operators.Attribute, len(proto.Attributes))
	for i := range proto.Attributes {
		attr := &proto.Attributes[i]
		attrs[i] = operators.Attribute{
			Name:    attr.Name,
			Type:    attr.Type,
			F:       attr.F,
			I:       attr.I,
			S:       attr.S,
			Floats:  attr.Floats,
			Ints:    attr.Ints,
			Strings: attr.Strings,
		}
		if attr.T != nil {
			var err error
			attrs[i].T, err = tensorFromProto(attr.T)
			if err != nil {
				return nil, err
			}
		}
	}
	return &operators.Node{
		Name:       proto.Name,
		OpType:     proto.OpType,
		Inputs:     proto.Inputs,
		Outputs:    proto.Outputs,
		Attributes: attrs,
		Domain:     proto.Domain,
	}, nil
}

// topologicalSort sorts nodes in execution order.
// Ensures dependencies are executed before dependents.
func topologicalSort(nodes []NodeProto) []NodeProto {
	// Build output-to-node map
	outputToNode := make(map[string]int)
	for i := range nodes {
		for _, output := range nodes[i].Outputs {
			outputToNode[output] = i
		}
	}

	// Track visited and result
	visited := make([]bool, len(nodes))
	result := make([]NodeProto, 0, len(nodes))

	var visit func(i int)
	visit = func(i int) {
		if visited[i] {
			return
		}
		visited[i] = true

		// Visit dependencies first
		for _, input := range nodes[i].Inputs {
			if depIdx, ok := outputToNode[input]; ok {
				visit(depIdx)
			}
		}

		result = append(result, nodes[i])
	}

	// Visit all nodes
	for i := range nodes {
		visit(i)
	}

	return result
}
