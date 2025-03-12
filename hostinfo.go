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

type HostInfo struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	CPUs   int    `json:"cpus"`
	Memory struct {
		Total uint64 `json:"total"`
	} `json:"memory"`
	GPUs []GPUInfo `json:"gpus"`
}

type GPUInfo struct {
	Model              string `json:"model"`
	TotalNumberOfCores string `json:"total_number_of_cores"`
	MetalSupport       string `json:"metal_support"`
}

func GetHostInfo() (HostInfo, error) {
	hostInfo := HostInfo{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
		CPUs: runtime.NumCPU(),
	}

	vmStat, _ := mem.VirtualMemory()
	hostInfo.Memory.Total = vmStat.Total

	switch runtime.GOOS {
	case "darwin":
		gpus, err := getMacOSGPUInfo()
		if err != nil {
			fmt.Printf("Error getting GPU info: %v\n", err)
		} else {
			hostInfo.GPUs = append(hostInfo.GPUs, gpus)
		}

	case "linux", "windows":
		gpu, err := ghw.GPU()
		if err != nil {
			fmt.Printf("Error getting GPU info: %v\n", err)
		} else {
			for _, card := range gpu.GraphicsCards {
				gpuInfo := GPUInfo{
					Model: card.DeviceInfo.Product.Name,
				}
				hostInfo.GPUs = append(hostInfo.GPUs, gpuInfo)
			}
		}
	}

	return hostInfo, nil
}

func getMacOSGPUInfo() (GPUInfo, error) {
	cmd := exec.Command("system_profiler", "SPDisplaysDataType")

	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return GPUInfo{}, err
	}

	return parseGPUInfo(out.String())
}

func parseGPUInfo(input string) (GPUInfo, error) {
	gpuInfo := GPUInfo{}

	for _, line := range strings.Split(input, "\n") {
		if strings.Contains(line, "Chipset Model") {
			gpuInfo.Model = strings.TrimSpace(strings.Split(line, ":")[1])
		}
		if strings.Contains(line, "Total Number of Cores") {
			gpuInfo.TotalNumberOfCores = strings.TrimSpace(strings.Split(line, ":")[1])
		}
		if strings.Contains(line, "Metal") {
			gpuInfo.MetalSupport = strings.TrimSpace(strings.Split(line, ":")[1])
		}
	}

	return gpuInfo, nil
}
