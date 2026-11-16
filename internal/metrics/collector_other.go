//go:build !linux && !windows

package metrics

import (
	"os"
	"runtime"
	"time"
)

func CollectSample(hostID string, _ []string, _ string) (HostSample, error) {
	hostname, _ := os.Hostname()
	if hostID == "" {
		hostID = hostname
	}
	return HostSample{
		HostID:    hostID,
		Hostname:  hostname,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		NumCPU:    runtime.NumCPU(),
		Timestamp: time.Now().UTC(),
	}, nil
}
