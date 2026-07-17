package nn_test

import (
	"testing"

	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/nn"
	"github.com/born-ml/born/internal/tensor"
	"github.com/stretchr/testify/assert"
)

func TestSetSeed_Reproducible_Xavier(t *testing.T) {
	backend := cpu.New()

	nn.SetSeed(42)
	w1 := nn.Xavier(128, 64, tensor.Shape{128, 64}, backend)

	nn.SetSeed(42)
	w2 := nn.Xavier(128, 64, tensor.Shape{128, 64}, backend)

	d1 := w1.Data()
	d2 := w2.Data()

	assert.Equal(t, len(d1), len(d2))
	for i := range d1 {
		assert.Equal(t, d1[i], d2[i], "Xavier weight[%d] should be identical with same seed", i)
	}
}

func TestSetSeed_Reproducible_Embedding(t *testing.T) {
	backend := cpu.New()

	nn.SetSeed(123)
	e1 := nn.NewEmbedding(100, 32, backend)

	nn.SetSeed(123)
	e2 := nn.NewEmbedding(100, 32, backend)

	d1 := e1.Weight.Tensor().Data()
	d2 := e2.Weight.Tensor().Data()

	assert.Equal(t, len(d1), len(d2))
	for i := range d1 {
		assert.Equal(t, d1[i], d2[i], "Embedding weight[%d] should be identical with same seed", i)
	}
}

func TestSetSeed_DifferentSeeds_DifferentWeights(t *testing.T) {
	backend := cpu.New()

	nn.SetSeed(1)
	w1 := nn.Xavier(64, 32, tensor.Shape{64, 32}, backend)

	nn.SetSeed(2)
	w2 := nn.Xavier(64, 32, tensor.Shape{64, 32}, backend)

	d1 := w1.Data()
	d2 := w2.Data()

	differ := false
	for i := range d1 {
		if d1[i] != d2[i] {
			differ = true
			break
		}
	}
	assert.True(t, differ, "different seeds should produce different weights")
}

func TestSetSeed_Reproducible_Randn(t *testing.T) {
	backend := cpu.New()

	nn.SetSeed(42)
	t1 := tensor.Randn[float32](tensor.Shape{10, 10}, backend)

	nn.SetSeed(42)
	t2 := tensor.Randn[float32](tensor.Shape{10, 10}, backend)

	d1 := t1.Data()
	d2 := t2.Data()

	for i := range d1 {
		assert.Equal(t, d1[i], d2[i], "Randn[%d] should be identical with same seed", i)
	}
}

func TestResetSeed_NonDeterministic(t *testing.T) {
	backend := cpu.New()

	nn.SetSeed(42)
	nn.ResetSeed()

	w1 := nn.Xavier(32, 16, tensor.Shape{32, 16}, backend)
	w2 := nn.Xavier(32, 16, tensor.Shape{32, 16}, backend)

	d1 := w1.Data()
	d2 := w2.Data()

	differ := false
	for i := range d1 {
		if d1[i] != d2[i] {
			differ = true
			break
		}
	}
	assert.True(t, differ, "after ResetSeed, consecutive calls should differ")
}

func TestSetSeed_FullModel_Reproducible(t *testing.T) {
	backend := cpu.New()

	nn.SetSeed(42)
	l1 := nn.NewLinear(64, 32, backend)

	nn.SetSeed(42)
	l2 := nn.NewLinear(64, 32, backend)

	d1 := l1.Weight().Tensor().Data()
	d2 := l2.Weight().Tensor().Data()

	for i := range d1 {
		assert.Equal(t, d1[i], d2[i], "Linear weight[%d] should be identical with same seed", i)
	}
}
