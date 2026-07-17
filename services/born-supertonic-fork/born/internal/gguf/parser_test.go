package gguf

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// createTestGGUF creates a minimal valid GGUF file in memory.
func createTestGGUF(t *testing.T) *bytes.Buffer {
	buf := new(bytes.Buffer)
	order := binary.LittleEndian

	// Write magic
	if err := binary.Write(buf, order, MagicGGUFLE); err != nil {
		t.Fatalf("write magic: %v", err)
	}

	// Write version (3)
	if err := binary.Write(buf, order, Version3); err != nil {
		t.Fatalf("write version: %v", err)
	}

	// Write tensor count (1)
	if err := binary.Write(buf, order, uint64(1)); err != nil {
		t.Fatalf("write tensor count: %v", err)
	}

	// Write metadata kv count (2)
	if err := binary.Write(buf, order, uint64(2)); err != nil {
		t.Fatalf("write metadata kv count: %v", err)
	}

	// Write metadata: general.architecture = "llama"
	writeString(buf, order, "general.architecture")
	if err := binary.Write(buf, order, uint32(ValueTypeString)); err != nil {
		t.Fatalf("write value type: %v", err)
	}
	writeString(buf, order, "llama")

	// Write metadata: llama.context_length = 4096
	writeString(buf, order, "llama.context_length")
	if err := binary.Write(buf, order, uint32(ValueTypeUint32)); err != nil {
		t.Fatalf("write value type: %v", err)
	}
	if err := binary.Write(buf, order, uint32(4096)); err != nil {
		t.Fatalf("write value: %v", err)
	}

	// Write tensor info: "test.weight" [16, 32] F32
	writeString(buf, order, "test.weight")
	if err := binary.Write(buf, order, uint32(2)); err != nil { // ndims
		t.Fatalf("write ndims: %v", err)
	}
	if err := binary.Write(buf, order, uint64(16)); err != nil { // dim 0
		t.Fatalf("write dim 0: %v", err)
	}
	if err := binary.Write(buf, order, uint64(32)); err != nil { // dim 1
		t.Fatalf("write dim 1: %v", err)
	}
	if err := binary.Write(buf, order, uint32(GGMLTypeF32)); err != nil { // type
		t.Fatalf("write type: %v", err)
	}
	if err := binary.Write(buf, order, uint64(0)); err != nil { // offset
		t.Fatalf("write offset: %v", err)
	}

	return buf
}

func writeString(buf *bytes.Buffer, order binary.ByteOrder, s string) {
	_ = binary.Write(buf, order, uint64(len(s)))
	_, _ = buf.WriteString(s)
}

func TestParseHeader(t *testing.T) {
	buf := createTestGGUF(t)

	file, err := Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if file.Header.Magic != MagicGGUFLE {
		t.Errorf("Magic = 0x%X, want 0x%X", file.Header.Magic, MagicGGUFLE)
	}

	if file.Header.Version != Version3 {
		t.Errorf("Version = %d, want %d", file.Header.Version, Version3)
	}

	if file.Header.TensorCount != 1 {
		t.Errorf("TensorCount = %d, want 1", file.Header.TensorCount)
	}

	if file.Header.MetadataKVCount != 2 {
		t.Errorf("MetadataKVCount = %d, want 2", file.Header.MetadataKVCount)
	}
}

func TestParseMetadata(t *testing.T) {
	buf := createTestGGUF(t)

	file, err := Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Check architecture
	arch, ok := file.Metadata["general.architecture"].(string)
	if !ok {
		t.Fatalf("general.architecture not found or wrong type")
	}
	if arch != "llama" {
		t.Errorf("Architecture = %q, want %q", arch, "llama")
	}

	// Check context length
	ctxLen, ok := file.Metadata["llama.context_length"].(uint32)
	if !ok {
		t.Fatalf("llama.context_length not found or wrong type")
	}
	if ctxLen != 4096 {
		t.Errorf("ContextLength = %d, want 4096", ctxLen)
	}
}

