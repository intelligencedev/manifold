package hostinfo

import (
	"testing"
)

func TestParseMacOSGPUInfo_SingleGPU(t *testing.T) {
	input := `Chipset Model: Intel UHD Graphics 630
Total Number of Cores: 24
Metal: Supported, feature set macOS GPUFamily2 v1
`
	gpus, err := parseMacOSGPUInfo(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gpus) != 1 {
		t.Fatalf("expected 1 GPU, got %d", len(gpus))
	}
	gpu := gpus[0]
	if gpu.Model != "Intel UHD Graphics 630" {
		t.Errorf("unexpected model: %s", gpu.Model)
	}
	if gpu.TotalNumberOfCores != "24" {
		t.Errorf("unexpected cores: %s", gpu.TotalNumberOfCores)
	}
	if gpu.MetalSupport != "Supported, feature set macOS GPUFamily2 v1" {
		t.Errorf("unexpected metal support: %s", gpu.MetalSupport)
	}
}

func TestParseMacOSGPUInfo_MultipleGPUs(t *testing.T) {
	input := `Chipset Model: GPU A
Total Number of Cores: 10
Metal: Unsupported
Chipset Model: GPU B
Total Number of Cores: 20
Metal: Supported
`
	gpus, err := parseMacOSGPUInfo(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gpus) != 2 {
		t.Fatalf("expected 2 GPUs, got %d", len(gpus))
	}
	if gpus[0].Model != "GPU A" || gpus[0].TotalNumberOfCores != "10" || gpus[0].MetalSupport != "Unsupported" {
		t.Errorf("first GPU data mismatch: %+v", gpus[0])
	}
	if gpus[1].Model != "GPU B" || gpus[1].TotalNumberOfCores != "20" || gpus[1].MetalSupport != "Supported" {
		t.Errorf("second GPU data mismatch: %+v", gpus[1])
	}
}
func TestParseMacOSGPUInfo_MalformedLines(t *testing.T) {
	input := `Chipset Model Intel Graphics
Total Number of Cores
Metal`
	gpus, err := parseMacOSGPUInfo(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gpus) != 1 {
		t.Fatalf("expected 1 GPU, got %d", len(gpus))
	}
}
