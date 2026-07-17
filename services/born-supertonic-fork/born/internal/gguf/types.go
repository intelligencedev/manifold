// Package gguf provides GGUF file format parsing and model loading.
//
// GGUF (GGML Universal Format) is the file format used by llama.cpp
// for storing quantized LLM models. This package enables Born to load
// and use the 10,000+ pre-quantized models available on HuggingFace.
//
// Specification: https://github.com/ggerganov/ggml/blob/master/docs/gguf.md
package gguf

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Magic bytes for GGUF format.
const (
	MagicGGUFLE uint32 = 0x46554747 // "GGUF" little-endian.
	MagicGGUFBE uint32 = 0x47475546 // "GGUF" big-endian (reversed).
)

// Version constants.
const (
	Version1 uint32 = 1
	Version2 uint32 = 2
	Version3 uint32 = 3 // Current version.
)

// DefaultAlignment is the default alignment for tensor data.
const DefaultAlignment = 32

// ValueType represents the type of a metadata value.
type ValueType uint32

// Metadata value types as defined in GGUF specification.
const (
	ValueTypeUint8   ValueType = 0
	ValueTypeInt8    ValueType = 1
	ValueTypeUint16  ValueType = 2
	ValueTypeInt16   ValueType = 3
	ValueTypeUint32  ValueType = 4
	ValueTypeInt32   ValueType = 5
	ValueTypeFloat32 ValueType = 6
	ValueTypeBool    ValueType = 7
	ValueTypeString  ValueType = 8
	ValueTypeArray   ValueType = 9
	ValueTypeUint64  ValueType = 10
	ValueTypeInt64   ValueType = 11
	ValueTypeFloat64 ValueType = 12
)

// valueTypeNames maps ValueType constants to their string representations.
// These match the GGUF specification type names.
const (
	valueTypeNameUint8   = "uint8"
	valueTypeNameInt8    = "int8"
	valueTypeNameUint16  = "uint16"
	valueTypeNameInt16   = "int16"
	valueTypeNameUint32  = "uint32"
	valueTypeNameInt32   = "int32"
	valueTypeNameFloat32 = "float32"
	valueTypeNameBool    = "bool"
	valueTypeNameString  = "string"
	valueTypeNameArray   = "array"
	valueTypeNameUint64  = "uint64"
	valueTypeNameInt64   = "int64"
	valueTypeNameFloat64 = "float64"
)

// String returns the string representation of the value type.
func (t ValueType) String() string {
	names := map[ValueType]string{
		ValueTypeUint8:   valueTypeNameUint8,
		ValueTypeInt8:    valueTypeNameInt8,
		ValueTypeUint16:  valueTypeNameUint16,
		ValueTypeInt16:   valueTypeNameInt16,
		ValueTypeUint32:  valueTypeNameUint32,
		ValueTypeInt32:   valueTypeNameInt32,
		ValueTypeFloat32: valueTypeNameFloat32,
		ValueTypeBool:    valueTypeNameBool,
		ValueTypeString:  valueTypeNameString,
		ValueTypeArray:   valueTypeNameArray,
		ValueTypeUint64:  valueTypeNameUint64,
		ValueTypeInt64:   valueTypeNameInt64,
		ValueTypeFloat64: valueTypeNameFloat64,
	}
	if name, ok := names[t]; ok {
		return name
	}
	return fmt.Sprintf("unknown(%d)", t)
}

// GGMLType represents the data type of tensor elements.
type GGMLType uint32

