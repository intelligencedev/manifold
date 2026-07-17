//go:build race

package cpu

// raceEnabled reports whether the test binary was built with -race. The race
// detector adds shadow allocations that make testing.AllocsPerRun unreliable, so
// alloc-counting tests skip when it is on (independently of -short).
const raceEnabled = true
