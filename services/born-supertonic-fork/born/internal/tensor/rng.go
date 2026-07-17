package tensor

import (
	"math/rand"
	"sync"
)

// globalRNG is the package-level random number generator for tensor creation.
// When nil, falls back to math/rand global functions.
var (
	globalRNG   *rand.Rand
	globalRNGMu sync.Mutex
)

// SetSeed sets the global random seed for tensor random operations (Randn, Rand).
//
// Call this before creating random tensors to ensure reproducible results.
// Thread-safe.
func SetSeed(seed int64) {
	globalRNGMu.Lock()
	defer globalRNGMu.Unlock()
	globalRNG = rand.New(rand.NewSource(seed)) //nolint:gosec // deterministic for reproducibility
}

// ResetSeed clears the seeded RNG, reverting to Go's auto-seeded global rand.
func ResetSeed() {
	globalRNGMu.Lock()
	defer globalRNGMu.Unlock()
	globalRNG = nil
}

// RandFloat64 returns a random float64 from the seeded or global source.
func RandFloat64() float64 {
	globalRNGMu.Lock()
	defer globalRNGMu.Unlock()
	if globalRNG != nil {
		return globalRNG.Float64()
	}
	return rand.Float64() //nolint:gosec // ML uses math/rand intentionally
}
