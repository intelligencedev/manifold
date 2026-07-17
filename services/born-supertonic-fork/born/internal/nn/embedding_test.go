package nn_test

import (
	"testing"

	"github.com/born-ml/born/internal/autodiff"
	"github.com/born-ml/born/internal/backend/cpu"
	"github.com/born-ml/born/internal/nn"
	"github.com/born-ml/born/internal/tensor"
)

func TestEmbedding_Forward_Basic(t *testing.T) {
	backend := cpu.New()

	// Create embedding: 5 embeddings, dimension 3
	embed := nn.NewEmbedding[*cpu.CPUBackend](5, 3, backend)

	// Set known weights for testing
	weightData := []float32{
		1.0, 2.0, 3.0, // embedding 0
		4.0, 5.0, 6.0, // embedding 1
		7.0, 8.0, 9.0, // embedding 2
		10.0, 11.0, 12.0, // embedding 3
		13.0, 14.0, 15.0, // embedding 4
	}
	weight, err := tensor.FromSlice[float32](weightData, tensor.Shape{5, 3}, backend)
	if err != nil {
		t.Fatalf("Failed to create weight: %v", err)
	}
	embed.Weight.SetGrad(nil) // Clear any existing gradient
	embed.Weight = nn.NewParameter("weight", weight)

	// Lookup indices [0, 1, 2]
	indices, err := tensor.FromSlice[int32]([]int32{0, 1, 2}, tensor.Shape{3}, backend)
	if err != nil {
		t.Fatalf("Failed to create indices: %v", err)
	}

	// Forward pass
	output := embed.Forward(indices)

	// Check shape: [3, 3]
	expectedShape := tensor.Shape{3, 3}
	if !shapesEqual(output.Shape(), expectedShape) {
		t.Errorf("Expected shape %v, got %v", expectedShape, output.Shape())
	}

	// Check values
	expected := []float32{
		1.0, 2.0, 3.0, // embedding 0
		4.0, 5.0, 6.0, // embedding 1
		7.0, 8.0, 9.0, // embedding 2
	}
	actual := output.Data()
	if !slicesAlmostEqual(actual, expected, 1e-6) {
		t.Errorf("Expected %v, got %v", expected, actual)
	}
}

func TestEmbedding_Forward_Batched(t *testing.T) {
	backend := cpu.New()

	// Create embedding: 10 embeddings, dimension 4
	embed := nn.NewEmbedding[*cpu.CPUBackend](10, 4, backend)

	// Set known weights
	weightData := make([]float32, 10*4)
	for i := range weightData {
		weightData[i] = float32(i)
	}
	weight, err := tensor.FromSlice[float32](weightData, tensor.Shape{10, 4}, backend)
	if err != nil {
		t.Fatalf("Failed to create weight: %v", err)
	}
	embed.Weight = nn.NewParameter("weight", weight)

	// Batched indices [2, 3]: batch_size=2, seq_len=3
	indices, err := tensor.FromSlice[int32](
		[]int32{0, 1, 2, 3, 4, 5},
		tensor.Shape{2, 3},
		backend,
	)
	if err != nil {
		t.Fatalf("Failed to create indices: %v", err)
	}

	// Forward pass
	output := embed.Forward(indices)

	// Check shape: [2, 3, 4]
	expectedShape := tensor.Shape{2, 3, 4}
	if !shapesEqual(output.Shape(), expectedShape) {
		t.Errorf("Expected shape %v, got %v", expectedShape, output.Shape())
	}

	// Check first batch, first token (index 0)
	outData := output.Data()
	expected0 := []float32{0, 1, 2, 3}
	actual0 := outData[0:4]
	if !slicesAlmostEqual(actual0, expected0, 1e-6) {
		t.Errorf("Expected first embedding %v, got %v", expected0, actual0)
	}

	// Check second batch, third token (index 5)
	expected5 := []float32{20, 21, 22, 23} // embedding 5: indices 20-23
	actual5 := outData[20:24]              // offset: (1*3 + 2) * 4 = 20
	if !slicesAlmostEqual(actual5, expected5, 1e-6) {
		t.Errorf("Expected embedding 5: %v, got %v", expected5, actual5)
	}
}

