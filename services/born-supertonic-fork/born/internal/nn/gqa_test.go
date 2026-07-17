package nn

import (
	"testing"

	"github.com/born-ml/born/internal/autodiff"
	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/tensor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGQA_Basic(t *testing.T) {
	backend := autodiff.New(cpu.New())

	cfg := GQAConfig{
		EmbedDim: 256,
		NQHeads:  8,
		NKVHeads: 2, // 4:1 ratio
		HeadDim:  32,
	}
	gqa := NewGQA(cfg, backend)

	assert.NotNil(t, gqa)
	assert.NotNil(t, gqa.QProj)
	assert.NotNil(t, gqa.KProj)
	assert.NotNil(t, gqa.VProj)
	assert.NotNil(t, gqa.OutProj)
	assert.Nil(t, gqa.rope) // RoPE not enabled
	assert.Equal(t, 8, gqa.config.NQHeads)
	assert.Equal(t, 2, gqa.config.NKVHeads)
}

func TestNewGQA_WithRoPE(t *testing.T) {
	backend := autodiff.New(cpu.New())

	cfg := GQAConfig{
		EmbedDim:  256,
		NQHeads:   8,
		NKVHeads:  2,
		HeadDim:   32,
		UseRoPE:   true,
		MaxSeqLen: 512,
	}
	gqa := NewGQA(cfg, backend)

	assert.NotNil(t, gqa)
	assert.NotNil(t, gqa.rope)
	assert.Equal(t, 512, gqa.rope.MaxSeqLen)
}

func TestNewGQA_AutoHeadDim(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// HeadDim should be computed automatically
	cfg := GQAConfig{
		EmbedDim: 256,
		NQHeads:  8,
		NKVHeads: 2,
		// HeadDim not specified
	}
	gqa := NewGQA(cfg, backend)

	assert.Equal(t, 32, gqa.config.HeadDim) // 256 / 8 = 32
}

func TestNewGQA_Panics(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// NQHeads not divisible by NKVHeads
	assert.Panics(t, func() {
		NewGQA(GQAConfig{
			EmbedDim: 256,
			NQHeads:  8,
			NKVHeads: 3, // 8 % 3 != 0
			HeadDim:  32,
		}, backend)
	})

	// Invalid dimensions
	assert.Panics(t, func() {
		NewGQA(GQAConfig{
			EmbedDim: 256,
			NQHeads:  8,
			NKVHeads: 2,
			HeadDim:  64, // 8 * 64 != 256
		}, backend)
	})

	// Zero NQHeads
	assert.Panics(t, func() {
		NewGQA(GQAConfig{
			EmbedDim: 256,
			NQHeads:  0,
			NKVHeads: 2,
			HeadDim:  32,
		}, backend)
	})
}

func TestGQA_Forward_Shape(t *testing.T) {
	backend := autodiff.New(cpu.New())

	cfg := GQAConfig{
		EmbedDim: 256,
		NQHeads:  8,
		NKVHeads: 2,
		HeadDim:  32,
	}
	gqa := NewGQA(cfg, backend)

	// Input: [batch=2, seq=10, embed=256]
	x := tensor.Randn[float32](tensor.Shape{2, 10, 256}, backend)
	out := gqa.Forward(x, nil, 0)

	// Output should have same shape
	assert.Equal(t, tensor.Shape{2, 10, 256}, out.Shape())
}

func TestGQA_Forward_WithRoPE(t *testing.T) {
	backend := autodiff.New(cpu.New())

	cfg := GQAConfig{
		EmbedDim:  256,
		NQHeads:   8,
		NKVHeads:  2,
		HeadDim:   32,
		UseRoPE:   true,
		MaxSeqLen: 512,
	}
	gqa := NewGQA(cfg, backend)

	x := tensor.Randn[float32](tensor.Shape{1, 16, 256}, backend)
	out := gqa.Forward(x, nil, 0)

	assert.Equal(t, tensor.Shape{1, 16, 256}, out.Shape())
}

func TestGQA_Forward_WithKVCache(t *testing.T) {
	backend := autodiff.New(cpu.New())

	cfg := GQAConfig{
		EmbedDim:  256,
		NQHeads:   8,
		NKVHeads:  2,
		HeadDim:   32,
		UseRoPE:   true,
		MaxSeqLen: 512,
	}
	gqa := NewGQA(cfg, backend)

	// Create cache for 2 KV heads (not 8 Q heads!)
	cache := NewKVCache[*autodiff.AutodiffBackend[*cpu.CPUBackend]](1, 2, 512, 32, backend)

	// First forward: process prompt (10 tokens)
	prompt := tensor.Randn[float32](tensor.Shape{1, 10, 256}, backend)
	out1 := gqa.Forward(prompt, cache, 0)

	assert.Equal(t, tensor.Shape{1, 10, 256}, out1.Shape())
	assert.Equal(t, 10, cache.Len())

	// Second forward: generate next token
	nextToken := tensor.Randn[float32](tensor.Shape{1, 1, 256}, backend)
	out2 := gqa.Forward(nextToken, cache, 10)

	assert.Equal(t, tensor.Shape{1, 1, 256}, out2.Shape())
	assert.Equal(t, 11, cache.Len())
}

func TestGQA_LLaMA2Config(t *testing.T) {
	// Test with LLaMA 2 7B style config (scaled down)
	backend := autodiff.New(cpu.New())

	// LLaMA 2 7B: 4096 dim, 32 Q heads, 8 KV heads, 128 head_dim
	// Scaled down: 512 dim, 8 Q heads, 2 KV heads, 64 head_dim
	cfg := GQAConfig{
		EmbedDim:  512,
		NQHeads:   8,
		NKVHeads:  2,
		HeadDim:   64,
		UseRoPE:   true,
		MaxSeqLen: 2048,
		Theta:     10000.0,
	}
	gqa := NewGQA(cfg, backend)

	x := tensor.Randn[float32](tensor.Shape{1, 32, 512}, backend)
	out := gqa.Forward(x, nil, 0)

	assert.Equal(t, tensor.Shape{1, 32, 512}, out.Shape())
}

func TestGQA_Parameters(t *testing.T) {
	backend := autodiff.New(cpu.New())

	cfg := GQAConfig{
		EmbedDim: 256,
		NQHeads:  8,
		NKVHeads: 2,
		HeadDim:  32,
	}
	gqa := NewGQA(cfg, backend)

	params := gqa.Parameters()

	// 4 linear layers * 2 params each (weight + bias) = 8 params
	assert.Equal(t, 8, len(params))
}

func TestRepeatKV_NoRepeat(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// nRep=1 should return input unchanged
	kv := tensor.Randn[float32](tensor.Shape{2, 8, 10, 64}, backend)
	result := RepeatKV(kv, 1)

	assert.Equal(t, kv, result) // Should be same pointer
}

func TestRepeatKV_Repeat4x(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// [batch=1, n_kv=2, seq=4, head_dim=8] -> [batch=1, n_q=8, seq=4, head_dim=8]
	kvData := make([]float32, 1*2*4*8)
	for i := range kvData {
		kvData[i] = float32(i)
	}
	kv, err := tensor.FromSlice[float32](kvData, tensor.Shape{1, 2, 4, 8}, backend)
	require.NoError(t, err)

	result := RepeatKV(kv, 4)

	assert.Equal(t, tensor.Shape{1, 8, 4, 8}, result.Shape())

	// Verify that head 0 is repeated at positions 0,1,2,3
	// and head 1 is repeated at positions 4,5,6,7
	resultData := result.Data()
	kvDataOrig := kv.Data()

	// Check that each KV head is repeated correctly
	seqLen := 4
	headDim := 8
	nKV := 2
	nRep := 4

	for h := 0; h < nKV; h++ {
		for r := 0; r < nRep; r++ {
			for s := 0; s < seqLen; s++ {
				srcBase := h*seqLen*headDim + s*headDim
				dstBase := (h*nRep+r)*seqLen*headDim + s*headDim
				for d := 0; d < headDim; d++ {
					assert.Equal(t, kvDataOrig[srcBase+d], resultData[dstBase+d],
						"mismatch at h=%d, r=%d, s=%d, d=%d", h, r, s, d)
				}
			}
		}
	}
}

func TestRepeatKV_Panics(t *testing.T) {
	backend := autodiff.New(cpu.New())

	// Wrong shape (3D instead of 4D)
	assert.Panics(t, func() {
		kv := tensor.Randn[float32](tensor.Shape{2, 8, 64}, backend)
		RepeatKV(kv, 2)
	})
}

func TestMQA_Config(t *testing.T) {
	cfg := MQA(512, 8, 64)

	assert.Equal(t, 512, cfg.EmbedDim)
	assert.Equal(t, 8, cfg.NQHeads)
	assert.Equal(t, 1, cfg.NKVHeads) // MQA has single KV head
	assert.Equal(t, 64, cfg.HeadDim)
}

func TestMQA_Forward(t *testing.T) {
	backend := autodiff.New(cpu.New())

	cfg := MQA(256, 8, 32)
	mqa := NewGQA(cfg, backend)

	x := tensor.Randn[float32](tensor.Shape{1, 10, 256}, backend)
	out := mqa.Forward(x, nil, 0)

	assert.Equal(t, tensor.Shape{1, 10, 256}, out.Shape())
}

func TestGQA_EqualsToMHA_WhenNKVEqualsNQ(t *testing.T) {
	// When n_kv_heads == n_q_heads, GQA should behave like MHA
	backend := autodiff.New(cpu.New())

	cfg := GQAConfig{
		EmbedDim: 256,
		NQHeads:  8,
		NKVHeads: 8, // Same as NQHeads - standard MHA
		HeadDim:  32,
	}
	gqa := NewGQA(cfg, backend)

	x := tensor.Randn[float32](tensor.Shape{2, 16, 256}, backend)
	out := gqa.Forward(x, nil, 0)

	// Should work and produce correct shape
	assert.Equal(t, tensor.Shape{2, 16, 256}, out.Shape())
}

func BenchmarkGQA_Forward(b *testing.B) {
	backend := autodiff.New(cpu.New())

	cfg := GQAConfig{
		EmbedDim: 256,
		NQHeads:  8,
		NKVHeads: 2,
		HeadDim:  32,
	}
	gqa := NewGQA(cfg, backend)

	x := tensor.Randn[float32](tensor.Shape{1, 64, 256}, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gqa.Forward(x, nil, 0)
	}
}

func BenchmarkRepeatKV_4x(b *testing.B) {
	backend := autodiff.New(cpu.New())

	kv := tensor.Randn[float32](tensor.Shape{1, 8, 128, 64}, backend)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RepeatKV(kv, 4)
	}
}
