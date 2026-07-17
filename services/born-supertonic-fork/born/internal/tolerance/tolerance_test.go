package tolerance

import (
	"math"
	"testing"
)

// TestAssertApproxEqual_Absolute exercises AssertApproxEqual in Abs mode
// across boundaries, zero values, negatives, and values crossing zero.
func TestAssertApproxEqual_Absolute(t *testing.T) {
	tests := []struct {
		name string
		a, b float64
		abs  float64
		want bool
	}{
		{name: "exact match", a: 1.0, b: 1.0, abs: 0.1, want: true},
		{name: "within tolerance", a: 1.0, b: 1.05, abs: 0.1, want: true},
		{name: "at tolerance boundary", a: 1.0, b: 1.099999, abs: 0.1, want: true},
		{name: "just over tolerance", a: 1.0, b: 1.100001, abs: 0.1, want: false},
		{name: "zero diff", a: 0.0, b: 0.0, abs: 1e-9, want: true},
		{name: "both zero and no tolerance", a: 0.0, b: 0.0, abs: 0.0, want: true},
		{name: "negative within tolerance", a: -1.0, b: -1.05, abs: 0.1, want: true},
		{name: "negative over tolerance", a: -1.0, b: -1.2, abs: 0.1, want: false},
		{name: "crossing zero within", a: -0.04, b: 0.04, abs: 0.1, want: true},
		{name: "crossing zero outside", a: -0.06, b: 0.06, abs: 0.1, want: false},
		{name: "abs less than 0.0", a: 1.0, b: 1.0, abs: -0.1, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tol := &Tolerance[float64]{TolType: Abs, Abs: tt.abs}
			err := AssertApproxEqual(tt.a, tt.b, tol)
			if (err == nil) != tt.want {
				t.Errorf("AssertApproxEqual(%v, %v, abs=%v): got err=%v, want success=%v",
					tt.a, tt.b, tt.abs, err, tt.want)
			}
		})
	}
}

// TestAssertApproxEqual_Relative exercises AssertApproxEqual in Rel mode
// across boundaries, large/small values, negatives, and zero tolerance.
func TestAssertApproxEqual_Relative(t *testing.T) {
	tests := []struct {
		name string
		a, b float64
		rel  float64
		want bool
	}{
		{name: "exact match", a: 1.0, b: 1.0, rel: 0.1, want: true},
		{name: "within relative tolerance", a: 1.0, b: 1.05, rel: 0.1, want: true},
		{name: "over relative tolerance", a: 1.0, b: 1.3, rel: 0.1, want: false},
		{name: "large values within", a: 1e6, b: 1.05e6, rel: 0.1, want: true},
		{name: "large values outside", a: 1e6, b: 1.3e6, rel: 0.1, want: false},
		{name: "values near zero within", a: 1e-8, b: 1.05e-8, rel: 0.1, want: true},
		{name: "values near zero outside", a: 1e-8, b: 1.3e-8, rel: 0.1, want: false},
		{name: "negative values within", a: -1.0, b: -1.05, rel: 0.1, want: true},
		{name: "negative values outside", a: -1.0, b: -1.3, rel: 0.1, want: false},
		{name: "zero rel tolerance exact match", a: 1.0, b: 1.0, rel: 0.0, want: true},
		{name: "rel greater than 1.0", a: 1.0, b: 1.0, rel: 1.1, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tol := &Tolerance[float64]{TolType: Rel, Rel: tt.rel}
			err := AssertApproxEqual(tt.a, tt.b, tol)
			if (err == nil) != tt.want {
				t.Errorf("AssertApproxEqual(%v, %v, rel=%v): got err=%v, want success=%v",
					tt.a, tt.b, tt.rel, err, tt.want)
			}
		})
	}
}

