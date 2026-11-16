package metrics

import "time"

type ServiceState struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

type HostSample struct {
	HostID        string         `json:"host_id"`
	Hostname      string         `json:"hostname"`
	OS            string         `json:"os"`
	Arch          string         `json:"arch"`
	NumCPU        int            `json:"num_cpu"`
	UptimeSec     uint64         `json:"uptime_sec"`
	CPUPercent    float64        `json:"cpu_percent"`
	MemTotalBytes uint64         `json:"mem_total_bytes"`
	MemUsedBytes  uint64         `json:"mem_used_bytes"`
	MemPercent    float64        `json:"mem_percent"`
	DiskTotalBytes uint64        `json:"disk_total_bytes"`
	DiskUsedBytes uint64         `json:"disk_used_bytes"`
	DiskPercent   float64        `json:"disk_percent"`
	NetRxBytes    uint64         `json:"net_rx_bytes"`
	NetTxBytes    uint64         `json:"net_tx_bytes"`
	Services      []ServiceState `json:"services,omitempty"`
	Timestamp     time.Time      `json:"timestamp"`
}