// GGML tensor types (quantization formats).
// Note: Names use underscores to match GGML specification exactly (e.g., Q4_K, not Q4K).
//
//nolint:revive // Underscores in names match GGML specification.
const (
	GGMLTypeF32  GGMLType = 0
	GGMLTypeF16  GGMLType = 1
	GGMLTypeQ4_0 GGMLType = 2
	GGMLTypeQ4_1 GGMLType = 3
	// Types 4, 5 are deprecated (Q4_2, Q4_3).
	GGMLTypeQ5_0    GGMLType = 6
	GGMLTypeQ5_1    GGMLType = 7
	GGMLTypeQ8_0    GGMLType = 8
	GGMLTypeQ8_1    GGMLType = 9
	GGMLTypeQ2_K    GGMLType = 10
	GGMLTypeQ3_K    GGMLType = 11
	GGMLTypeQ4_K    GGMLType = 12
	GGMLTypeQ5_K    GGMLType = 13
	GGMLTypeQ6_K    GGMLType = 14
	GGMLTypeQ8_K    GGMLType = 15
	GGMLTypeIQ2_XXS GGMLType = 16
	GGMLTypeIQ2_XS  GGMLType = 17
	GGMLTypeIQ3_XXS GGMLType = 18
	GGMLTypeIQ1_S   GGMLType = 19
	GGMLTypeIQ4_NL  GGMLType = 20
	GGMLTypeIQ3_S   GGMLType = 21
	GGMLTypeIQ2_S   GGMLType = 22
	GGMLTypeIQ4_XS  GGMLType = 23
	GGMLTypeI8      GGMLType = 24
	GGMLTypeI16     GGMLType = 25
	GGMLTypeI32     GGMLType = 26
	GGMLTypeI64     GGMLType = 27
	GGMLTypeF64     GGMLType = 28
	GGMLTypeBF16    GGMLType = 29
)

// TypeTrait contains metadata about a GGML type.
type TypeTrait struct {
	BlockSize int // Number of elements per block.
	TypeSize  int // Size in bytes per block.
	Quantized bool
}

// typeTraits maps GGML types to their traits.
var typeTraits = map[GGMLType]TypeTrait{
	GGMLTypeF32:     {BlockSize: 1, TypeSize: 4, Quantized: false},
	GGMLTypeF16:     {BlockSize: 1, TypeSize: 2, Quantized: false},
	GGMLTypeQ4_0:    {BlockSize: 32, TypeSize: 18, Quantized: true},
	GGMLTypeQ4_1:    {BlockSize: 32, TypeSize: 20, Quantized: true},
	GGMLTypeQ5_0:    {BlockSize: 32, TypeSize: 22, Quantized: true},
	GGMLTypeQ5_1:    {BlockSize: 32, TypeSize: 24, Quantized: true},
	GGMLTypeQ8_0:    {BlockSize: 32, TypeSize: 34, Quantized: true},
	GGMLTypeQ8_1:    {BlockSize: 32, TypeSize: 36, Quantized: true},
	GGMLTypeQ2_K:    {BlockSize: 256, TypeSize: 84, Quantized: true},
	GGMLTypeQ3_K:    {BlockSize: 256, TypeSize: 110, Quantized: true},
	GGMLTypeQ4_K:    {BlockSize: 256, TypeSize: 144, Quantized: true},
	GGMLTypeQ5_K:    {BlockSize: 256, TypeSize: 176, Quantized: true},
	GGMLTypeQ6_K:    {BlockSize: 256, TypeSize: 210, Quantized: true},
	GGMLTypeQ8_K:    {BlockSize: 256, TypeSize: 292, Quantized: true},
	GGMLTypeIQ2_XXS: {BlockSize: 256, TypeSize: 66, Quantized: true},
	GGMLTypeIQ2_XS:  {BlockSize: 256, TypeSize: 74, Quantized: true},
	GGMLTypeIQ3_XXS: {BlockSize: 256, TypeSize: 98, Quantized: true},
	GGMLTypeIQ1_S:   {BlockSize: 256, TypeSize: 50, Quantized: true},
	GGMLTypeIQ4_NL:  {BlockSize: 32, TypeSize: 18, Quantized: true},
	GGMLTypeIQ3_S:   {BlockSize: 256, TypeSize: 110, Quantized: true},
	GGMLTypeIQ2_S:   {BlockSize: 256, TypeSize: 82, Quantized: true},
	GGMLTypeIQ4_XS:  {BlockSize: 256, TypeSize: 136, Quantized: true},
	GGMLTypeI8:      {BlockSize: 1, TypeSize: 1, Quantized: false},
	GGMLTypeI16:     {BlockSize: 1, TypeSize: 2, Quantized: false},
	GGMLTypeI32:     {BlockSize: 1, TypeSize: 4, Quantized: false},
	GGMLTypeI64:     {BlockSize: 1, TypeSize: 8, Quantized: false},
	GGMLTypeF64:     {BlockSize: 1, TypeSize: 8, Quantized: false},
	GGMLTypeBF16:    {BlockSize: 1, TypeSize: 2, Quantized: false},
}

