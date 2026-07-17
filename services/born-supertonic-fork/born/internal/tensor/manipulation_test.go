package tensor

import (
	"testing"
)

// Helper function to create tensor from slice, panicking on error.
func mustFromSlice[T DType, B Backend](t *testing.T, data []T, shape Shape, backend B) *Tensor[T, B] {
	tensor, err := FromSlice(data, shape, backend)
	if err != nil {
		t.Fatalf("FromSlice failed: %v", err)
	}
	return tensor
}

// TestCat tests concatenation of tensors along various dimensions.
func TestCat(t *testing.T) {
	backend := NewMockBackend()

	t.Run("concat 2 tensors along dim 0", func(t *testing.T) {
		a := mustFromSlice(t, []float32{1, 2, 3}, Shape{1, 3}, backend)
		b := mustFromSlice(t, []float32{4, 5, 6}, Shape{1, 3}, backend)

		result := Cat([]*Tensor[float32, *MockBackend]{a, b}, 0)

		expected := Shape{2, 3}
		if !result.Shape().Equal(expected) {
			t.Errorf("expected shape %v, got %v", expected, result.Shape())
		}

		wantData := []float32{1, 2, 3, 4, 5, 6}
		got := result.Data()
		if !sliceEqual(got, wantData) {
			t.Errorf("expected data %v, got %v", wantData, got)
		}
	})

	t.Run("concat 2 tensors along dim 1", func(t *testing.T) {
		a := mustFromSlice(t, []float32{1, 2, 3, 4}, Shape{2, 2}, backend)
		b := mustFromSlice(t, []float32{5, 6, 7, 8}, Shape{2, 2}, backend)

		result := Cat([]*Tensor[float32, *MockBackend]{a, b}, 1)

		expected := Shape{2, 4}
		if !result.Shape().Equal(expected) {
			t.Errorf("expected shape %v, got %v", expected, result.Shape())
		}

		wantData := []float32{1, 2, 5, 6, 3, 4, 7, 8}
		got := result.Data()
		if !sliceEqual(got, wantData) {
			t.Errorf("expected data %v, got %v", wantData, got)
		}
	})

	t.Run("concat 3 tensors along dim -1", func(t *testing.T) {
		a := mustFromSlice(t, []float32{1, 2}, Shape{2, 1}, backend)
		b := mustFromSlice(t, []float32{3, 4}, Shape{2, 1}, backend)
		c := mustFromSlice(t, []float32{5, 6}, Shape{2, 1}, backend)

		result := Cat([]*Tensor[float32, *MockBackend]{a, b, c}, -1)

		expected := Shape{2, 3}
		if !result.Shape().Equal(expected) {
			t.Errorf("expected shape %v, got %v", expected, result.Shape())
		}

		wantData := []float32{1, 3, 5, 2, 4, 6}
		got := result.Data()
		if !sliceEqual(got, wantData) {
			t.Errorf("expected data %v, got %v", wantData, got)
		}
	})

	t.Run("concat single tensor returns clone", func(t *testing.T) {
		a := mustFromSlice(t, []float32{1, 2, 3, 4}, Shape{2, 2}, backend)

		result := Cat([]*Tensor[float32, *MockBackend]{a}, 0)

		expected := Shape{2, 2}
		if !result.Shape().Equal(expected) {
			t.Errorf("expected shape %v, got %v", expected, result.Shape())
		}

		wantData := []float32{1, 2, 3, 4}
		got := result.Data()
		if !sliceEqual(got, wantData) {
			t.Errorf("expected data %v, got %v", wantData, got)
		}
	})
}

// TestCatPanics tests error cases for Cat.
func TestCatPanics(t *testing.T) {
	t.Run("empty tensors list", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic")
			}
		}()
		Cat([]*Tensor[float32, *MockBackend]{}, 0)
	})
}

