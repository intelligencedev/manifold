// Package main provides utilities for retrieving host system information, including OS, architecture, CPU, memory, and GPU details.
package main

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

// GPUInfo represents information about a GPU, including its model, number of cores, and Metal support.
type GPUInfo struct {
	Model              string `json:"model"`
	TotalNumberOfCores string `json:"total_number_of_cores"`
	MetalSupport       string `json:"metal_support"`
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
	var gpus []GPUInfo
	var currentGPU GPUInfo

	for _, line := range strings.Split(input, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Chipset Model") {
			if currentGPU.Model != "" {
				gpus = append(gpus, currentGPU)
				currentGPU = GPUInfo{}
			}
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				currentGPU.Model = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(line, "Total Number of Cores") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				currentGPU.TotalNumberOfCores = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(line, "Metal") {
			if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
				currentGPU.MetalSupport = strings.TrimSpace(parts[1])
			}
		}
	}

	if currentGPU.Model != "" {
		gpus = append(gpus, currentGPU)
	}

	return gpus, nil
}
