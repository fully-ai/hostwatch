package server

import (
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fully-ai/hostwatch/internal/metrics"
)

//go:embed index.html
var indexHTML []byte

type Thresholds struct {
	MaxCPUPercent  float64 `json:"max_cpu_percent"`
	MaxMemPercent  float64 `json:"max_mem_percent"`
	MaxDiskPercent float64 `json:"max_disk_percent"`
}

type Alert struct {
	HostID      string    `json:"host_id"`
	Metric      string    `json:"metric"`
	Value       float64   `json:"value"`
	Threshold   float64   `json:"threshold"`
	Message     string    `json:"message"`
	TriggeredAt time.Time `json:"triggered_at"`
}

type Store struct {
	mu          sync.RWMutex
	maxSamples  int
	agentToken  string
	thresholds  Thresholds
	history     map[string][]metrics.HostSample
	alerts      []Alert
}

func NewStore(agentToken string, thresholds Thresholds, maxSamples int) *Store {
	if maxSamples <= 0 {
		maxSamples = 120
	}
	return &Store{
		maxSamples: maxSamples,
		agentToken: agentToken,
		thresholds: thresholds,
		history:    make(map[string][]metrics.HostSample),
	}
}

func (s *Store) Ingest(sample metrics.HostSample) {
	if sample.Timestamp.IsZero() {
		sample.Timestamp = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	buf := append(s.history[sample.HostID], sample)
	if len(buf) > s.maxSamples {
		buf = buf[len(buf)-s.maxSamples:]
	}
	s.history[sample.HostID] = buf

	if s.thresholds.MaxCPUPercent > 0 && sample.CPUPercent > s.thresholds.MaxCPUPercent {
		s.alerts = append(s.alerts, Alert{
			HostID:      sample.HostID,
			Metric:      "cpu_percent",
			Value:       sample.CPUPercent,
			Threshold:   s.thresholds.MaxCPUPercent,
			Message:     fmt.Sprintf("%s CPU at %.1f%% (threshold %.1f%%)", sample.HostID, sample.CPUPercent, s.thresholds.MaxCPUPercent),
			TriggeredAt: sample.Timestamp,
		})
	}
	if s.thresholds.MaxMemPercent > 0 && sample.MemPercent > s.thresholds.MaxMemPercent {
		s.alerts = append(s.alerts, Alert{
			HostID:      sample.HostID,
			Metric:      "mem_percent",
			Value:       sample.MemPercent,
			Threshold:   s.thresholds.MaxMemPercent,
			Message:     fmt.Sprintf("%s Memory at %.1f%% (threshold %.1f%%)", sample.HostID, sample.MemPercent, s.thresholds.MaxMemPercent),
			TriggeredAt: sample.Timestamp,
		})
	}
	if s.thresholds.MaxDiskPercent > 0 && sample.DiskPercent > s.thresholds.MaxDiskPercent {
		s.alerts = append(s.alerts, Alert{
			HostID:      sample.HostID,
			Metric:      "disk_percent",
			Value:       sample.DiskPercent,
			Threshold:   s.thresholds.MaxDiskPercent,
			Message:     fmt.Sprintf("%s Disk at %.1f%% (threshold %.1f%%)", sample.HostID, sample.DiskPercent, s.thresholds.MaxDiskPercent),
			TriggeredAt: sample.Timestamp,
		})
	}
	for _, svc := range sample.Services {
		if !svc.Active {
			s.alerts = append(s.alerts, Alert{
				HostID:      sample.HostID,
				Metric:      "service:" + svc.Name,
				Value:       0,
				Threshold:   1,
				Message:     fmt.Sprintf("%s service %q is inactive", sample.HostID, svc.Name),
				TriggeredAt: sample.Timestamp,
			})
		}
	}
}

func (s *Store) LatestHosts() []metrics.HostSample {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]metrics.HostSample, 0, len(s.history))
	for _, samples := range s.history {
		if len(samples) > 0 {
			out = append(out, samples[len(samples)-1])
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].HostID < out[j].HostID })
	return out
}

func (s *Store) HostHistory(hostID string) []metrics.HostSample {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.history[hostID]
	out := make([]metrics.HostSample, len(src))
	copy(out, src)
	return out
}

func (s *Store) Alerts() []Alert {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Alert, len(s.alerts))
	copy(out, s.alerts)
	return out
}

func (s *Store) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/ingest", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if s.agentToken != "" {
			auth := r.Header.Get("Authorization")
			token := strings.TrimPrefix(auth, "Bearer ")
			if !strings.HasPrefix(auth, "Bearer ") || subtle.ConstantTimeCompare([]byte(token), []byte(s.agentToken)) != 1 {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		var sample metrics.HostSample
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<10)).Decode(&sample); err != nil || sample.HostID == "" {
			http.Error(w, "invalid sample payload", http.StatusBadRequest)
			return
		}
		s.Ingest(sample)
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	})
	mux.HandleFunc("/api/hosts", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"hosts": s.LatestHosts()})
	})
	mux.HandleFunc("/api/hosts/", func(w http.ResponseWriter, r *http.Request) {
		hostID := strings.TrimPrefix(r.URL.Path, "/api/hosts/")
		hostID = strings.TrimSuffix(hostID, "/history")
		if hostID == "" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"host_id": hostID, "samples": s.HostHistory(hostID)})
	})
	mux.HandleFunc("/api/alerts", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"alerts": s.Alerts()})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
