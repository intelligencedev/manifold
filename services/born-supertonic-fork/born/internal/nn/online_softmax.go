// Package nn provides neural network modules and layers for building deep learning models.
package nn

import "math"

// OnlineSoftmax computes softmax incrementally without storing all values.
//
// This is the key enabler for Flash Attention's O(N) memory complexity.
// Instead of materializing the full attention matrix, OnlineSoftmax maintains
// running statistics (max and sum) and accumulates weighted outputs on-the-fly.
//
// Algorithm:
//
//	When processing a new block of scores:
//	  1. new_max = max(running_max, max(scores))
//	  2. scale = exp(old_max - new_max)
//	  3. running_sum = scale * running_sum + sum(exp(scores - new_max))
//	  4. output = scale * output + exp(scores - new_max) @ values
//	  5. running_max = new_max
//
//	After all blocks: output /= running_sum
//
// This maintains numerical stability while avoiding O(N²) memory.
type OnlineSoftmax struct {
	maxVal  float32   // Running maximum across all blocks.
	sumExp  float32   // Running sum of exp(x - max).
	output  []float32 // Accumulated weighted output [headDim].
	headDim int       // Dimension of each attention head.
}

// NewOnlineSoftmax creates a new online softmax accumulator.
//
// Parameters:
//   - headDim: Dimension of the attention head (output size).
//
// Returns:
//   - *OnlineSoftmax: Initialized accumulator with maxVal = -inf, sumExp = 0, output = zeros.
//
// Example:
//
//	softmax := nn.NewOnlineSoftmax(64) // For head_dim=64
//	softmax.Update(scores1, values1)   // Process first block
//	softmax.Update(scores2, values2)   // Process second block
//	result := softmax.Normalize()      // Get final output
func NewOnlineSoftmax(headDim int) *OnlineSoftmax {
	return &OnlineSoftmax{
		maxVal:  float32(math.Inf(-1)), // Start with -infinity
		sumExp:  0,
		output:  make([]float32, headDim),
		headDim: headDim,
	}
}

// Update processes a block of attention scores and values.
//
// This method implements the core online softmax algorithm:
//   - Updates the running maximum.
//   - Rescales previous accumulations.
//   - Adds new weighted contributions.
//
// Parameters:
//   - scores: [blockSize] - attention scores for this block (QK^T / sqrt(d_k)).
//   - values: [blockSize * headDim] - V values for this block (flattened row-major).
//
// The values tensor should be in row-major order:
//
//	[v0_0, v0_1, ..., v0_{headDim-1}, v1_0, v1_1, ..., v1_{headDim-1}, ...]
//
// Example:
//
//	scores := []float32{1.0, 2.0, 3.0} // 3 keys
//	values := []float32{1, 2, 3, 4, 5, 6, 7, 8, 9} // [3, 3] flattened
//	softmax.Update(scores, values)
func (o *OnlineSoftmax) Update(scores, values []float32) {
	blockSize := len(scores)
	if len(values) != blockSize*o.headDim {
		panic("OnlineSoftmax.Update: values length must be blockSize * headDim")
	}

	// 1. Compute new maximum across this block
	blockMax := float32(math.Inf(-1))
	for _, score := range scores {
		if score > blockMax {
			blockMax = score
		}
	}

	// 2. Update running maximum and compute correction factor
	oldMax := o.maxVal
	newMax := max(oldMax, blockMax)
	correction := float32(math.Exp(float64(oldMax - newMax)))

	// 3. Rescale previous sum and output by correction factor
	o.sumExp *= correction
	for i := range o.output {
		o.output[i] *= correction
	}

	// 4. Add contributions from this block
	for i := 0; i < blockSize; i++ {
		// Compute exp(score - newMax) for numerical stability
		expScore := float32(math.Exp(float64(scores[i] - newMax)))
		o.sumExp += expScore

		// Accumulate weighted values: output += expScore * values[i]
		for j := 0; j < o.headDim; j++ {
			o.output[j] += expScore * values[i*o.headDim+j]
		}
	}

	// 5. Update running maximum
	o.maxVal = newMax
}

// Normalize returns the final output after all blocks have been processed.
//
// This divides the accumulated output by the sum of exponentials to get
// the properly normalized softmax-weighted sum.
//
// Returns:
//   - []float32: [headDim] - normalized attention output.
//
// Example:
//
//	result := softmax.Normalize() // Final attention output for this query
func (o *OnlineSoftmax) Normalize() []float32 {
	result := make([]float32, o.headDim)
	for i := range result {
		result[i] = o.output[i] / o.sumExp
	}
	return result
}

// Reset clears the accumulator for reuse.
//
// This allows reusing the same OnlineSoftmax instance for multiple queries,
// avoiding repeated allocations.
//
// Example:
//
//	softmax := nn.NewOnlineSoftmax(64)
//	for _, query := range queries {
//	    // Process blocks for this query
//	    softmax.Update(scores1, values1)
//	    result := softmax.Normalize()
//	    // Use result...
//	    softmax.Reset() // Prepare for next query
//	}
func (o *OnlineSoftmax) Reset() {
	o.maxVal = float32(math.Inf(-1))
	o.sumExp = 0
	for i := range o.output {
		o.output[i] = 0
	}
}