func TestParseTensorInfo(t *testing.T) {
	buf := createTestGGUF(t)

	file, err := Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(file.TensorInfo) != 1 {
		t.Fatalf("TensorInfo length = %d, want 1", len(file.TensorInfo))
	}

	ti := file.TensorInfo[0]

	if ti.Name != "test.weight" {
		t.Errorf("Name = %q, want %q", ti.Name, "test.weight")
	}

	if ti.NDims != 2 {
		t.Errorf("NDims = %d, want 2", ti.NDims)
	}

	if len(ti.Dimensions) != 2 {
		t.Fatalf("Dimensions length = %d, want 2", len(ti.Dimensions))
	}

	if ti.Dimensions[0] != 16 || ti.Dimensions[1] != 32 {
		t.Errorf("Dimensions = %v, want [16, 32]", ti.Dimensions)
	}

	if ti.Type != GGMLTypeF32 {
		t.Errorf("Type = %s, want F32", ti.Type)
	}
}

func TestFileHelpers(t *testing.T) {
	buf := createTestGGUF(t)

	file, err := Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if file.Architecture() != "llama" {
		t.Errorf("Architecture() = %q, want %q", file.Architecture(), "llama")
	}

	if file.ContextLength() != 4096 {
		t.Errorf("ContextLength() = %d, want 4096", file.ContextLength())
	}

	tensor := file.GetTensor("test.weight")
	if tensor == nil {
		t.Fatal("GetTensor(test.weight) = nil")
	}

	if tensor.NumElements() != 16*32 {
		t.Errorf("NumElements() = %d, want %d", tensor.NumElements(), 16*32)
	}

	// F32: 4 bytes per element, 16*32 = 512 elements = 2048 bytes
	if tensor.Size() != 2048 {
		t.Errorf("Size() = %d, want 2048", tensor.Size())
	}
}

func TestGGMLTypes(t *testing.T) {
	tests := []struct {
		typ       GGMLType
		name      string
		blockSize int
		typeSize  int
		quantized bool
	}{
		{GGMLTypeF32, "F32", 1, 4, false},
		{GGMLTypeF16, "F16", 1, 2, false},
		{GGMLTypeQ4_0, "Q4_0", 32, 18, true},
		{GGMLTypeQ4_K, "Q4_K", 256, 144, true},
		{GGMLTypeQ5_K, "Q5_K", 256, 176, true},
		{GGMLTypeQ6_K, "Q6_K", 256, 210, true},
		{GGMLTypeQ8_0, "Q8_0", 32, 34, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.typ.String() != tt.name {
				t.Errorf("String() = %q, want %q", tt.typ.String(), tt.name)
			}

			trait := tt.typ.Trait()
			if trait.BlockSize != tt.blockSize {
				t.Errorf("BlockSize = %d, want %d", trait.BlockSize, tt.blockSize)
			}
			if trait.TypeSize != tt.typeSize {
				t.Errorf("TypeSize = %d, want %d", trait.TypeSize, tt.typeSize)
			}
			if trait.Quantized != tt.quantized {
				t.Errorf("Quantized = %v, want %v", trait.Quantized, tt.quantized)
			}
			if tt.typ.IsQuantized() != tt.quantized {
				t.Errorf("IsQuantized() = %v, want %v", tt.typ.IsQuantized(), tt.quantized)
			}
		})
	}
}

func TestRowSize(t *testing.T) {
	// F32: 1 element = 4 bytes
	if GGMLTypeF32.RowSize(100) != 400 {
		t.Errorf("F32.RowSize(100) = %d, want 400", GGMLTypeF32.RowSize(100))
	}

	// Q4_K: 256 elements per block, 144 bytes per block
	// 256 elements = 1 block = 144 bytes
	if GGMLTypeQ4_K.RowSize(256) != 144 {
		t.Errorf("Q4_K.RowSize(256) = %d, want 144", GGMLTypeQ4_K.RowSize(256))
	}

	// 512 elements = 2 blocks = 288 bytes
	if GGMLTypeQ4_K.RowSize(512) != 288 {
		t.Errorf("Q4_K.RowSize(512) = %d, want 288", GGMLTypeQ4_K.RowSize(512))
	}

	// 257 elements = 2 blocks (rounded up) = 288 bytes
	if GGMLTypeQ4_K.RowSize(257) != 288 {
		t.Errorf("Q4_K.RowSize(257) = %d, want 288", GGMLTypeQ4_K.RowSize(257))
	}
}

