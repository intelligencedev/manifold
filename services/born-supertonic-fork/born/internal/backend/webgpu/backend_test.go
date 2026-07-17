//go:build windows || linux

package webgpu

import (
	"testing"

	"github.com/born-ml/born/internal/tensor"
)

func TestIsAvailable(t *testing.T) {
	available := IsAvailable()
	t.Logf("WebGPU available: %v", available)
	// Note: This test doesn't fail if WebGPU is unavailable
	// It just reports the status
}

func TestListAdapters(t *testing.T) {
	adapters, err := ListAdapters()
	if err != nil {
		t.Logf("WebGPU not available: %v", err)
		t.Skip("WebGPU not available on this system")
	}

	for i, info := range adapters {
		t.Logf("Adapter %d:", i)
		t.Logf("  Name: %s", info.Name)
		t.Logf("  Vendor: %s", info.Vendor)
		t.Logf("  Driver: %s", info.Driver)
		t.Logf("  DriverInfo: %s", info.DriverInfo)
		t.Logf("  Backend: %v", info.Backend)
		t.Logf("  DeviceType: %v", info.DeviceType)
		t.Logf("  VendorID: 0x%04X", info.VendorID)
		t.Logf("  DeviceID: 0x%04X", info.DeviceID)
	}
}

func TestNew(t *testing.T) {
	backend, err := New()
	if err != nil {
		t.Logf("WebGPU not available: %v", err)
		t.Skip("WebGPU not available on this system")
	}
	defer backend.Release()

	// Check backend properties
	if backend.Name() == "" {
		t.Error("Backend name should not be empty")
	}
	t.Logf("Backend name: %s", backend.Name())

	if backend.Device() != tensor.WebGPU {
		t.Errorf("Expected device WebGPU, got %v", backend.Device())
	}

	info := backend.AdapterInfo()
	if info == nil {
		t.Log("Note: Adapter info unavailable (GetInfo API issue)")
	} else {
		t.Logf("Using GPU: %s (%s)", info.Name, info.Vendor)
	}
}

func TestBackendInterface(t *testing.T) {
	backend, err := New()
	if err != nil {
		t.Skip("WebGPU not available on this system")
	}
	defer backend.Release()

	// Verify it implements tensor.Backend interface
	var _ tensor.Backend = backend
}