func TestEmbedding_Forward_OutOfBounds(t *testing.T) {
	backend := cpu.New()

	embed := nn.NewEmbedding[*cpu.CPUBackend](5, 3, backend)

	tests := []struct {
		name    string
		indices []int32
		shape   tensor.Shape
	}{
		{"negative index", []int32{-1}, tensor.Shape{1}},
		{"index too large", []int32{5}, tensor.Shape{1}},
		{"mixed valid and invalid", []int32{0, 1, 10}, tensor.Shape{3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			indices, err := tensor.FromSlice[int32](tt.indices, tt.shape, backend)
			if err != nil {
				t.Fatalf("Failed to create indices: %v", err)
			}

			// Should panic
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("Expected panic for out of bounds index")
				}
			}()

			embed.Forward(indices)
		})
	}
}

func TestEmbedding_Forward_RepeatedIndices(t *testing.T) {
	backend := cpu.New()

	embed := nn.NewEmbedding[*cpu.CPUBackend](3, 2, backend)

	// Set known weights
	weightData := []float32{
		1.0, 2.0, // embedding 0
		3.0, 4.0, // embedding 1
		5.0, 6.0, // embedding 2
	}
	weight, err := tensor.FromSlice[float32](weightData, tensor.Shape{3, 2}, backend)
	if err != nil {
		t.Fatalf("Failed to create weight: %v", err)
	}
	embed.Weight = nn.NewParameter("weight", weight)

	// Repeated indices: [0, 1, 0, 2, 1]
	indices, err := tensor.FromSlice[int32]([]int32{0, 1, 0, 2, 1}, tensor.Shape{5}, backend)
	if err != nil {
		t.Fatalf("Failed to create indices: %v", err)
	}

	output := embed.Forward(indices)

	// Check shape: [5, 2]
	expectedShape := tensor.Shape{5, 2}
	if !shapesEqual(output.Shape(), expectedShape) {
		t.Errorf("Expected shape %v, got %v", expectedShape, output.Shape())
	}

	// Check values: each index should get correct embedding
	expected := []float32{
		1.0, 2.0, // index 0
		3.0, 4.0, // index 1
		1.0, 2.0, // index 0 (repeated)
		5.0, 6.0, // index 2
		3.0, 4.0, // index 1 (repeated)
	}
	actual := output.Data()
	if !slicesAlmostEqual(actual, expected, 1e-6) {
		t.Errorf("Expected %v, got %v", expected, actual)
	}
}

func TestEmbedding_Forward_DifferentShapes(t *testing.T) {
	backend := cpu.New()

	embed := nn.NewEmbedding[*cpu.CPUBackend](10, 5, backend)

	tests := []struct {
		name          string
		indicesShape  tensor.Shape
		expectedShape tensor.Shape
	}{
		{"1D indices", tensor.Shape{3}, tensor.Shape{3, 5}},
		{"2D indices", tensor.Shape{2, 4}, tensor.Shape{2, 4, 5}},
		{"3D indices", tensor.Shape{2, 3, 4}, tensor.Shape{2, 3, 4, 5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create indices with the specified shape
			numIndices := tt.indicesShape.NumElements()
			indicesData := make([]int32, numIndices)
			for i := range indicesData {
				indicesData[i] = int32(i % 10) // Valid indices
			}

			indices, err := tensor.FromSlice[int32](indicesData, tt.indicesShape, backend)
			if err != nil {
				t.Fatalf("Failed to create indices: %v", err)
			}

			output := embed.Forward(indices)

			if !shapesEqual(output.Shape(), tt.expectedShape) {
				t.Errorf("Expected shape %v, got %v", tt.expectedShape, output.Shape())
			}
		})
	}
}

func TestEmbedding_Parameters(t *testing.T) {
	backend := cpu.New()

	embed := nn.NewEmbedding[*cpu.CPUBackend](100, 50, backend)

	params := embed.Parameters()
	if len(params) != 1 {
		t.Errorf("Expected 1 parameter, got %d", len(params))
	}

	if params[0] != embed.Weight {
		t.Errorf("Expected parameter to be weight")
	}

	// Check weight shape
	expectedShape := tensor.Shape{100, 50}
	if !shapesEqual(embed.Weight.Tensor().Shape(), expectedShape) {
		t.Errorf("Expected weight shape %v, got %v", expectedShape, embed.Weight.Tensor().Shape())
	}
}

