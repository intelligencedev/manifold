//go:build !race

package cpu

// raceEnabled reports whether the test binary was built with -race. See
// race_on_test.go for why alloc-counting tests consult it.
const raceEnabled = false