func TestValueTypeString(t *testing.T) {
	tests := []struct {
		typ  ValueType
		want string
	}{
		{ValueTypeUint8, "uint8"},
		{ValueTypeInt32, "int32"},
		{ValueTypeFloat32, "float32"},
		{ValueTypeString, "string"},
		{ValueTypeArray, "array"},
		{ValueType(99), "unknown(99)"},
	}

	for _, tt := range tests {
		if got := tt.typ.String(); got != tt.want {
			t.Errorf("ValueType(%d).String() = %q, want %q", tt.typ, got, tt.want)
		}
	}
}

func TestInvalidMagic(t *testing.T) {
	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.LittleEndian, uint32(0xDEADBEEF))

	_, err := Parse(bytes.NewReader(buf.Bytes()))
	if err == nil {
		t.Error("Parse should fail with invalid magic")
	}
}

func TestInvalidVersion(t *testing.T) {
	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.LittleEndian, MagicGGUFLE)
	_ = binary.Write(buf, binary.LittleEndian, uint32(99)) // Invalid version

	_, err := Parse(bytes.NewReader(buf.Bytes()))
	if err == nil {
		t.Error("Parse should fail with invalid version")
	}
}

// createTestGGUFWithArray creates a GGUF file with array metadata.
func createTestGGUFWithArray(_ *testing.T) *bytes.Buffer {
	buf := new(bytes.Buffer)
	order := binary.LittleEndian

	// Header.
	_ = binary.Write(buf, order, MagicGGUFLE)
	_ = binary.Write(buf, order, Version3)
	_ = binary.Write(buf, order, uint64(0)) // no tensors
	_ = binary.Write(buf, order, uint64(1)) // 1 metadata

	// Metadata: tokenizer.ggml.tokens = ["<s>", "</s>", "hello"].
	writeString(buf, order, "tokenizer.ggml.tokens")
	_ = binary.Write(buf, order, uint32(ValueTypeArray))
	_ = binary.Write(buf, order, uint32(ValueTypeString)) // element type
	_ = binary.Write(buf, order, uint64(3))               // length
	writeString(buf, order, "<s>")
	writeString(buf, order, "</s>")
	writeString(buf, order, "hello")

	return buf
}

func TestParseArrayMetadata(t *testing.T) {
	buf := createTestGGUFWithArray(t)

	file, err := Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	tokens, ok := file.Metadata["tokenizer.ggml.tokens"].([]string)
	if !ok {
		t.Fatalf("tokenizer.ggml.tokens not found or wrong type: %T", file.Metadata["tokenizer.ggml.tokens"])
	}

	expected := []string{"<s>", "</s>", "hello"}
	if len(tokens) != len(expected) {
		t.Fatalf("tokens length = %d, want %d", len(tokens), len(expected))
	}

	for i, tok := range tokens {
		if tok != expected[i] {
			t.Errorf("tokens[%d] = %q, want %q", i, tok, expected[i])
		}
	}
}

func TestAlignOffset(t *testing.T) {
	tests := []struct {
		offset    int64
		alignment int
		want      int64
	}{
		{0, 32, 0},
		{1, 32, 32},
		{31, 32, 32},
		{32, 32, 32},
		{33, 32, 64},
		{100, 32, 128},
		{128, 32, 128},
		{0, 0, 0},  // Test default alignment
		{1, 0, 32}, // Test default alignment (should use 32)
	}

	for _, tt := range tests {
		got := alignOffset(tt.offset, tt.alignment)
		if got != tt.want {
			t.Errorf("alignOffset(%d, %d) = %d, want %d", tt.offset, tt.alignment, got, tt.want)
		}
	}
}