// TestChunk tests splitting tensor into equal parts.
func TestChunk(t *testing.T) {
	backend := NewMockBackend()

	t.Run("chunk into 2 parts along dim 0", func(t *testing.T) {
		input := mustFromSlice(t, []float32{1, 2, 3, 4, 5, 6}, Shape{6}, backend)
		chunks := input.Chunk(2, 0)

		if len(chunks) != 2 {
			t.Errorf("expected 2 chunks, got %d", len(chunks))
		}

		expectedShape := Shape{3}
		for i, chunk := range chunks {
			if !chunk.Shape().Equal(expectedShape) {
				t.Errorf("chunk %d: expected shape %v, got %v", i, expectedShape, chunk.Shape())
			}
		}

		// Verify concatenating chunks gives back original
		reconstructed := Cat(chunks, 0)
		origData := input.Data()
		reconData := reconstructed.Data()
		if !sliceEqual(origData, reconData) {
			t.Errorf("reconstructed data doesn't match original")
		}
	})

	t.Run("chunk into 3 parts along dim 1", func(t *testing.T) {
		input := mustFromSlice(t, []float32{1, 2, 3, 4, 5, 6}, Shape{2, 3}, backend)
		chunks := input.Chunk(3, 1)

		if len(chunks) != 3 {
			t.Errorf("expected 3 chunks, got %d", len(chunks))
		}

		expectedShape := Shape{2, 1}
		for i, chunk := range chunks {
			if !chunk.Shape().Equal(expectedShape) {
				t.Errorf("chunk %d: expected shape %v, got %v", i, expectedShape, chunk.Shape())
			}
		}
	})

	t.Run("chunk into 1 part returns original", func(t *testing.T) {
		input := mustFromSlice(t, []float32{1, 2, 3, 4}, Shape{2, 2}, backend)
		chunks := input.Chunk(1, 0)

		if len(chunks) != 1 {
			t.Errorf("expected 1 chunk, got %d", len(chunks))
		}

		expectedShape := Shape{2, 2}
		if !chunks[0].Shape().Equal(expectedShape) {
			t.Errorf("expected shape %v, got %v", expectedShape, chunks[0].Shape())
		}
	})
}

// TestChunkPanics tests error cases for Chunk.
func TestChunkPanics(t *testing.T) {
	backend := NewMockBackend()

	t.Run("n is zero", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic when n is zero")
			}
		}()
		input := mustFromSlice(t, []float32{1, 2, 3, 4}, Shape{2, 2}, backend)
		input.Chunk(0, 0)
	})

	t.Run("dimension not divisible by n", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic when dimension not divisible")
			}
		}()
		input := mustFromSlice(t, []float32{1, 2, 3, 4, 5}, Shape{5}, backend)
		input.Chunk(2, 0)
	})
}

// TestUnsqueeze tests adding dimensions.
func TestUnsqueeze(t *testing.T) {
	backend := NewMockBackend()

	tests := []struct {
		name     string
		input    *Tensor[float32, *MockBackend]
		dim      int
		expected Shape
	}{
		{
			name:     "unsqueeze at beginning",
			input:    mustFromSlice(t, []float32{1, 2, 3, 4}, Shape{2, 2}, backend),
			dim:      0,
			expected: Shape{1, 2, 2},
		},
		{
			name:     "unsqueeze in middle",
			input:    mustFromSlice(t, []float32{1, 2, 3, 4}, Shape{2, 2}, backend),
			dim:      1,
			expected: Shape{2, 1, 2},
		},
		{
			name:     "unsqueeze at end",
			input:    mustFromSlice(t, []float32{1, 2, 3, 4}, Shape{2, 2}, backend),
			dim:      2,
			expected: Shape{2, 2, 1},
		},
		{
			name:     "unsqueeze with negative dim",
			input:    mustFromSlice(t, []float32{1, 2, 3, 4}, Shape{2, 2}, backend),
			dim:      -1,
			expected: Shape{2, 2, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input.Unsqueeze(tt.dim)

			if !result.Shape().Equal(tt.expected) {
				t.Errorf("expected shape %v, got %v", tt.expected, result.Shape())
			}

			// Verify data is unchanged
			origData := tt.input.Data()
			resultData := result.Data()
			if !sliceEqual(origData, resultData) {
				t.Errorf("data changed after unsqueeze")
			}
		})
	}
}

// TestSqueeze tests removing dimensions.
func TestSqueeze(t *testing.T) {
	backend := NewMockBackend()

	tests := []struct {
		name     string
		input    *Tensor[float32, *MockBackend]
		dim      int
		expected Shape
	}{
		{
			name:     "squeeze first dim",
			input:    mustFromSlice(t, []float32{1, 2, 3, 4}, Shape{1, 2, 2}, backend),
			dim:      0,
			expected: Shape{2, 2},
		},
		{
			name:     "squeeze middle dim",
			input:    mustFromSlice(t, []float32{1, 2, 3, 4}, Shape{2, 1, 2}, backend),
			dim:      1,
			expected: Shape{2, 2},
		},
		{
			name:     "squeeze last dim",
			input:    mustFromSlice(t, []float32{1, 2, 3, 4}, Shape{2, 2, 1}, backend),
			dim:      2,
			expected: Shape{2, 2},
		},
		{
			name:     "squeeze with negative dim",
			input:    mustFromSlice(t, []float32{1, 2, 3, 4}, Shape{2, 2, 1}, backend),
			dim:      -1,
			expected: Shape{2, 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input.Squeeze(tt.dim)

			if !result.Shape().Equal(tt.expected) {
				t.Errorf("expected shape %v, got %v", tt.expected, result.Shape())
			}

			// Verify data is unchanged
			origData := tt.input.Data()
			resultData := result.Data()
			if !sliceEqual(origData, resultData) {
				t.Errorf("data changed after squeeze")
			}
		})
	}
}

