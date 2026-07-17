//go:build !wasm

package operators

import (
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()

	// Check that essential operators are registered
	essentialOps := []string{
		"Add", "Sub", "Mul", "Div", "MatMul",
		"Relu", "Sigmoid", "Tanh", "Softmax",
		"Reshape", "Transpose",
		"Identity", "Dropout",
	}

	for _, op := range essentialOps {
		if _, ok := r.Get(op); !ok {
			t.Errorf("Expected operator %s to be registered", op)
		}
	}
}

func TestRegistryGetUnknown(t *testing.T) {
	r := NewRegistry()

	if _, ok := r.Get("UnknownOp"); ok {
		t.Error("Expected unknown operator to not be found")
	}
}

func TestSupportedOps(t *testing.T) {
	r := NewRegistry()
	ops := r.SupportedOps()

	if len(ops) < 20 {
		t.Errorf("Expected at least 20 supported ops, got %d", len(ops))
	}
}

func TestRegisterCustomOp(t *testing.T) {
	r := NewRegistry()

	// Register custom operator
	r.Register("MyCustomOp", func(_ *Context, _ *Node, _ []*tensor.RawTensor) ([]*tensor.RawTensor, error) {
		return nil, nil
	})

	if _, ok := r.Get("MyCustomOp"); !ok {
		t.Error("Expected custom operator to be registered")
	}
}

func TestRegisterEqualOp(t *testing.T) {
	r := NewRegistry()

	if _, ok := r.Get("Equal"); !ok {
		t.Error("Expected Equal operator to be registered")
	}
}

func TestRegisterGreaterOp(t *testing.T) {
	r := NewRegistry()

	if _, ok := r.Get("Greater"); !ok {
		t.Error("Expected Greater operator to be registered")
	}
}

func TestRegisterGreaterOrEqualOp(t *testing.T) {
	r := NewRegistry()

	if _, ok := r.Get("GreaterOrEqual"); !ok {
		t.Error("Expected GreaterOrEqual operator to be registered")
	}
}

func TestRegisterLessOp(t *testing.T) {
	r := NewRegistry()

	if _, ok := r.Get("Less"); !ok {
		t.Error("Expected Less operator to be registered")
	}
}

func TestRegisterLessOrEqualOp(t *testing.T) {
	r := NewRegistry()

	if _, ok := r.Get("LessOrEqual"); !ok {
		t.Error("Expected LessOrEqual operator to be registered")
	}
}

func TestRegisterNotOp(t *testing.T) {
	r := NewRegistry()

	if _, ok := r.Get("Not"); !ok {
		t.Error("Expected Not operator to be registered")
	}
}

func TestRegisterAndOp(t *testing.T) {
	r := NewRegistry()

	if _, ok := r.Get("And"); !ok {
		t.Error("Expected And operator to be registered")
	}
}

func TestRegisterOrOp(t *testing.T) {
	r := NewRegistry()

	if _, ok := r.Get("Or"); !ok {
		t.Error("Expected Or operator to be registered")
	}
}

func TestRegisterXorOp(t *testing.T) {
	r := NewRegistry()

	if _, ok := r.Get("Xor"); !ok {
		t.Error("Expected Xor operator to be registered")
	}
}

func TestRegisterErfOp(t *testing.T) {
	r := NewRegistry()

	if _, ok := r.Get("Erf"); !ok {
		t.Error("Expected Erf operator to be registered")
	}
}

func TestRegisterLayerNormalizationOp(t *testing.T) {
	r := NewRegistry()

	if _, ok := r.Get("LayerNormalization"); !ok {
		t.Error("Expected LayerNormalization operator to be registered")
	}
}

func TestRegisterPowOp(t *testing.T) {
	r := NewRegistry()

	if _, ok := r.Get("Pow"); !ok {
		t.Error("Expected Pow operator to be registered")
	}
}

func TestRegisterReduceMeanOp(t *testing.T) {
	r := NewRegistry()

	if _, ok := r.Get("ReduceMean"); !ok {
		t.Error("Expected ReduceMean operator to be registered")
	}
}

func TestRegisterReduceMaxOp(t *testing.T) {
	r := NewRegistry()

	if _, ok := r.Get("ReduceMax"); !ok {
		t.Error("Expected ReduceMax operator to be registered")
	}
}

func TestRegisterReduceMinOp(t *testing.T) {
	r := NewRegistry()

	if _, ok := r.Get("ReduceMin"); !ok {
		t.Error("Expected ReduceMin operator to be registered")
	}
}

func TestRegisterMaxPoolOp(t *testing.T) {
	r := NewRegistry()

	if _, ok := r.Get("MaxPool"); !ok {
		t.Error("Expected MaxPool operator to be registered")
	}
}

func TestRegisterAveragePoolOp(t *testing.T) {
	r := NewRegistry()

	if _, ok := r.Get("AveragePool"); !ok {
		t.Error("Expected AveragePool operator to be registered")
	}
}

func TestRegisterConvOp(t *testing.T) {
	r := NewRegistry()

	if _, ok := r.Get("Conv"); !ok {
		t.Error("Expected Conv operator to be registered")
	}
}
