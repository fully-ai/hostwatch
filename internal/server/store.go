package server

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
	ID           string     `json:"id"`
	HostID       string     `json:"host_id"`
	Metric       string     `json:"metric"`
	Value        float64    `json:"value"`
	Threshold    float64    `json:"threshold"`
	Message      string     `json:"message"`
	TriggeredAt  time.Time  `json:"triggered_at"`
	Acknowledged bool       `json:"acknowledged"`
	AckAt        *time.Time `json:"ack_at,omitempty"`
}

type HostStatus struct {
	metrics.HostSample
	Status string `json:"status"`
}

type Store struct {
	mu           sync.RWMutex
	maxSamples   int
	agentToken   string
	statePath    string
	webhookURL   string
	thresholds   Thresholds
	history      map[string][]metrics.HostSample
	alerts       []Alert
	lastNotified map[string]time.Time
	httpClient   *http.Client
}

func NewStore(agentToken string, thresholds Thresholds, maxSamples int) *Store {
	if maxSamples <= 0 {
		maxSamples = 120
	}
	return &Store{
		maxSamples:   maxSamples,
		agentToken:   agentToken,
		thresholds:   thresholds,
		history:      make(map[string][]metrics.HostSample),
		lastNotified: make(map[string]time.Time),
		httpClient:   &http.Client{Timeout: 5 * time.Second},
	}
}

func (s *Store) SetWebhookURL(u string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.webhookURL = strings.TrimSpace(u)
}

func newAlertID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (s *Store) EnablePersistence(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statePath = path
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var snap struct {
		History map[string][]metrics.HostSample `json:"history"`
		Alerts  []Alert                         `json:"alerts"`
	}
	if err := json.Unmarshal(raw, &snap); err != nil {
		return err
	}
	if snap.History != nil {
		s.history = snap.History
	}
	if snap.Alerts != nil {
		s.alerts = snap.Alerts
	}
	return nil
}

func (s *Store) saveLocked() {
	if s.statePath == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.statePath), 0o755)
	snap := struct {
		History map[string][]metrics.HostSample `json:"history"`
		Alerts  []Alert                         `json:"alerts"`
	}{History: s.history, Alerts: s.alerts}
	if data, err := json.Marshal(snap); err == nil {
		_ = os.WriteFile(s.statePath, data, 0o600)
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

	var triggered []Alert
	addAlert := func(metric string, val, thr float64, msg string) {
		a := Alert{
			ID:          newAlertID(),
			HostID:      sample.HostID,
			Metric:      metric,
			Value:       val,
			Threshold:   thr,
			Message:     msg,
			TriggeredAt: sample.Timestamp,
		}
		s.alerts = append(s.alerts, a)
		key := sample.HostID + "|" + metric
		if s.webhookURL != "" && time.Since(s.lastNotified[key]) >= 60*time.Second {
			s.lastNotified[key] = time.Now().UTC()
			triggered = append(triggered, a)
		}
	}

	if s.thresholds.MaxCPUPercent > 0 && sample.CPUPercent > s.thresholds.MaxCPUPercent {
		addAlert("cpu_percent", sample.CPUPercent, s.thresholds.MaxCPUPercent,
			fmt.Sprintf("%s CPU at %.1f%% (threshold %.1f%%)", sample.HostID, sample.CPUPercent, s.thresholds.MaxCPUPercent))
	}
	if s.thresholds.MaxMemPercent > 0 && sample.MemPercent > s.thresholds.MaxMemPercent {
		addAlert("mem_percent", sample.MemPercent, s.thresholds.MaxMemPercent,
			fmt.Sprintf("%s Memory at %.1f%% (threshold %.1f%%)", sample.HostID, sample.MemPercent, s.thresholds.MaxMemPercent))
	}
	if s.thresholds.MaxDiskPercent > 0 && sample.DiskPercent > s.thresholds.MaxDiskPercent {
		addAlert("disk_percent", sample.DiskPercent, s.thresholds.MaxDiskPercent,
			fmt.Sprintf("%s Disk at %.1f%% (threshold %.1f%%)", sample.HostID, sample.DiskPercent, s.thresholds.MaxDiskPercent))
	}
	for _, svc := range sample.Services {
		if !svc.Active {
			addAlert("service:"+svc.Name, 0, 1, fmt.Sprintf("%s service %q is inactive", sample.HostID, svc.Name))
		}
	}
	webhook := s.webhookURL
	s.saveLocked()
	if webhook != "" && len(triggered) > 0 {
		for _, a := range triggered {
			go s.dispatchWebhook(webhook, a)
		}
	}
}

