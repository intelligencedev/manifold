// Copyright 2025 Born ML Framework. All rights reserved.
// Use of this source code is governed by an Apache 2.0
// license that can be found in the LICENSE file.

package tensor

import (
	"github.com/born-ml/born/internal/tensor"
)

// RawTensor is the low-level tensor representation.
//
// RawTensor provides:
//   - Shape and type information via Shape(), DType(), Device()
//   - Type-safe data access via AsFloat32(), AsInt64(), etc.
//   - Copy-on-Write semantics via Clone()
//   - Lazy GPU evaluation support via IsLazy()
//   - Reference counting for efficient memory management
//
// Most users should use the high-level Tensor[T, B] type instead.
//
// Example:
//
//	raw, _ := tensor.NewRaw(tensor.Shape{2, 3}, tensor.Float32, tensor.CPU)
//	data := raw.AsFloat32()  // Type-safe access
//	clone := raw.Clone()     // Shares buffer via reference counting
type RawTensor = tensor.RawTensor
