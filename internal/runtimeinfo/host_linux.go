package runtimeinfo

import (
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func hostResources() (*cpuTimes, *Space) {
	stat, _ := os.ReadFile("/proc/stat")
	mem, _ := os.ReadFile("/proc/meminfo")
	return linuxCPU(string(stat)), linuxMemory(string(mem))
}

func linuxCPU(stat string) *cpuTimes {
	line, _, _ := strings.Cut(stat, "\n")
	fields := strings.Fields(line)
	if len(fields) < 9 || fields[0] != "cpu" {
		return nil
	}
	var total, idle uint64
	// The guest fields are already counted in user and nice.
	for i, field := range fields[1:9] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return nil
		}
		total += value
		if i == 3 || i == 4 {
			idle += value
		}
	}
	return &cpuTimes{total: total, busy: total - idle}
}

func linuxMemory(mem string) *Space {
	var total, available uint64
	var measured bool
	for _, line := range strings.Split(mem, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || fields[2] != "kB" {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total, _ = strconv.ParseUint(fields[1], 10, 64)
		case "MemAvailable:":
			value, err := strconv.ParseUint(fields[1], 10, 64)
			available, measured = value, err == nil
		}
	}
	if total == 0 || !measured || available > total {
		return nil
	}
	return &Space{Total: total * 1024, Available: available * 1024}
}

func diskSpace(path string) *Space {
	var stat unix.Statfs_t
	if path == "" || unix.Statfs(path, &stat) != nil || stat.Blocks == 0 || stat.Bsize <= 0 {
		return nil
	}
	return &Space{Total: stat.Blocks * uint64(stat.Bsize), Available: stat.Bavail * uint64(stat.Bsize)}
}