func TestNewEmbeddingWithWeight(t *testing.T) {
	backend := cpu.New()

	// Create custom weight
	weightData := []float32{
		0.1, 0.2, 0.3,
		0.4, 0.5, 0.6,
		0.7, 0.8, 0.9,
	}
	weight, err := tensor.FromSlice[float32](weightData, tensor.Shape{3, 3}, backend)
	if err != nil {
		t.Fatalf("Failed to create weight: %v", err)
	}

	// Create embedding with custom weight
	embed := nn.NewEmbeddingWithWeight(weight)

	if embed.NumEmbed != 3 {
		t.Errorf("Expected NumEmbed=3, got %d", embed.NumEmbed)
	}
	if embed.EmbedDim != 3 {
		t.Errorf("Expected EmbedDim=3, got %d", embed.EmbedDim)
	}

	// Check forward pass uses the custom weight
	indices, err := tensor.FromSlice[int32]([]int32{0, 1, 2}, tensor.Shape{3}, backend)
	if err != nil {
		t.Fatalf("Failed to create indices: %v", err)
	}

	output := embed.Forward(indices)
	actual := output.Data()
	if !slicesAlmostEqual(actual, weightData, 1e-6) {
		t.Errorf("Expected output %v, got %v", weightData, actual)
	}
}

func TestNewEmbeddingWithWeight_InvalidShape(t *testing.T) {
	backend := cpu.New()

	// Create 1D weight (invalid)
	weightData := []float32{1.0, 2.0, 3.0}
	weight, err := tensor.FromSlice[float32](weightData, tensor.Shape{3}, backend)
	if err != nil {
		t.Fatalf("Failed to create weight: %v", err)
	}

	// Should panic
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for 1D weight")
		}
	}()

	nn.NewEmbeddingWithWeight(weight)
}

// ============================================================================
// Backward pass tests (autodiff)
// ============================================================================

// TestEmbedding_Backward_1D tests gradient computation for 1D indices [N].
//
// Gradient shape: gradWeight [V, D] where each row accumulates grads from
// matching indices. With a sum loss (all-ones output grad), every row that
// appears once gets grad [1,1,...,1].
func TestEmbedding_Backward_1D(t *testing.T) {
	backend := autodiff.New(cpu.New())
	backend.Tape().StartRecording()
	defer backend.Tape().StopRecording()

	// vocab=5, dim=3, indices: 3 tokens
	embed := nn.NewEmbedding[*autodiff.AutodiffBackend[*cpu.CPUBackend]](5, 3, backend)

	indices, err := tensor.FromSlice[int32]([]int32{0, 2, 4}, tensor.Shape{3}, backend)
	if err != nil {
		t.Fatalf("failed to create indices: %v", err)
	}

	output := embed.Forward(indices) // [3, 3]

	// Use the output directly as the loss tensor — autodiff.Backward initializes
	// output gradient to all-ones matching output.Shape(), which is equivalent to
	// a sum-reduction loss (∂sum/∂output = 1 everywhere).
	grads := autodiff.Backward(output, backend)

	weightGrad := grads[embed.Weight.Tensor().Raw()]
	if weightGrad == nil {
		t.Fatal("embedding weight gradient is nil")
	}

	// Shape must match weight matrix [vocab=5, dim=3]
	expectedShape := tensor.Shape{5, 3}
	if !weightGrad.Shape().Equal(expectedShape) {
		t.Errorf("grad shape = %v, want %v", weightGrad.Shape(), expectedShape)
	}

	gradData := weightGrad.AsFloat32()
	const eps = 1e-5

	// Indices 0, 2, 4 each appear once → their rows get grad [1, 1, 1].
	// Indices 1, 3 never appear → their rows stay [0, 0, 0].
	usedRows := map[int]float32{0: 1.0, 2: 1.0, 4: 1.0}
	for row := 0; row < 5; row++ {
		want := usedRows[row] // 0.0 if not in map
		for col := 0; col < 3; col++ {
			got := gradData[row*3+col]
			if got < want-eps || got > want+eps {
				t.Errorf("gradWeight[%d, %d] = %f, want %f", row, col, got, want)
			}
		}
	}
}

