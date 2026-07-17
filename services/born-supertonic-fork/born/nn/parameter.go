// Copyright 2025 Born ML Framework. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package nn

import (
	"github.com/born-ml/born/internal/nn"
	"github.com/born-ml/born/tensor"
)

// Parameter represents a trainable parameter in a neural network.
//
// Parameters are tensors that require gradient computation during training.
// They typically represent weights and biases of layers.
//
// Example:
//
//	// Create a weight parameter
//	weight := nn.NewParameter("weight", weightTensor)
//
//	// Access the tensor
//	w := weight.Tensor()
//
//	// Get gradient after backward pass
//	grad := weight.Grad()
//
// Methods:
//
//	Name() string
//	    Returns the parameter name (e.g., "weight", "bias").
//
//	Tensor() *tensor.Tensor[float32, B]
//	    Returns the parameter tensor.
//
//	Grad() *tensor.Tensor[float32, B]
//	    Returns the gradient tensor (nil if not computed yet).
//
//	SetGrad(grad *tensor.Tensor[float32, B])
//	    Sets the gradient tensor.
//
//	ZeroGrad()
//	    Clears the gradient tensor.
//
// Note: Parameter is implemented as a type alias because it is used as a return type
// in the Module interface. Go's type system requires exact type matches for interface
// implementations, so we cannot use an interface here.
type Parameter[B tensor.Backend] = nn.Parameter[B]
