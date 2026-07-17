// Copyright 2025 Born ML Framework. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

// Package nn provides neural network layers and building blocks.
//
// # Overview
//
// This package contains:
//   - Layers: Linear, Conv2D, MaxPool2D
//   - Activations: ReLU, Sigmoid, Tanh
//   - Loss functions: CrossEntropyLoss, MSELoss
//   - Utilities: Sequential, Module interface, Parameter
//   - Initialization: Xavier, Zeros, Ones, Randn
//
// # Basic Usage
//
//	import (
//	    "github.com/born-ml/born/nn"
//	    "github.com/born-ml/born/backend/cpu"
//	)
//
//	func main() {
//	    backend := cpu.New()
//
//	    // Build a simple MLP
//	    model := nn.NewSequential(
//	        nn.NewLinear(784, 128, backend),
//	        nn.NewReLU(),
//	        nn.NewLinear(128, 10, backend),
//	    )
//
//	    // Forward pass
//	    output := model.Forward(input)
//	}
//
// # Layers
//
// Linear: Fully connected layer with Xavier initialization
//
//	layer := nn.NewLinear(inFeatures, outFeatures, backend)
//
// Conv2D: 2D convolutional layer with im2col algorithm
//
//	conv := nn.NewConv2D(inChannels, outChannels, kernelSize, stride, padding, backend)
//
// MaxPool2D: 2D max pooling layer
//
//	pool := nn.NewMaxPool2D(kernelSize, stride, backend)
//
// # Activations
//
// Common activation functions:
//
//	relu := nn.NewReLU()
//	sigmoid := nn.NewSigmoid()
//	tanh := nn.NewTanh()
//
// # Loss Functions
//
// CrossEntropyLoss: For classification tasks (numerically stable)
//
//	criterion := nn.NewCrossEntropyLoss(backend)
//	loss := criterion.Forward(logits, labels)
//
// MSELoss: For regression tasks
//
//	criterion := nn.NewMSELoss(backend)
//	loss := criterion.Forward(predictions, targets)
//
// # Sequential Models
//
// Build models by composing layers:
//
//	model := nn.NewSequential(
//	    nn.NewLinear(784, 256, backend),
//	    nn.NewReLU(),
//	    nn.NewLinear(256, 128, backend),
//	    nn.NewReLU(),
//	    nn.NewLinear(128, 10, backend),
//	)
//
// # Parameter Management
//
// Access model parameters for optimization:
//
//	params := model.Parameters()
//	for _, param := range params {
//	    fmt.Println(param.Name(), param.Tensor().Shape())
//	}
package nn
