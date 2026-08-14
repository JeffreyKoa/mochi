package capability

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Probe 采集本机硬件信息（跨平台尽力而为；Windows 优先 PowerShell / nvidia-smi）。
func Probe() HardwareSnapshot {
	snap := HardwareSnapshot{
		CPUCores: runtime.NumCPU(),
		CPUModel: strings.TrimSpace(runtime.GOARCH + "/" + runtime.GOOS),
	}
	snap.RAMTotalMB, snap.RAMAvailableMB = probeMemoryMB()
	snap.DiskFreeMB = probeDiskFreeMB()
	snap.GPU = probeGPU()
	if model := probeCPUModel(); model != "" {
		snap.CPUModel = model
	}
	return snap
}

func probeCPUModel() string {
	switch runtime.GOOS {
	case "windows":
		out, err := exec.Command("powershell", "-NoProfile", "-Command",
			"(Get-CimInstance Win32_Processor | Select-Object -First 1 -ExpandProperty Name)").Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	case "linux":
		data, err := os.ReadFile("/proc/cpuinfo")
		if err != nil {
			return ""
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "model name") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					return strings.TrimSpace(parts[1])
				}
			}
		}
	}
	return ""
}

func probeMemoryMB() (totalMB, availMB int64) {
	switch runtime.GOOS {
	case "windows":
		out, err := exec.Command("powershell", "-NoProfile", "-Command",
			"$os = Get-CimInstance Win32_OperatingSystem; Write-Output $os.TotalVisibleMemorySize; Write-Output $os.FreePhysicalMemory").Output()
		if err != nil {
			return 0, 0
		}
		lines := strings.Fields(strings.TrimSpace(string(out)))
		if len(lines) >= 2 {
			totalKB, _ := strconv.ParseInt(lines[0], 10, 64)
			freeKB, _ := strconv.ParseInt(lines[1], 10, 64)
			return totalKB / 1024, freeKB / 1024
		}
	case "linux":
		data, err := os.ReadFile("/proc/meminfo")
		if err != nil {
			return 0, 0
		}
		var totalKB, availKB int64
		sc := bufio.NewScanner(bytes.NewReader(data))
		for sc.Scan() {
			line := sc.Text()
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			val, _ := strconv.ParseInt(fields[1], 10, 64)
			switch fields[0] {
			case "MemTotal:":
				totalKB = val
			case "MemAvailable:":
				availKB = val
			}
		}
		if totalKB > 0 {
			return totalKB / 1024, availKB / 1024
		}
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0, 0
		}
		bytesTotal, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		return bytesTotal / (1024 * 1024), 0
	}
	return 0, 0
}

func probeDiskFreeMB() int64 {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	switch runtime.GOOS {
	case "windows":
		root := filepath.VolumeName(wd)
		if root == "" {
			root = "C:"
		}
		out, err := exec.Command("powershell", "-NoProfile", "-Command",
			"(Get-PSDrive -Name ($env:SystemDrive.TrimEnd(':'))).Free / 1MB").Output()
		if err != nil {
			return 0
		}
		v, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		return v
	default:
		var st struct {
			Bavail uint64
			Bsize  uint64
		}
		// 使用 statfs 需 cgo；此处用 df 作为 fallback
		out, err := exec.Command("df", "-k", wd).Output()
		if err != nil {
			return 0
		}
		lines := strings.Split(string(out), "\n")
		if len(lines) < 2 {
			return 0
		}
		fields := strings.Fields(lines[1])
		if len(fields) < 4 {
			return 0
		}
		availKB, _ := strconv.ParseInt(fields[3], 10, 64)
		_ = st
		return availKB / 1024
	}
}

func probeGPU() GPUInfo {
	out, err := exec.Command("nvidia-smi",
		"--query-gpu=name,memory.total",
		"--format=csv,noheader,nounits").Output()
	if err != nil {
		return GPUInfo{Present: false}
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return GPUInfo{Present: false}
	}
	// 取第一块 GPU："NVIDIA GeForce RTX 3060, 12288"
	parts := strings.SplitN(line, "\n", 2)
	first := strings.TrimSpace(parts[0])
	comma := strings.LastIndex(first, ",")
	if comma < 0 {
		return GPUInfo{Present: true, Name: first}
	}
	name := strings.TrimSpace(first[:comma])
	vramStr := strings.TrimSpace(first[comma+1:])
	vramMB, _ := strconv.ParseInt(strings.Fields(vramStr)[0], 10, 64)
	return GPUInfo{
		Present:     true,
		Name:        name,
		VRAMTotalMB: vramMB,
	}
}