// TestEmbedding_Backward_2D tests gradient computation for 2D indices [B, S].
//
// This is the critical regression test: before the fix, passing [B, S, D]
// gradOutput to SelectAdd (which expected [N, D]) caused a panic or wrong
// shapes. After flattening, grads must accumulate correctly.
func TestEmbedding_Backward_2D(t *testing.T) {
	backend := autodiff.New(cpu.New())
	backend.Tape().StartRecording()
	defer backend.Tape().StopRecording()

	// vocab=5, dim=4, batch=2, seq=3
	embed := nn.NewEmbedding[*autodiff.AutodiffBackend[*cpu.CPUBackend]](5, 4, backend)

	// 2D indices [2, 3]: index 0 appears twice (positions [0,0] and [1,2])
	indices, err := tensor.FromSlice[int32](
		[]int32{0, 1, 2, 3, 4, 0},
		tensor.Shape{2, 3},
		backend,
	)
	if err != nil {
		t.Fatalf("failed to create indices: %v", err)
	}

	output := embed.Forward(indices) // [2, 3, 4]

	if !output.Shape().Equal(tensor.Shape{2, 3, 4}) {
		t.Fatalf("forward output shape = %v, want [2 3 4]", output.Shape())
	}

	grads := autodiff.Backward(output, backend)

	weightGrad := grads[embed.Weight.Tensor().Raw()]
	if weightGrad == nil {
		t.Fatal("embedding weight gradient is nil (3D backward panic regression)")
	}

	if !weightGrad.Shape().Equal(tensor.Shape{5, 4}) {
		t.Errorf("grad shape = %v, want [5 4]", weightGrad.Shape())
	}

	gradData := weightGrad.AsFloat32()
	const eps = 1e-5

	// Index 0 appears twice → row 0 accumulates 2 grad rows of all-ones → [2, 2, 2, 2]
	for col := 0; col < 4; col++ {
		got := gradData[0*4+col]
		if got < 2.0-eps || got > 2.0+eps {
			t.Errorf("gradWeight[0, %d] = %f, want 2.0 (index 0 appears twice)", col, got)
		}
	}

	// Indices 1–4 each appear once → rows 1–4 get [1, 1, 1, 1]
	for row := 1; row <= 4; row++ {
		for col := 0; col < 4; col++ {
			got := gradData[row*4+col]
			if got < 1.0-eps || got > 1.0+eps {
				t.Errorf("gradWeight[%d, %d] = %f, want 1.0", row, col, got)
			}
		}
	}
}

// TestEmbedding_Backward_DuplicateIndices tests that scatter-add correctly
// accumulates gradients when the same embedding index is referenced multiple times.
func TestEmbedding_Backward_DuplicateIndices(t *testing.T) {
	backend := autodiff.New(cpu.New())
	backend.Tape().StartRecording()
	defer backend.Tape().StopRecording()

	// vocab=3, dim=2; indices: [0, 1, 0] — index 0 repeats twice
	embed := nn.NewEmbedding[*autodiff.AutodiffBackend[*cpu.CPUBackend]](3, 2, backend)

	indices, err := tensor.FromSlice[int32]([]int32{0, 1, 0}, tensor.Shape{3}, backend)
	if err != nil {
		t.Fatalf("failed to create indices: %v", err)
	}

	output := embed.Forward(indices) // [3, 2]
	grads := autodiff.Backward(output, backend)

	weightGrad := grads[embed.Weight.Tensor().Raw()]
	if weightGrad == nil {
		t.Fatal("embedding weight gradient is nil")
	}

	if !weightGrad.Shape().Equal(tensor.Shape{3, 2}) {
		t.Errorf("grad shape = %v, want [3 2]", weightGrad.Shape())
	}

	gradData := weightGrad.AsFloat32()
	const eps = 1e-5

	// Row 0: index 0 appears at positions 0 and 2 → accumulated grad = 2.0 per dim
	for col := 0; col < 2; col++ {
		got := gradData[0*2+col]
		if got < 2.0-eps || got > 2.0+eps {
			t.Errorf("gradWeight[0, %d] = %f, want 2.0 (duplicate accumulation)", col, got)
		}
	}

	// Row 1: index 1 appears once → grad = 1.0 per dim
	for col := 0; col < 2; col++ {
		got := gradData[1*2+col]
		if got < 1.0-eps || got > 1.0+eps {
			t.Errorf("gradWeight[1, %d] = %f, want 1.0", col, got)
		}
	}

	// Row 2: index 2 never appears → grad = 0.0
	for col := 0; col < 2; col++ {
		got := gradData[2*2+col]
		if got < -eps || got > eps {
			t.Errorf("gradWeight[2, %d] = %f, want 0.0 (unused index)", col, got)
		}
	}
}

