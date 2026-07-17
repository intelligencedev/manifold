package onnx

// ONNX protobuf data structures (hand-written).

// ModelProto represents an ONNX model.
type ModelProto struct {
	IRVersion       int64               // IR version (e.g., 7, 8, 9)
	OpsetImport     []OperatorSetID     // Opset version(s)
	ProducerName    string              // Framework name (e.g., "pytorch", "tf")
	ProducerVersion string              // Framework version
	Domain          string              // Model domain
	ModelVersion    int64               // Model version number
	DocString       string              // Model description
	Graph           *GraphProto         // Computation graph
	MetadataProps   []StringStringEntry // Key-value metadata
}

// GraphProto represents the computation graph.
type GraphProto struct {
	Name         string           // Graph name
	Nodes        []NodeProto      // Operation nodes
	Inputs       []ValueInfoProto // Graph inputs
	Outputs      []ValueInfoProto // Graph outputs
	Initializers []TensorProto    // Weight tensors
	DocString    string           // Graph description
	ValueInfo    []ValueInfoProto // Intermediate tensor info
}

// NodeProto represents a single operation.
type NodeProto struct {
	Name       string           // Node name (optional)
	OpType     string           // Operation type (e.g., "Conv", "MatMul", "Relu")
	Inputs     []string         // Input tensor names
	Outputs    []string         // Output tensor names
	Attributes []AttributeProto // Operation attributes
	Domain     string           // Custom domain (empty for default)
	DocString  string           // Node description
}

// TensorProto represents a tensor (weights/initializers).
type TensorProto struct {
	Name      string    // Tensor name
	DataType  int32     // Element data type
	Dims      []int64   // Tensor shape
	RawData   []byte    // Raw binary data (most common)
	FloatData []float32 // Float32 data (legacy)
	Int32Data []int32   // Int32 data (legacy)
	Int64Data []int64   // Int64 data (legacy)
	DocString string    // Tensor description
}

// ValueInfoProto describes input/output tensor specifications.
type ValueInfoProto struct {
	Name      string     // Tensor name
	Type      *TypeProto // Tensor type information
	DocString string     // Description
}

// TypeProto describes tensor type.
type TypeProto struct {
	TensorType *TensorTypeProto // Tensor type (most common)
}

// TensorTypeProto describes tensor shape and element type.
type TensorTypeProto struct {
	ElemType int32             // Element data type
	Shape    *TensorShapeProto // Tensor shape
}

// TensorShapeProto describes tensor dimensions.
type TensorShapeProto struct {
	Dims []DimensionProto // Dimensions
}

// DimensionProto describes a single dimension.
type DimensionProto struct {
	DimValue int64  // Static dimension value (e.g., 224 for image size)
	DimParam string // Dynamic dimension name (e.g., "batch_size")
}

// AttributeProto represents node attributes.
type AttributeProto struct {
	Name      string        // Attribute name
	Type      int32         // Attribute type
	F         float32       // FLOAT value
	I         int64         // INT value
	S         []byte        // STRING value
	T         *TensorProto  // TENSOR value
	G         *GraphProto   // GRAPH value
	Floats    []float32     // FLOATS array
	Ints      []int64       // INTS array
	Strings   [][]byte      // STRINGS array
	Tensors   []TensorProto // TENSORS array
	Graphs    []GraphProto  // GRAPHS array
	DocString string        // Description
}

// OperatorSetID identifies opset version.
type OperatorSetID struct {
	Domain  string // Operator domain (empty for default)
	Version int64  // Opset version number
}

// StringStringEntry represents key-value metadata.
type StringStringEntry struct {
	Key   string
	Value string
}

// ONNX data types (TensorProto.DataType).
const (
	TensorProtoUndefined  = 0
	TensorProtoFloat      = 1  // float32
	TensorProtoUint8      = 2  // uint8
	TensorProtoInt8       = 3  // int8
	TensorProtoUint16     = 4  // uint16
	TensorProtoInt16      = 5  // int16
	TensorProtoInt32      = 6  // int32
	TensorProtoInt64      = 7  // int64
	TensorProtoString     = 8  // string
	TensorProtoBool       = 9  // bool
	TensorProtoFloat16    = 10 // float16
	TensorProtoDouble     = 11 // float64
	TensorProtoUint32     = 12 // uint32
	TensorProtoUint64     = 13 // uint64
	TensorProtoComplex64  = 14 // complex64
	TensorProtoComplex128 = 15 // complex128
	TensorProtoBfloat16   = 16 // bfloat16
)

// ONNX attribute types (AttributeProto.Type).
const (
	AttributeProtoUndefined = 0
	AttributeProtoFloat     = 1  // FLOAT
	AttributeProtoInt       = 2  // INT
	AttributeProtoString    = 3  // STRING
	AttributeProtoTensor    = 4  // TENSOR
	AttributeProtoGraph     = 5  // GRAPH
	AttributeProtoFloats    = 6  // FLOATS
	AttributeProtoInts      = 7  // INTS
	AttributeProtoStrings   = 8  // STRINGS
	AttributeProtoTensors   = 9  // TENSORS
	AttributeProtoGraphs    = 10 // GRAPHS
)
