//go:build linux

package metrics

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type cpuTimes struct {
	total uint64
	idle  uint64
}

func CollectSample(hostID string, services []string, diskPath string) (HostSample, error) {
	if diskPath == "" {
		diskPath = "/"
	}
	hostname, _ := os.Hostname()
	if hostID == "" {
		hostID = hostname
	}

	c1, err1 := readProcCPU()
	time.Sleep(150 * time.Millisecond)
	c2, err2 := readProcCPU()
	var cpuPct float64
	if err1 == nil && err2 == nil && c2.total > c1.total {
		dTotal := float64(c2.total - c1.total)
		dIdle := float64(c2.idle - c1.idle)
		cpuPct = ((dTotal - dIdle) / dTotal) * 100.0
	}

	memTotal, memUsed := readProcMeminfo()
	var memPct float64
	if memTotal > 0 {
		memPct = (float64(memUsed) / float64(memTotal)) * 100.0
	}

	var st syscall.Statfs_t
	var diskTotal, diskUsed uint64
	var diskPct float64
	if err := syscall.Statfs(diskPath, &st); err == nil {
		diskTotal = st.Blocks * uint64(st.Bsize)
		free := st.Bavail * uint64(st.Bsize)
		if diskTotal > free {
			diskUsed = diskTotal - free
			diskPct = (float64(diskUsed) / float64(diskTotal)) * 100.0
		}
	}

	rx, tx := readProcNetDev()
	uptime := readProcUptime()
	svcStates := checkLinuxProcesses(services)

	return HostSample{
		HostID:         hostID,
		Hostname:       hostname,
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
		NumCPU:         runtime.NumCPU(),
		UptimeSec:      uptime,
		CPUPercent:     round2(cpuPct),
		MemTotalBytes:  memTotal,
		MemUsedBytes:   memUsed,
		MemPercent:     round2(memPct),
		DiskTotalBytes: diskTotal,
		DiskUsedBytes:  diskUsed,
		DiskPercent:    round2(diskPct),
		NetRxBytes:     rx,
		NetTxBytes:     tx,
		Services:       svcStates,
		Timestamp:      time.Now().UTC(),
	}, nil
}

func readProcCPU() (cpuTimes, error) {
	raw, err := os.ReadFile("/proc/stat")
	if err != nil {
		return cpuTimes{}, err
	}
	return parseProcStat(string(raw))
}

func parseProcStat(content string) (cpuTimes, error) {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) < 5 {
				break
			}
			var total, idle uint64
			for i, f := range fields[1:] {
				v, _ := strconv.ParseUint(f, 10, 64)
				total += v
				if i == 3 || i == 4 { // idle + iowait
					idle += v
				}
			}
			return cpuTimes{total: total, idle: idle}, nil
		}
	}
	return cpuTimes{}, os.ErrInvalid
}

func readProcMeminfo() (uint64, uint64) {
	raw, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	return parseMeminfo(string(raw))
}

func parseMeminfo(content string) (uint64, uint64) {
	var totalKB, availKB uint64
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, _ := strconv.ParseUint(fields[1], 10, 64)
		switch fields[0] {
		case "MemTotal:":
			totalKB = v
		case "MemAvailable:":
			availKB = v
		}
	}
	total := totalKB * 1024
	avail := availKB * 1024
	if total > avail {
		return total, total - avail
	}
	return total, 0
}

func readProcNetDev() (uint64, uint64) {
	raw, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	return parseNetDev(string(raw))
}

func parseNetDev(content string) (uint64, uint64) {
	var rxTotal, txTotal uint64
	for _, line := range strings.Split(content, "\n") {
		colon := strings.Index(line, ":")
		if colon == -1 {
			continue
		}
		iface := strings.TrimSpace(line[:colon])
		if iface == "lo" {
			continue
		}
		fields := strings.Fields(line[colon+1:])
		if len(fields) >= 9 {
			rx, _ := strconv.ParseUint(fields[0], 10, 64)
			tx, _ := strconv.ParseUint(fields[8], 10, 64)
			rxTotal += rx
			txTotal += tx
		}
	}
	return rxTotal, txTotal
}

func readProcUptime() uint64 {
	raw, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(raw))
	if len(fields) == 0 {
		return 0
	}
	f, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || f < 0 {
		return 0
	}
	return uint64(f)
}

func checkLinuxProcesses(services []string) []ServiceState {
	if len(services) == 0 {
		return nil
	}
	entries, err := filepath.Glob("/proc/[0-9]*/comm")
	running := make(map[string]bool)
	if err == nil {
		for _, p := range entries {
			if b, err := os.ReadFile(p); err == nil {
				running[strings.TrimSpace(string(b))] = true
			}
		}
	}
	out := make([]ServiceState, 0, len(services))
	for _, s := range services {
		out = append(out, ServiceState{Name: s, Active: running[s]})
	}
	return out
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