// TestSqueezePanics tests error cases for Squeeze.
func TestSqueezePanics(t *testing.T) {
	backend := NewMockBackend()

	t.Run("squeeze non-1 dimension", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic when squeezing non-1 dimension")
			}
		}()

		input := mustFromSlice(t, []float32{1, 2, 3, 4}, Shape{2, 2}, backend)
		input.Squeeze(0) // Dimension 0 has size 2, not 1
	})
}

// TestUnsqueezeSqueezeRoundtrip tests that unsqueeze and squeeze are inverses.
func TestUnsqueezeSqueezeRoundtrip(t *testing.T) {
	backend := NewMockBackend()

	input := mustFromSlice(t, []float32{1, 2, 3, 4, 5, 6}, Shape{2, 3}, backend)
	origShape := input.Shape()
	origData := input.Data()

	// Unsqueeze then squeeze
	unsqueezed := input.Unsqueeze(1)  // [2, 1, 3]
	squeezed := unsqueezed.Squeeze(1) // [2, 3]

	if !squeezed.Shape().Equal(origShape) {
		t.Errorf("roundtrip shape: expected %v, got %v", origShape, squeezed.Shape())
	}

	squeezedData := squeezed.Data()
	if !sliceEqual(origData, squeezedData) {
		t.Errorf("roundtrip data changed")
	}
}

// TestCatChunkRoundtrip tests that Cat and Chunk are inverses.
func TestCatChunkRoundtrip(t *testing.T) {
	backend := NewMockBackend()

	// Create test tensor
	input := Arange[float32](0, 24, backend).Reshape(2, 3, 4)
	origData := input.Data()

	// Chunk along each dimension
	for dim := 0; dim < 3; dim++ {
		dimSize := input.Shape()[dim]
		for n := 1; n <= dimSize; n++ {
			if dimSize%n != 0 {
				continue
			}

			chunks := input.Chunk(n, dim)
			reconstructed := Cat(chunks, dim)

			if !reconstructed.Shape().Equal(input.Shape()) {
				t.Errorf("dim=%d n=%d: shape mismatch %v != %v", dim, n, reconstructed.Shape(), input.Shape())
			}

			reconData := reconstructed.Data()
			if !sliceEqual(origData, reconData) {
				t.Errorf("dim=%d n=%d: data mismatch", dim, n)
			}
		}
	}
}

// TestClone tests tensor cloning.
func TestClone(t *testing.T) {
	backend := NewMockBackend()

	input := mustFromSlice(t, []float32{1, 2, 3, 4, 5, 6}, Shape{2, 3}, backend)
	cloned := input.Clone()

	// Check shape matches
	if !cloned.Shape().Equal(input.Shape()) {
		t.Errorf("cloned shape %v != original %v", cloned.Shape(), input.Shape())
	}

	// Check data matches
	origData := input.Data()
	clonedData := cloned.Data()
	if !sliceEqual(origData, clonedData) {
		t.Errorf("cloned data doesn't match original")
	}

	// Note: Clone() uses copy-on-write, so buffer may be shared
	// This is the expected behavior for performance
}

// TestManipulationDTypes tests manipulation operations with different data types.
func TestManipulationDTypes(t *testing.T) {
	backend := NewMockBackend()

	t.Run("float64 cat", func(t *testing.T) {
		a := mustFromSlice(t, []float64{1.1, 2.2}, Shape{2}, backend)
		b := mustFromSlice(t, []float64{3.3, 4.4}, Shape{2}, backend)
		result := Cat([]*Tensor[float64, *MockBackend]{a, b}, 0)

		expected := []float64{1.1, 2.2, 3.3, 4.4}
		got := result.Data()

		for i := range expected {
			if got[i] != expected[i] {
				t.Errorf("index %d: expected %f, got %f", i, expected[i], got[i])
			}
		}
	})

	t.Run("int32 chunk", func(t *testing.T) {
		input := mustFromSlice(t, []int32{1, 2, 3, 4, 5, 6}, Shape{6}, backend)
		chunks := input.Chunk(3, 0)

		if len(chunks) != 3 {
			t.Errorf("expected 3 chunks, got %d", len(chunks))
		}

		expected := [][]int32{{1, 2}, {3, 4}, {5, 6}}
		for i, chunk := range chunks {
			got := chunk.Data()
			for j := range expected[i] {
				if got[j] != expected[i][j] {
					t.Errorf("chunk %d: index %d: expected %d, got %d", i, j, expected[i][j], got[j])
				}
			}
		}
	})
}

// Helper function to compare float32 slices.
func sliceEqual(a, b []float32) bool {
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