// TestNumericArrays tests parsing of various numeric array types.
func TestNumericArrays(t *testing.T) {
	buf := new(bytes.Buffer)
	order := binary.LittleEndian

	// Header.
	_ = binary.Write(buf, order, MagicGGUFLE)
	_ = binary.Write(buf, order, Version3)
	_ = binary.Write(buf, order, uint64(0)) // no tensors
	_ = binary.Write(buf, order, uint64(1)) // 1 metadata

	// Metadata: test.uint32_array = [1, 2, 3].
	writeString(buf, order, "test.uint32_array")
	_ = binary.Write(buf, order, uint32(ValueTypeArray))
	_ = binary.Write(buf, order, uint32(ValueTypeUint32)) // element type
	_ = binary.Write(buf, order, uint64(3))               // length
	_ = binary.Write(buf, order, uint32(1))
	_ = binary.Write(buf, order, uint32(2))
	_ = binary.Write(buf, order, uint32(3))

	file, err := Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	arr, ok := file.Metadata["test.uint32_array"].([]uint32)
	if !ok {
		t.Fatalf("uint32 array not found or wrong type: %T", file.Metadata["test.uint32_array"])
	}

	expected := []uint32{1, 2, 3}
	if len(arr) != len(expected) {
		t.Fatalf("array length = %d, want %d", len(arr), len(expected))
	}
	for i, v := range arr {
		if v != expected[i] {
			t.Errorf("arr[%d] = %d, want %d", i, v, expected[i])
		}
	}
}

// TestFloat32Array tests parsing of float32 arrays.
func TestFloat32Array(t *testing.T) {
	buf := new(bytes.Buffer)
	order := binary.LittleEndian

	// Header.
	_ = binary.Write(buf, order, MagicGGUFLE)
	_ = binary.Write(buf, order, Version3)
	_ = binary.Write(buf, order, uint64(0)) // no tensors
	_ = binary.Write(buf, order, uint64(1)) // 1 metadata

	// Metadata: test.floats = [1.5, 2.5].
	writeString(buf, order, "test.floats")
	_ = binary.Write(buf, order, uint32(ValueTypeArray))
	_ = binary.Write(buf, order, uint32(ValueTypeFloat32)) // element type
	_ = binary.Write(buf, order, uint64(2))                // length
	_ = binary.Write(buf, order, float32(1.5))
	_ = binary.Write(buf, order, float32(2.5))

	file, err := Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	arr, ok := file.Metadata["test.floats"].([]float32)
	if !ok {
		t.Fatalf("float32 array not found or wrong type: %T", file.Metadata["test.floats"])
	}

	if len(arr) != 2 || arr[0] != 1.5 || arr[1] != 2.5 {
		t.Errorf("array = %v, want [1.5, 2.5]", arr)
	}
}