func (s *Store) dispatchWebhook(url string, alert Alert) {
	body, err := json.Marshal(alert)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
}

func computeStatus(ts time.Time) string {
	age := time.Since(ts)
	if age <= 30*time.Second {
		return "online"
	}
	if age <= 120*time.Second {
		return "degraded"
	}
	return "offline"
}

func (s *Store) LatestHosts() []HostStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]HostStatus, 0, len(s.history))
	for _, samples := range s.history {
		if len(samples) > 0 {
			last := samples[len(samples)-1]
			out = append(out, HostStatus{
				HostSample: last,
				Status:     computeStatus(last.Timestamp),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].HostID < out[j].HostID })
	return out
}

func (s *Store) AcknowledgeAlert(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.alerts {
		if s.alerts[i].ID == id {
			now := time.Now().UTC()
			s.alerts[i].Acknowledged = true
			s.alerts[i].AckAt = &now
			s.saveLocked()
			return true
		}
	}
	return false
}

func (s *Store) Thresholds() Thresholds {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.thresholds
}

func (s *Store) SetThresholds(t Thresholds) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.thresholds = t
}

func (s *Store) PrometheusMetrics() string {
	hosts := s.LatestHosts()
	alerts := s.Alerts()
	activeAlerts := 0
	for _, a := range alerts {
		if !a.Acknowledged {
			activeAlerts++
		}
	}
	var sb strings.Builder
	sb.WriteString("# TYPE hostwatch_cpu_percent gauge\n")
	for _, h := range hosts {
		fmt.Fprintf(&sb, "hostwatch_cpu_percent{host_id=%q,os=%q} %.2f\n", h.HostID, h.OS, h.CPUPercent)
	}
	sb.WriteString("# TYPE hostwatch_mem_percent gauge\n")
	for _, h := range hosts {
		fmt.Fprintf(&sb, "hostwatch_mem_percent{host_id=%q,os=%q} %.2f\n", h.HostID, h.OS, h.MemPercent)
	}
	sb.WriteString("# TYPE hostwatch_disk_percent gauge\n")
	for _, h := range hosts {
		fmt.Fprintf(&sb, "hostwatch_disk_percent{host_id=%q,os=%q} %.2f\n", h.HostID, h.OS, h.DiskPercent)
	}
	sb.WriteString("# TYPE hostwatch_net_rx_bytes counter\n")
	for _, h := range hosts {
		fmt.Fprintf(&sb, "hostwatch_net_rx_bytes{host_id=%q} %d\n", h.HostID, h.NetRxBytes)
	}
	sb.WriteString("# TYPE hostwatch_net_tx_bytes counter\n")
	for _, h := range hosts {
		fmt.Fprintf(&sb, "hostwatch_net_tx_bytes{host_id=%q} %d\n", h.HostID, h.NetTxBytes)
	}
	sb.WriteString("# TYPE hostwatch_service_up gauge\n")
	for _, h := range hosts {
		for _, svc := range h.Services {
			v := 0
			if svc.Active {
				v = 1
			}
			fmt.Fprintf(&sb, "hostwatch_service_up{host_id=%q,service=%q} %d\n", h.HostID, svc.Name, v)
		}
	}
	sb.WriteString("# TYPE hostwatch_alerts_active gauge\n")
	fmt.Fprintf(&sb, "hostwatch_alerts_active %d\n", activeAlerts)
	return sb.String()
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
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(s.PrometheusMetrics()))
	})
	mux.HandleFunc("/api/thresholds", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			writeJSON(w, http.StatusOK, s.Thresholds())
			return
		}
		if r.Method != http.MethodPut {
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
		var thr Thresholds
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&thr); err != nil {
			http.Error(w, "invalid thresholds", http.StatusBadRequest)
			return
		}
		if thr.MaxCPUPercent < 0 || thr.MaxCPUPercent > 100 || thr.MaxMemPercent < 0 || thr.MaxMemPercent > 100 || thr.MaxDiskPercent < 0 || thr.MaxDiskPercent > 100 {
			http.Error(w, "thresholds must be 0..100", http.StatusBadRequest)
			return
		}
		s.SetThresholds(thr)
		writeJSON(w, http.StatusOK, thr)
	})
	mux.HandleFunc("/api/alerts", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"alerts": s.Alerts()})
	})
	mux.HandleFunc("/api/alerts/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/ack") {
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
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/alerts/"), "/ack")
		if id == "" || !s.AcknowledgeAlert(id) {
			http.Error(w, "alert not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "acknowledged", "id": id})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
