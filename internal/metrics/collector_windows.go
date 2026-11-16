//go:build windows

package metrics

import (
	"os"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGlobalMemoryStatus = kernel32.NewProc("GlobalMemoryStatusEx")
	procGetDiskFreeSpaceEx = kernel32.NewProc("GetDiskFreeSpaceExW")
	procGetTickCount64     = kernel32.NewProc("GetTickCount64")
	procGetSystemTimes     = kernel32.NewProc("GetSystemTimes")
)

func CollectSample(hostID string, services []string, diskPath string) (HostSample, error) {
	if diskPath == "" {
		diskPath = `C:\`
	}
	hostname, _ := os.Hostname()
	if hostID == "" {
		hostID = hostname
	}

	var mem memoryStatusEx
	mem.Length = uint32(unsafe.Sizeof(mem))
	_, _, _ = procGlobalMemoryStatus.Call(uintptr(unsafe.Pointer(&mem)))
	memUsed := mem.TotalPhys - mem.AvailPhys
	var memPct float64
	if mem.TotalPhys > 0 {
		memPct = float64(memUsed) / float64(mem.TotalPhys) * 100.0
	}

	var freeBytes, totalBytes uint64
	pathPtr, _ := syscall.UTF16PtrFromString(diskPath)
	_, _, _ = procGetDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		0,
	)
	var diskUsed uint64
	var diskPct float64
	if totalBytes > freeBytes {
		diskUsed = totalBytes - freeBytes
		diskPct = float64(diskUsed) / float64(totalBytes) * 100.0
	}

	tickMs, _, _ := procGetTickCount64.Call()
	cpuPct := sampleWindowsCPU()

	var svcStates []ServiceState
	for _, s := range services {
		svcStates = append(svcStates, ServiceState{Name: s, Active: false})
	}

	return HostSample{
		HostID:         hostID,
		Hostname:       hostname,
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
		NumCPU:         runtime.NumCPU(),
		UptimeSec:      uint64(tickMs) / 1000,
		CPUPercent:     float64(int(cpuPct*100+0.5)) / 100,
		MemTotalBytes:  mem.TotalPhys,
		MemUsedBytes:   memUsed,
		MemPercent:     float64(int(memPct*100+0.5)) / 100,
		DiskTotalBytes: totalBytes,
		DiskUsedBytes:  diskUsed,
		DiskPercent:    float64(int(diskPct*100+0.5)) / 100,
		Services:       svcStates,
		Timestamp:      time.Now().UTC(),
	}, nil
}

func sampleWindowsCPU() float64 {
	var idle1, kern1, user1 syscall.Filetime
	var idle2, kern2, user2 syscall.Filetime
	r1, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle1)),
		uintptr(unsafe.Pointer(&kern1)),
		uintptr(unsafe.Pointer(&user1)),
	)
	time.Sleep(150 * time.Millisecond)
	r2, _, _ := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle2)),
		uintptr(unsafe.Pointer(&kern2)),
		uintptr(unsafe.Pointer(&user2)),
	)
	if r1 == 0 || r2 == 0 {
		return 0
	}
	ft := func(f syscall.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }
	dIdle := ft(idle2) - ft(idle1)
	dTotal := (ft(kern2) - ft(kern1)) + (ft(user2) - ft(user1))
	if dTotal == 0 || dIdle > dTotal {
		return 0
	}
	return float64(dTotal-dIdle) / float64(dTotal) * 100.0
}