// TestAllNumericValueTypes tests all numeric value types.
func TestAllNumericValueTypes(t *testing.T) {
	tests := []struct {
		name      string
		valueType ValueType
		writeVal  func(*bytes.Buffer, binary.ByteOrder)
		checkVal  func(interface{}) bool
	}{
		{
			"uint8",
			ValueTypeUint8,
			func(buf *bytes.Buffer, order binary.ByteOrder) { _ = binary.Write(buf, order, uint8(42)) },
			func(v interface{}) bool { return v.(uint8) == 42 },
		},
		{
			"int8",
			ValueTypeInt8,
			func(buf *bytes.Buffer, order binary.ByteOrder) { _ = binary.Write(buf, order, int8(-10)) },
			func(v interface{}) bool { return v.(int8) == -10 },
		},
		{
			"uint16",
			ValueTypeUint16,
			func(buf *bytes.Buffer, order binary.ByteOrder) { _ = binary.Write(buf, order, uint16(1000)) },
			func(v interface{}) bool { return v.(uint16) == 1000 },
		},
		{
			"int16",
			ValueTypeInt16,
			func(buf *bytes.Buffer, order binary.ByteOrder) { _ = binary.Write(buf, order, int16(-500)) },
			func(v interface{}) bool { return v.(int16) == -500 },
		},
		{
			"int32",
			ValueTypeInt32,
			func(buf *bytes.Buffer, order binary.ByteOrder) { _ = binary.Write(buf, order, int32(-100000)) },
			func(v interface{}) bool { return v.(int32) == -100000 },
		},
		{
			"float32",
			ValueTypeFloat32,
			func(buf *bytes.Buffer, order binary.ByteOrder) { _ = binary.Write(buf, order, float32(3.14)) },
			func(v interface{}) bool { return v.(float32) > 3.13 && v.(float32) < 3.15 },
		},
		{
			"uint64",
			ValueTypeUint64,
			func(buf *bytes.Buffer, order binary.ByteOrder) { _ = binary.Write(buf, order, uint64(1<<40)) },
			func(v interface{}) bool { return v.(uint64) == 1<<40 },
		},
		{
			"int64",
			ValueTypeInt64,
			func(buf *bytes.Buffer, order binary.ByteOrder) { _ = binary.Write(buf, order, int64(-1<<40)) },
			func(v interface{}) bool { return v.(int64) == -1<<40 },
		},
		{
			"float64",
			ValueTypeFloat64,
			func(buf *bytes.Buffer, order binary.ByteOrder) { _ = binary.Write(buf, order, float64(2.718281828)) },
			func(v interface{}) bool { return v.(float64) > 2.71 && v.(float64) < 2.72 },
		},
		{
			"bool_true",
			ValueTypeBool,
			func(buf *bytes.Buffer, order binary.ByteOrder) { _ = binary.Write(buf, order, uint8(1)) },
			func(v interface{}) bool { return v.(bool) == true },
		},
		{
			"bool_false",
			ValueTypeBool,
			func(buf *bytes.Buffer, order binary.ByteOrder) { _ = binary.Write(buf, order, uint8(0)) },
			func(v interface{}) bool { return v.(bool) == false },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := new(bytes.Buffer)
			order := binary.LittleEndian

			// Header.
			_ = binary.Write(buf, order, MagicGGUFLE)
			_ = binary.Write(buf, order, Version3)
			_ = binary.Write(buf, order, uint64(0)) // no tensors
			_ = binary.Write(buf, order, uint64(1)) // 1 metadata

			// Metadata: test.value = <value>.
			writeString(buf, order, "test.value")
			_ = binary.Write(buf, order, uint32(tt.valueType))
			tt.writeVal(buf, order)

			file, err := Parse(bytes.NewReader(buf.Bytes()))
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			v, ok := file.Metadata["test.value"]
			if !ok {
				t.Fatalf("value not found")
			}
			if !tt.checkVal(v) {
				t.Errorf("unexpected value: %v (type %T)", v, v)
			}
		})
	}
}

// TestUnknownGGMLType tests behavior with unknown GGML type.
func TestUnknownGGMLType(t *testing.T) {
	unknownType := GGMLType(255)
	trait := unknownType.Trait()
	if trait.BlockSize != 1 || trait.TypeSize != 0 || trait.Quantized != false {
		t.Errorf("unexpected trait for unknown type: %+v", trait)
	}

	name := unknownType.String()
	if name != "unknown(255)" {
		t.Errorf("String() = %q, want %q", name, "unknown(255)")
	}
}