// TestAssertApproxEqual_RelAbs exercises AssertApproxEqual in RelAbs
// (combined) mode, verifying that the looser of the two tolerances applies.
func TestAssertApproxEqual_RelAbs(t *testing.T) {
	tests := []struct {
		name        string
		a, b        float64
		rel, abs    float64
		want        bool
		description string
	}{
		{name: "exact match", a: 1.0, b: 1.0, rel: 0.1, abs: 0.1, want: true},
		{name: "within relative tolerance", a: 1.0, b: 1.05, rel: 0.1, abs: 1e-9, want: true},
		{name: "within absolute tolerance", a: 1.0, b: 1.05, rel: 1e-9, abs: 0.1, want: true},
		{name: "over both tolerances", a: 1.0, b: 1.3, rel: 0.1, abs: 0.1, want: false},
		{name: "large values fail rel pass abs", a: 1e6, b: 1.01e6, rel: 1e-9, abs: 0.1, want: false},
		{name: "tight rel loose abs", a: 1.0, b: 1.01, rel: 1e-9, abs: 0.1, want: true},
		{name: "abs less than 0.0", a: 1.0, b: 1.0, abs: -0.1, want: false},
		{name: "rel greater than 1.0", a: 1.0, b: 1.0, rel: 1.1, want: false},
		{name: "within relative tolerance (negative)", a: -1.0, b: -1.05, rel: 0.1, abs: 1e-9, want: true},
		{name: "within absolute tolerance (negative)", a: -1.0, b: -1.05, rel: 1e-9, abs: 0.1, want: true},
		{name: "over both tolerances (negative)", a: -1.0, b: -1.3, rel: 0.1, abs: 0.1, want: false},
		{name: "large values fail rel pass abs (negative)", a: -1e6, b: -1.01e6, rel: 1e-9, abs: 0.1, want: false},
		{name: "tight rel loose abs (negative)", a: -1.0, b: -1.01, rel: 1e-9, abs: 0.1, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tol := &Tolerance[float64]{TolType: RelAbs, Rel: tt.rel, Abs: tt.abs}
			err := AssertApproxEqual(tt.a, tt.b, tol)
			if (err == nil) != tt.want {
				t.Errorf("AssertApproxEqual(%v, %v, rel=%v, abs=%v): got err=%v, want success=%v",
					tt.a, tt.b, tt.rel, tt.abs, err, tt.want)
			}
		})
	}
}

// TestAssertApproxEqual_WithDefaultTolerance verifies that the default
// tolerances accept close values for both float32 and float64.
func TestAssertApproxEqual_WithDefaultTolerance(t *testing.T) {
	tests := []struct {
		name string
		a, b float64
		is32 bool
	}{
		{name: "float32 identical", a: 1.0, b: 1.0, is32: true},
		{name: "float32 close", a: 1.0, b: 1.005, is32: true},
		{name: "float64 identical", a: 1.0, b: 1.0, is32: false},
		{name: "float64 close", a: 1.0, b: 1.00005, is32: false},
	}

	tol32 := NewDefaultTolerance[float32]()
	tol64 := NewDefaultTolerance[float64]()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.is32 {
				err = AssertApproxEqual(float32(tt.a), float32(tt.b), tol32)
			} else {
				err = AssertApproxEqual(tt.a, tt.b, tol64)
			}
			if err != nil {
				t.Errorf("expected success, got: %v", err)
			}
		})
	}
}

// TestAssertApproxEqual_Special covers NaN and Inf inputs.
func TestAssertApproxEqual_Special(t *testing.T) {
	tol := &Tolerance[float64]{TolType: Abs, Abs: 0.1}

	tests := []struct {
		name string
		a, b float64
		want bool
	}{
		{name: "a is NaN", a: math.NaN(), b: 1.0, want: false},
		{name: "b is NaN", a: 1.0, b: math.NaN(), want: false},
		{name: "both NaN", a: math.NaN(), b: math.NaN(), want: true},
		{name: "a is Inf", a: math.Inf(1), b: 1.0, want: false},
		{name: "b is Inf", a: 1.0, b: math.Inf(1), want: false},
		{name: "both Inf", a: math.Inf(1), b: math.Inf(1), want: true},
		{name: "a is -Inf", a: math.Inf(-1), b: 1.0, want: false},
		{name: "b is -Inf", a: 1.0, b: math.Inf(-1), want: false},
		{name: "both -Inf", a: math.Inf(-1), b: math.Inf(-1), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := AssertApproxEqual(tt.a, tt.b, tol)
			if tt.want {
				if err != nil {
					t.Errorf("expected success, got: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error for %s input", tt.name)
				}
			}
		})
	}
}

// TestAssertAllApproxEqual exercises AssertAllApproxEqual over
// identical slices, mismatched elements, empty slices, and
// length mismatches.
func TestAssertAllApproxEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b []float64
		abs  float64
		want bool
	}{
		{name: "all equal", a: []float64{1.0, 2.0, 3.0}, b: []float64{1.05, 2.05, 3.05}, abs: 0.1, want: true},
		{name: "element outside tolerance", a: []float64{1.0, 2.0, 3.0}, b: []float64{1.05, 2.2, 3.05}, abs: 0.1, want: false},
		{name: "single element slice", a: []float64{math.Pi}, b: []float64{math.Pi}, abs: 1e-9, want: true},
		{name: "empty slices", a: []float64{}, b: []float64{}, abs: 0.1, want: true},
		{name: "length mismatch", a: []float64{1.0}, b: []float64{1.0, 2.0}, abs: 0.1, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tol := &Tolerance[float64]{TolType: Abs, Abs: tt.abs}
			err := AssertAllApproxEqual(tt.a, tt.b, tol)
			if tt.want {
				if err != nil {
					t.Errorf("expected success, got: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected failure")
				}
			}
		})
	}
}
