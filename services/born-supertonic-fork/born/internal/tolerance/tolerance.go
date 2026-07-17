// Package tolerance provides helpers for approximate floating-point equality
// using absolute, relative, or combined tolerance checks.
package tolerance

import (
	"fmt"
	"math"
)

// TolType specifies the kind of tolerance check to apply.
type TolType int

const (
	// RelAbs uses both absolute and relative tolerance, passing if either check passes (default).
	RelAbs TolType = iota
	// Rel uses only relative tolerance.
	Rel
	// Abs uses only absolute tolerance.
	Abs
)

// Tolerance holds parameters for approximate floating-point equality checks.
//
// Rel is constrained to < 1.0 in validation checks.
// Abs is constrained to >= 0.0 in validation checks.
type Tolerance[T float32 | float64] struct {
	TolType TolType
	Abs     T // absolute tolerance
	Rel     T // relative tolerance factor
}

// NewDefaultTolerance returns a Tolerance with sensible defaults.
//
// The tolerance type is set to RelAbs with the following values:
//   - float32: rel = 1e-2, abs = 1e-5
//   - float64: rel = 1e-4, abs = 1e-5
func NewDefaultTolerance[T float32 | float64]() *Tolerance[T] {
	var rel T
	switch any(rel).(type) {
	case float32:
		rel = 1e-2
	case float64:
		rel = 1e-4
	}
	return &Tolerance[T]{
		TolType: RelAbs,
		Abs:     1e-5,
		Rel:     rel,
	}
}

// AssertApproxEqual checks whether a and b are approximately equal according
// to the given tolerance. Passes if the values are exactly equal, both +/-Inf, or both NaN.
// Returns an error if tol.Rel > 1.0 or tol.Abs < 0.0.
// Follows Burn's Tolerance.approx_eq implementation.
//
// Absolute tolerance: passes when |a-b| < tol.Abs.
// Relative tolerance: passes when |a-b| < tol.Rel * max(|a|, |b|).
// Combined (RelAbs): passes when |a-b| < max(tol.Rel * max(|a|, |b|), tol.Abs).
//
// Returns nil if the values are approximately equal, or an error describing
// which tolerance check failed.
func AssertApproxEqual[T float32 | float64](a, b T, tol *Tolerance[T]) error {
	err := validateTolerances(tol)
	if err != nil {
		return err
	}

	// handle exact equality, both +/-Inf
	if a == b {
		return nil
	}

	// handle both NaN
	if math.IsNaN(float64(a)) && math.IsNaN(float64(b)) {
		return nil
	}

	switch tol.TolType {
	case Abs:
		return checkAbs(a, b, tol.Abs)
	case Rel:
		return checkRel(a, b, tol.Rel)
	case RelAbs:
		return checkRelAbs(a, b, tol.Rel, tol.Abs)
	}
	return fmt.Errorf("unexpected TolType: %d", tol.TolType)
}

// AssertAllApproxEqual checks a and b element-wise for approximate equality.
//
// Returns an error describing the first element that fails, or a length
// mismatch error if the slices differ in length.
func AssertAllApproxEqual[T float32 | float64](a, b []T, tol *Tolerance[T]) error {
	if len(a) != len(b) {
		return fmt.Errorf("slices differ in length: len(a)=%d; len(b)=%d", len(a), len(b))
	}
	for i := range a {
		if err := AssertApproxEqual(a[i], b[i], tol); err != nil {
			return fmt.Errorf("element[%d]: %w", i, err)
		}
	}
	return nil
}

// checkRel is a helper to compare two values using relative tolerance.
//
// Returns nil if |a-b| < rel * max(|a|, |b|), an error otherwise.
func checkRel[T float32 | float64](a, b, rel T) error {
	absDiff := math.Abs(float64(a - b))
	relTol := float64(rel) * math.Max(math.Abs(float64(a)), math.Abs(float64(b)))
	// handles NaN case, if a or b is NaN then absDiff compared to anything will be false
	if absDiff < relTol {
		return nil
	}
	return fmt.Errorf("relative tolerance failure: %f >= %f", absDiff, relTol)
}

// checkAbs is a helper to compare two values using absolute tolerance.
//
// Returns nil if |a-b| < abs, an error otherwise.
func checkAbs[T float32 | float64](a, b, abs T) error {
	absDiff := math.Abs(float64(a - b))
	// handles NaN case, if a or b is NaN then absDiff compared to anything will be false
	if absDiff < float64(abs) {
		return nil
	}
	return fmt.Errorf("absolute tolerance failure: %f >= %f", absDiff, abs)
}

// checkRelAbs is a helper to compare two values using relative and
// absolute tolerance.
//
// Returns nil if |a-b| < max(rel * max(|a|, |b|), abs).
func checkRelAbs[T float32 | float64](a, b, rel, abs T) error {
	absDiff := math.Abs(float64(a - b))
	relTol := float64(rel) * math.Max(math.Abs(float64(a)), math.Abs(float64(b)))
	moreLenientTol := math.Max(float64(abs), relTol)

	// handles NaN case, if a or b is NaN then absDiff compared to anything will be false
	if absDiff < moreLenientTol {
		return nil
	}
	return fmt.Errorf("relAbs tolerance failure: %f >= %f", absDiff, moreLenientTol)
}

// validateTolerances returns an error if tol.Rel > 1.0 or tol.Abs < 0.0, nil otherwise.
func validateTolerances[T float32 | float64](tol *Tolerance[T]) error {
	if tol.Rel > 1.0 {
		return fmt.Errorf("relative tolerance cannot be >1.0: %f", tol.Rel)
	}
	if tol.Abs < 0.0 {
		return fmt.Errorf("absolute tolerance cannot be <0.0: %f", tol.Abs)
	}
	return nil
}