// TestFileMetadataHelpers tests additional File helper methods.
func TestFileMetadataHelpers(t *testing.T) {
	buf := new(bytes.Buffer)
	order := binary.LittleEndian

	// Header.
	_ = binary.Write(buf, order, MagicGGUFLE)
	_ = binary.Write(buf, order, Version3)
	_ = binary.Write(buf, order, uint64(0)) // no tensors
	_ = binary.Write(buf, order, uint64(7)) // 7 metadata entries

	// general.architecture = "test".
	writeString(buf, order, "general.architecture")
	_ = binary.Write(buf, order, uint32(ValueTypeString))
	writeString(buf, order, "test")

	// general.name = "TestModel".
	writeString(buf, order, "general.name")
	_ = binary.Write(buf, order, uint32(ValueTypeString))
	writeString(buf, order, "TestModel")

	// test.embedding_length = 512.
	writeString(buf, order, "test.embedding_length")
	_ = binary.Write(buf, order, uint32(ValueTypeUint32))
	_ = binary.Write(buf, order, uint32(512))

	// test.block_count = 12.
	writeString(buf, order, "test.block_count")
	_ = binary.Write(buf, order, uint32(ValueTypeUint32))
	_ = binary.Write(buf, order, uint32(12))

	// test.attention.head_count = 8.
	writeString(buf, order, "test.attention.head_count")
	_ = binary.Write(buf, order, uint32(ValueTypeUint32))
	_ = binary.Write(buf, order, uint32(8))

	// test.attention.head_count_kv = 4.
	writeString(buf, order, "test.attention.head_count_kv")
	_ = binary.Write(buf, order, uint32(ValueTypeUint32))
	_ = binary.Write(buf, order, uint32(4))

	// test.feed_forward_length = 2048.
	writeString(buf, order, "test.feed_forward_length")
	_ = binary.Write(buf, order, uint32(ValueTypeUint32))
	_ = binary.Write(buf, order, uint32(2048))

	file, err := Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Test Name().
	if name := file.Name(); name != "TestModel" {
		t.Errorf("Name() = %q, want %q", name, "TestModel")
	}

	// Test EmbeddingLength().
	if emb := file.EmbeddingLength(); emb != 512 {
		t.Errorf("EmbeddingLength() = %d, want 512", emb)
	}

	// Test BlockCount().
	if bc := file.BlockCount(); bc != 12 {
		t.Errorf("BlockCount() = %d, want 12", bc)
	}

	// Test HeadCount().
	if hc := file.HeadCount(); hc != 8 {
		t.Errorf("HeadCount() = %d, want 8", hc)
	}

	// Test HeadCountKV().
	if hckv := file.HeadCountKV(); hckv != 4 {
		t.Errorf("HeadCountKV() = %d, want 4", hckv)
	}

	// Test FeedForwardLength().
	if ff := file.FeedForwardLength(); ff != 2048 {
		t.Errorf("FeedForwardLength() = %d, want 2048", ff)
	}
}

// TestVocabSize tests vocabulary size extraction.
func TestVocabSize(t *testing.T) {
	buf := createTestGGUFWithArray(t)

	file, err := Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Should return 3 because createTestGGUFWithArray creates ["<s>", "</s>", "hello"].
	if vs := file.VocabSize(); vs != 3 {
		t.Errorf("VocabSize() = %d, want 3", vs)
	}
}

// TestGetTensorNotFound tests GetTensor with non-existent tensor.
func TestGetTensorNotFound(t *testing.T) {
	buf := createTestGGUF(t)

	file, err := Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	tensor := file.GetTensor("nonexistent")
	if tensor != nil {
		t.Errorf("GetTensor(nonexistent) should return nil")
	}
}

// TestHeadCountKVDefault tests HeadCountKV when not specified.
func TestHeadCountKVDefault(t *testing.T) {
	buf := new(bytes.Buffer)
	order := binary.LittleEndian

	// Header.
	_ = binary.Write(buf, order, MagicGGUFLE)
	_ = binary.Write(buf, order, Version3)
	_ = binary.Write(buf, order, uint64(0)) // no tensors
	_ = binary.Write(buf, order, uint64(2)) // 2 metadata entries

	// general.architecture = "test".
	writeString(buf, order, "general.architecture")
	_ = binary.Write(buf, order, uint32(ValueTypeString))
	writeString(buf, order, "test")

	// test.attention.head_count = 16 (no head_count_kv).
	writeString(buf, order, "test.attention.head_count")
	_ = binary.Write(buf, order, uint32(ValueTypeUint32))
	_ = binary.Write(buf, order, uint32(16))

	file, err := Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// HeadCountKV should default to HeadCount when not specified.
	if hckv := file.HeadCountKV(); hckv != 16 {
		t.Errorf("HeadCountKV() = %d, want 16 (default to HeadCount)", hckv)
	}
}