// Trait returns the type trait for this GGML type.
func (t GGMLType) Trait() TypeTrait {
	if trait, ok := typeTraits[t]; ok {
		return trait
	}
	return TypeTrait{BlockSize: 1, TypeSize: 0, Quantized: false}
}

// IsQuantized returns true if the type is a quantized format.
func (t GGMLType) IsQuantized() bool {
	return t.Trait().Quantized
}

// ggmlTypeNames maps GGML types to their string names.
var ggmlTypeNames = map[GGMLType]string{
	GGMLTypeF32:     "F32",
	GGMLTypeF16:     "F16",
	GGMLTypeQ4_0:    "Q4_0",
	GGMLTypeQ4_1:    "Q4_1",
	GGMLTypeQ5_0:    "Q5_0",
	GGMLTypeQ5_1:    "Q5_1",
	GGMLTypeQ8_0:    "Q8_0",
	GGMLTypeQ8_1:    "Q8_1",
	GGMLTypeQ2_K:    "Q2_K",
	GGMLTypeQ3_K:    "Q3_K",
	GGMLTypeQ4_K:    "Q4_K",
	GGMLTypeQ5_K:    "Q5_K",
	GGMLTypeQ6_K:    "Q6_K",
	GGMLTypeQ8_K:    "Q8_K",
	GGMLTypeIQ2_XXS: "IQ2_XXS",
	GGMLTypeIQ2_XS:  "IQ2_XS",
	GGMLTypeIQ3_XXS: "IQ3_XXS",
	GGMLTypeIQ1_S:   "IQ1_S",
	GGMLTypeIQ4_NL:  "IQ4_NL",
	GGMLTypeIQ3_S:   "IQ3_S",
	GGMLTypeIQ2_S:   "IQ2_S",
	GGMLTypeIQ4_XS:  "IQ4_XS",
	GGMLTypeI8:      "I8",
	GGMLTypeI16:     "I16",
	GGMLTypeI32:     "I32",
	GGMLTypeI64:     "I64",
	GGMLTypeF64:     "F64",
	GGMLTypeBF16:    "BF16",
}

// String returns the string representation of the GGML type.
func (t GGMLType) String() string {
	if name, ok := ggmlTypeNames[t]; ok {
		return name
	}
	return fmt.Sprintf("unknown(%d)", t)
}

// RowSize calculates the size in bytes for a row of elements.
func (t GGMLType) RowSize(elements int) int {
	trait := t.Trait()
	if trait.BlockSize == 0 {
		return 0
	}
	numBlocks := (elements + trait.BlockSize - 1) / trait.BlockSize
	return numBlocks * trait.TypeSize
}

// Header represents the GGUF file header.
type Header struct {
	Magic           uint32
	Version         uint32
	TensorCount     uint64
	MetadataKVCount uint64
}

// TensorInfo contains metadata about a tensor in the file.
type TensorInfo struct {
	Name       string
	NDims      uint32
	Dimensions []uint64
	Type       GGMLType
	Offset     uint64 // Offset from start of tensor data section.
}

// NumElements returns the total number of elements in the tensor.
func (t *TensorInfo) NumElements() uint64 {
	if len(t.Dimensions) == 0 {
		return 0
	}
	n := uint64(1)
	for _, d := range t.Dimensions {
		n *= d
	}
	return n
}