// TestEmbedding_Backward_GradientFlow verifies end-to-end gradient propagation
// through an Embedding → Linear → sum pipeline. This is the "embedding learns"
// test: weight gradients must be non-zero when a downstream loss is backpropagated.
func TestEmbedding_Backward_GradientFlow(t *testing.T) {
	backend := autodiff.New(cpu.New())
	backend.Tape().StartRecording()
	defer backend.Tape().StopRecording()

	const (
		vocab    = 10
		embedDim = 8
		outDim   = 4
		seqLen   = 5
	)

	embed := nn.NewEmbedding[*autodiff.AutodiffBackend[*cpu.CPUBackend]](vocab, embedDim, backend)
	linear := nn.NewLinear(embedDim, outDim, backend)

	indices, err := tensor.FromSlice[int32](
		[]int32{0, 1, 2, 3, 4},
		tensor.Shape{seqLen},
		backend,
	)
	if err != nil {
		t.Fatalf("failed to create indices: %v", err)
	}

	// Embedding → [seqLen, embedDim]
	embOut := embed.Forward(indices)

	// Linear requires [batch, in_features]; treat seqLen as the batch dimension.
	linOut := linear.Forward(embOut) // [seqLen, outDim]

	// Treat the full output as the loss (sum-grad = all-ones).
	grads := autodiff.Backward(linOut, backend)

	// Embedding weight gradient must exist and be non-zero.
	weightGrad := grads[embed.Weight.Tensor().Raw()]
	if weightGrad == nil {
		t.Fatal("embedding weight gradient is nil (gradient did not flow through Linear)")
	}

	if !weightGrad.Shape().Equal(tensor.Shape{vocab, embedDim}) {
		t.Errorf("embed weight grad shape = %v, want [%d %d]", weightGrad.Shape(), vocab, embedDim)
	}

	gradData := weightGrad.AsFloat32()
	hasNonZero := false
	for _, g := range gradData {
		if g != 0.0 {
			hasNonZero = true
			break
		}
	}
	if !hasNonZero {
		t.Error("embedding weight gradient is all zeros — gradient did not flow from Linear")
	}

	// Linear weight gradient must also exist (sanity check that tape captured everything).
	linWeightGrad := grads[linear.Weight().Tensor().Raw()]
	if linWeightGrad == nil {
		t.Error("linear weight gradient is nil")
	}
}

// TestEmbedding_Backward_ScalarMul verifies that gradients flow through
// MulScalar correctly: after Embedding → MulScalar(scale) → backward,
// the embedding weight gradient should be scaled by the scalar.
func TestEmbedding_Backward_ScalarMul(t *testing.T) {
	backend := autodiff.New(cpu.New())
	backend.Tape().StartRecording()
	defer backend.Tape().StopRecording()

	// vocab=4, dim=3; each index appears exactly once
	embed := nn.NewEmbedding[*autodiff.AutodiffBackend[*cpu.CPUBackend]](4, 3, backend)

	indices, err := tensor.FromSlice[int32]([]int32{0, 1, 2, 3}, tensor.Shape{4}, backend)
	if err != nil {
		t.Fatalf("failed to create indices: %v", err)
	}

	const scale = float32(3.0)

	embOut := embed.Forward(indices)  // [4, 3]
	scaled := embOut.MulScalar(scale) // [4, 3] — each element × scale

	grads := autodiff.Backward(scaled, backend)

	weightGrad := grads[embed.Weight.Tensor().Raw()]
	if weightGrad == nil {
		t.Fatal("embedding weight gradient is nil after MulScalar (scalar-grad flow regression)")
	}

	if !weightGrad.Shape().Equal(tensor.Shape{4, 3}) {
		t.Errorf("grad shape = %v, want [4 3]", weightGrad.Shape())
	}

	gradData := weightGrad.AsFloat32()
	const eps = 1e-5

	// All four indices appear once; output grad through MulScalar is scale × 1.0.
	// So every row of gradWeight should equal [scale, scale, scale].
	for row := 0; row < 4; row++ {
		for col := 0; col < 3; col++ {
			got := gradData[row*3+col]
			if got < scale-eps || got > scale+eps {
				t.Errorf("gradWeight[%d, %d] = %f, want %f (scale factor)", row, col, got, scale)
			}
		}
	}
}

// Helper functions.

func shapesEqual(a, b tensor.Shape) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

//nolint:unparam // tolerance parameter allows flexible comparison in future tests
func slicesAlmostEqual(a, b []float32, tolerance float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		diff := a[i] - b[i]
		if diff < 0 {
			diff = -diff
		}
		if diff > tolerance {
			return false
		}
	}
	return true
}
