package tensor

import (
	"fmt"
	"testing"
)

func BenchmarkTensorCreation(b *testing.B) {
	backend := NewMockBackend()
	shape := Shape{100, 100}

	b.Run("Zeros", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Zeros[float32](shape, backend)
		}
	})

	b.Run("Ones", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Ones[float32](shape, backend)
		}
	})

	b.Run("Randn", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = Randn[float32](shape, backend)
		}
	})
}

func BenchmarkShapeOperations(b *testing.B) {
	shape1 := Shape{100, 100}
	shape2 := Shape{100, 100}

	b.Run("NumElements", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = shape1.NumElements()
		}
	})

	b.Run("ComputeStrides", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = shape1.ComputeStrides()
		}
	})

	b.Run("BroadcastShapes", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _, _ = BroadcastShapes(shape1, shape2)
		}
	})

	b.Run("Validate", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = shape1.Validate()
		}
	})
}

func BenchmarkTensorElementWise(b *testing.B) {
	backend := NewMockBackend()
	sizes := []int{100, 1000, 10000}

	for _, size := range sizes {
		shape := Shape{size}
		a := Ones[float32](shape, backend)
		c := Ones[float32](shape, backend)

		b.Run(fmt.Sprintf("Add-%d", size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = a.Add(c)
			}
		})

		b.Run(fmt.Sprintf("Mul-%d", size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = a.Mul(c)
			}
		})
	}
}

func BenchmarkMatMul(b *testing.B) {
	backend := NewMockBackend()
	sizes := []int{10, 50, 100}

	for _, size := range sizes {
		shape := Shape{size, size}
		a := Randn[float32](shape, backend)
		c := Randn[float32](shape, backend)

		b.Run(fmt.Sprintf("MatMul-%dx%d", size, size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = a.MatMul(c)
			}
		})
	}
}

func BenchmarkReshapeTranspose(b *testing.B) {
	backend := NewMockBackend()
	tensor := Randn[float32](Shape{100, 100}, backend)

	b.Run("Reshape", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = tensor.Reshape(10000)
		}
	})

	b.Run("Transpose", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = tensor.T()
		}
	})
}

func BenchmarkTensorAccess(b *testing.B) {
	backend := NewMockBackend()
	tensor := Randn[float32](Shape{100, 100}, backend)

	b.Run("At", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = tensor.At(50, 50)
		}
	})

	b.Run("Set", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			tensor.Set(1.0, 50, 50)
		}
	})

	b.Run("Data", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = tensor.Data()
		}
	})
}
