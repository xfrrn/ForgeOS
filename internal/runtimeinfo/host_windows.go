package runtimeinfo

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32           = windows.NewLazySystemDLL("kernel32.dll")
	getSystemTimes     = kernel32.NewProc("GetSystemTimes")
	globalMemoryStatus = kernel32.NewProc("GlobalMemoryStatusEx")
)

func hostResources() (*cpuTimes, *Space) {
	var idle, kernel, user uint64
	var cpu *cpuTimes
	ok, _, _ := getSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
	if ok != 0 && kernel >= idle {
		cpu = &cpuTimes{total: kernel + user, busy: kernel - idle + user}
	}
	// MEMORYSTATUSEX starts with a size and load DWORD followed by seven DWORDLONGs.
	memory := struct {
		Size, Load               uint32
		Total, Available         uint64
		PageTotal, PageAvailable uint64
		VirtualTotal, Virtual    uint64
		ExtendedVirtual          uint64
	}{}
	memory.Size = uint32(unsafe.Sizeof(memory))
	ok, _, _ = globalMemoryStatus.Call(uintptr(unsafe.Pointer(&memory)))
	if ok == 0 || memory.Total == 0 || memory.Available > memory.Total {
		return cpu, nil
	}
	return cpu, &Space{Total: memory.Total, Available: memory.Available}
}

func diskSpace(path string) *Space {
	if path == "" {
		return nil
	}
	dir, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil
	}
	var available, total, free uint64
	if windows.GetDiskFreeSpaceEx(dir, &available, &total, &free) != nil || total == 0 || available > total {
		return nil
	}
	return &Space{Total: total, Available: available}
}
