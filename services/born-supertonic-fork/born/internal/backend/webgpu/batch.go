//go:build windows || linux

package webgpu

import (
	"fmt"

	wgpu "github.com/gogpu/wgpu"
)

// CommandBatch accumulates GPU operations for single submission.
// Instead of submitting each operation separately (causing GPU overhead),
// we collect all operations in a batch and submit them together.
type CommandBatch struct {
	backend *Backend
	encoder *wgpu.CommandEncoder
	ops     []pendingOp
}

// pendingOp represents a single GPU operation waiting to be executed.
type pendingOp struct {
	name     string     // Operation name for debugging (e.g., "add", "matmul")
	output   *GPUTensor // Result tensor
	execFunc func()     // Function to encode the operation
}

// NewBatch creates a new command batch for accumulating operations.
// The batch will use a single CommandEncoder for all operations.
func (b *Backend) NewBatch() *CommandBatch {
	encoder, err := b.device.CreateCommandEncoder(nil)
	if err != nil {
		panic(fmt.Sprintf("webgpu: NewBatch: failed to create command encoder: %v", err))
	}
	return &CommandBatch{
		backend: b,
		encoder: encoder,
		ops:     make([]pendingOp, 0, 8), // Pre-allocate for typical batch size
	}
}

// Add adds an operation to the batch.
// The operation function should encode the compute pass but NOT submit it.
// Returns the batch for method chaining.
func (batch *CommandBatch) Add(name string, output *GPUTensor, execFunc func()) *CommandBatch {
	batch.ops = append(batch.ops, pendingOp{
		name:     name,
		output:   output,
		execFunc: execFunc,
	})
	return batch
}

// Submit executes all batched operations in a single GPU submission.
// This dramatically reduces GPU overhead compared to submitting each operation separately.
//
// Example performance difference:
//
//	3 separate submissions: encode → submit → wait (×3) = ~1.5ms overhead
//	1 batched submission:   encode → encode → encode → submit → wait = ~0.5ms overhead
//
// The batch is consumed after Submit() and cannot be reused.
func (batch *CommandBatch) Submit() {
	if len(batch.ops) == 0 {
		return
	}

	// Execute all operation encoding functions
	for i := range batch.ops {
		batch.ops[i].execFunc()
	}

	// Finish command encoder and submit all commands at once
	cmdBuffer, err := batch.encoder.Finish()
	if err != nil {
		panic(fmt.Sprintf("webgpu: CommandBatch.Submit: failed to finish encoder: %v", err))
	}
	if _, submitErr := batch.backend.queue.Submit(cmdBuffer); submitErr != nil {
		panic(fmt.Sprintf("webgpu: CommandBatch.Submit: failed to submit commands: %v", submitErr))
	}

	// Mark all outputs as computed
	for i := range batch.ops {
		if batch.ops[i].output != nil {
			batch.ops[i].output.computed = true
		}
	}
}

// Count returns the number of operations in the batch.
func (batch *CommandBatch) Count() int {
	return len(batch.ops)
}
