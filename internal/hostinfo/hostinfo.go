// Package hostinfo provides utilities for retrieving host system information,
// including OS, architecture, CPU, memory, and GPU details.
package hostinfo

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/jaypipes/ghw"
	"github.com/shirou/gopsutil/mem"
)

// HostInfo represents the system's host information, including OS, architecture, CPU, memory, and GPU details.
type HostInfo struct {
	OS     string    `json:"os"`
	Arch   string    `json:"arch"`
	CPUs   int       `json:"cpus"`
	Memory Memory    `json:"memory"`
	GPUs   []GPUInfo `json:"gpus"`
}

// Memory represents the total memory available on the system.
type Memory struct {
	Total uint64 `json:"total"`
}

type GPUInfo struct {
	Model              string
	TotalNumberOfCores string
	MetalSupport       string
}

// GetHostInfo retrieves information about the host system, including OS, architecture, CPU, memory, and GPU details.
func GetHostInfo() (HostInfo, error) {
	hostInfo := HostInfo{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
		CPUs: runtime.NumCPU(),
	}

	if err := populateMemoryInfo(&hostInfo); err != nil {
		return HostInfo{}, fmt.Errorf("failed to retrieve memory info: %w", err)
	}

	if err := populateGPUInfo(&hostInfo); err != nil {
		return HostInfo{}, fmt.Errorf("failed to retrieve GPU info: %w", err)
	}

	return hostInfo, nil
}

// populateMemoryInfo populates the memory information in the HostInfo struct.
func populateMemoryInfo(hostInfo *HostInfo) error {
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return err
	}
	hostInfo.Memory = Memory{Total: vmStat.Total}
	return nil
}

// populateGPUInfo populates the GPU information in the HostInfo struct.
func populateGPUInfo(hostInfo *HostInfo) error {
	switch runtime.GOOS {
	case "darwin":
		gpus, err := getMacOSGPUInfo()
		if err != nil {
			return fmt.Errorf("error getting macOS GPU info: %w", err)
		}
		hostInfo.GPUs = gpus

	case "linux", "windows":
		gpu, err := ghw.GPU()
		if err != nil {
			return fmt.Errorf("error getting GPU info: %w", err)
		}
		for _, card := range gpu.GraphicsCards {
			hostInfo.GPUs = append(hostInfo.GPUs, GPUInfo{
				Model: card.DeviceInfo.Product.Name,
			})
		}
	}
	return nil
}

// getMacOSGPUInfo retrieves GPU information specific to macOS systems.
func getMacOSGPUInfo() ([]GPUInfo, error) {
	cmd := exec.Command("system_profiler", "SPDisplaysDataType")

	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return parseMacOSGPUInfo(out.String())
}

// parseMacOSGPUInfo parses the output of the macOS system_profiler command to extract GPU information.
func parseMacOSGPUInfo(input string) ([]GPUInfo, error) {
	lines := strings.Split(input, "\n")
	var gpus []GPUInfo
	var current GPUInfo
	anyFieldSet := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "Chipset Model:") {
			if anyFieldSet {
				gpus = append(gpus, current)
				current = GPUInfo{}
				anyFieldSet = false
			}
			current.Model = strings.TrimSpace(strings.TrimPrefix(line, "Chipset Model:"))
			anyFieldSet = true
		} else if strings.HasPrefix(line, "Total Number of Cores:") {
			current.TotalNumberOfCores = strings.TrimSpace(strings.TrimPrefix(line, "Total Number of Cores:"))
			anyFieldSet = true
		} else if strings.HasPrefix(line, "Metal:") {
			current.MetalSupport = strings.TrimSpace(strings.TrimPrefix(line, "Metal:"))
			anyFieldSet = true
		}
	}
	if anyFieldSet || (current.Model == "" && current.TotalNumberOfCores == "" && current.MetalSupport == "" && len(lines) > 0) {
		gpus = append(gpus, current)
	}
	return gpus, nil
}