// Size returns the size in bytes of the tensor data.
func (t *TensorInfo) Size() uint64 {
	elements := t.NumElements()
	//nolint:gosec // Elements won't exceed int range for practical tensors.
	return uint64(t.Type.RowSize(int(elements)))
}

// MetadataKV represents a key-value pair in the metadata.
type MetadataKV struct {
	Key       string
	ValueType ValueType
	Value     interface{}
}

// File represents a parsed GGUF file.
type File struct {
	Header     Header
	Metadata   map[string]interface{}
	TensorInfo []TensorInfo
	Alignment  int

	// Calculated offsets.
	TensorDataOffset int64

	// Source info.
	FilePath string
	FileSize int64
}

// Architecture returns the model architecture (e.g., "llama", "gpt2").
func (f *File) Architecture() string {
	if arch, ok := f.Metadata["general.architecture"].(string); ok {
		return arch
	}
	return ""
}

// Name returns the model name.
func (f *File) Name() string {
	if name, ok := f.Metadata["general.name"].(string); ok {
		return name
	}
	return ""
}

// getIntMetadata retrieves an integer metadata value by key.
func (f *File) getIntMetadata(key string) int {
	if v, ok := f.Metadata[key].(uint32); ok {
		return int(v)
	}
	if v, ok := f.Metadata[key].(uint64); ok {
		return int(v) //nolint:gosec // G115: integer overflow conversion uint64 -> int
	}
	return 0
}

// ContextLength returns the maximum context length.
func (f *File) ContextLength() int {
	return f.getIntMetadata(f.Architecture() + ".context_length")
}

// EmbeddingLength returns the embedding dimension.
func (f *File) EmbeddingLength() int {
	return f.getIntMetadata(f.Architecture() + ".embedding_length")
}

// BlockCount returns the number of transformer blocks.
func (f *File) BlockCount() int {
	return f.getIntMetadata(f.Architecture() + ".block_count")
}

// HeadCount returns the number of attention heads.
func (f *File) HeadCount() int {
	return f.getIntMetadata(f.Architecture() + ".attention.head_count")
}

// HeadCountKV returns the number of KV heads (for GQA).
func (f *File) HeadCountKV() int {
	kv := f.getIntMetadata(f.Architecture() + ".attention.head_count_kv")
	if kv == 0 {
		// Default to head_count if not specified.
		return f.HeadCount()
	}
	return kv
}

// FeedForwardLength returns the FFN intermediate size.
func (f *File) FeedForwardLength() int {
	return f.getIntMetadata(f.Architecture() + ".feed_forward_length")
}

// VocabSize returns the vocabulary size.
func (f *File) VocabSize() int {
	if tokens, ok := f.Metadata["tokenizer.ggml.tokens"].([]string); ok {
		return len(tokens)
	}
	return 0
}

// GetTensor finds a tensor by name.
func (f *File) GetTensor(name string) *TensorInfo {
	for i := range f.TensorInfo {
		if f.TensorInfo[i].Name == name {
			return &f.TensorInfo[i]
		}
	}
	return nil
}

// alignOffset calculates the aligned offset.
func alignOffset(offset int64, alignment int) int64 {
	if alignment <= 0 {
		alignment = DefaultAlignment
	}
	return offset + int64((alignment-int(offset%int64(alignment)))%alignment)
}

// readString reads a GGUF string (length-prefixed, NOT null-terminated).
func readString(r io.Reader, order binary.ByteOrder) (string, error) {
	var length uint64
	if err := binary.Read(r, order, &length); err != nil {
		return "", fmt.Errorf("read string length: %w", err)
	}

	// Sanity check: limit string length to 1MB.
	if length > 1<<20 {
		return "", fmt.Errorf("string too long: %d bytes", length)
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return "", fmt.Errorf("read string data: %w", err)
	}

	return string(data), nil
}
