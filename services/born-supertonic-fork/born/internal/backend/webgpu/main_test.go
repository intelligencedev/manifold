//go:build windows || linux

package webgpu

import (
	"flag"
	"os"
	"testing"
)

var computeAvailable bool

func TestMain(m *testing.M) {
	// flag.Parse must be called before testing.Short() is readable.
	// The go test runner normally does this, but TestMain executes
	// before the runner's own initialization completes.
	flag.Parse()

	// Skip the GPU availability probe when running with -short.
	// GitHub Actions Windows runners have no real GPU: calling wgpu.CreateInstance
	// with the Vulkan backend on a driverless machine can hang or raise an access
	// violation inside wgpu_native.dll before any t.Skip() gets a chance to run.
	// With -short the probe is skipped entirely and computeAvailable stays false,
	// causing every GPU test to t.Skip gracefully instead of crashing the binary.
	if !testing.Short() {
		computeAvailable = IsAvailable()
	}
	os.Exit(m.Run())
}
