package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fully-ai/hostwatch/internal/metrics"
)

func TestStoreAuthMultiHostHistoryAndAlerts(t *testing.T) {
	store := NewStore("secret-token", Thresholds{MaxCPUPercent: 85, MaxMemPercent: 90, MaxDiskPercent: 95}, 10)
	h := store.Handler()

	sample := metrics.HostSample{
		HostID:      "web-01",
		Hostname:    "web-01",
		OS:          "linux",
		Arch:        "amd64",
		CPUPercent:  92.5,
		MemPercent:  45.0,
		DiskPercent: 50.0,
		Services:    []metrics.ServiceState{{Name: "nginx", Active: false}},
	}
	body, _ := json.Marshal(sample)

	// Unauthenticated request rejected
	wUnauth := httptest.NewRecorder()
	h.ServeHTTP(wUnauth, httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(body)))
	if wUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", wUnauth.Code)
	}

	// Authenticated ingest accepted
	req := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token")
	wOk := httptest.NewRecorder()
	h.ServeHTTP(wOk, req)
	if wOk.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", wOk.Code)
	}

	// Second host
	s2 := sample
	s2.HostID = "win-01"
	s2.OS = "windows"
	s2.CPUPercent = 20.0
	s2.Services = nil
	b2, _ := json.Marshal(s2)
	req2 := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(b2))
	req2.Header.Set("Authorization", "Bearer secret-token")
	h.ServeHTTP(httptest.NewRecorder(), req2)

	if len(store.LatestHosts()) != 2 {
		t.Fatalf("expected 2 hosts registered")
	}
	if len(store.Alerts()) != 2 { // 1 CPU alert + 1 inactive service alert
		t.Fatalf("expected 2 alerts, got %d", len(store.Alerts()))
	}
	if len(store.HostHistory("web-01")) != 1 {
		t.Fatalf("expected 1 sample in history")
	}

	// Verify state persistence across store reload
	snapFile := filepath.Join(t.TempDir(), "state.json")
	if err := store.EnablePersistence(snapFile); err != nil {
		t.Fatal(err)
	}
	store.Ingest(s2)

	reloaded := NewStore("secret-token", Thresholds{}, 10)
	if err := reloaded.EnablePersistence(snapFile); err != nil {
		t.Fatal(err)
	}
	if len(reloaded.LatestHosts()) != 2 {
		t.Fatalf("expected 2 hosts restored from snapshot, got %d", len(reloaded.LatestHosts()))
	}
}


func TestPrometheusMetricsThresholdsAckAndWebhook(t *testing.T) {
	var webhookHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		webhookHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := NewStore("tok", Thresholds{MaxCPUPercent: 80, MaxMemPercent: 85, MaxDiskPercent: 90}, 20)
	store.SetWebhookURL(srv.URL)
	h := store.Handler()

	// Update thresholds via API
	thrBody, _ := json.Marshal(Thresholds{MaxCPUPercent: 70, MaxMemPercent: 80, MaxDiskPercent: 90})
	putReq := httptest.NewRequest(http.MethodPut, "/api/thresholds", bytes.NewReader(thrBody))
	putReq.Header.Set("Authorization", "Bearer tok")
	putW := httptest.NewRecorder()
	h.ServeHTTP(putW, putReq)
	if putW.Code != http.StatusOK {
		t.Fatalf("expected 200 on PUT /api/thresholds, got %d", putW.Code)
	}

	// Trigger CPU alert (75 > 70)
	sample := metrics.HostSample{
		HostID:      "db-01",
		Hostname:    "db-01",
		OS:          "linux",
		Arch:        "amd64",
		CPUPercent:  75.5,
		MemPercent:  40.0,
		DiskPercent: 55.0,
		Services:    []metrics.ServiceState{{Name: "postgres", Active: true}},
		Timestamp:   time.Now().UTC(),
	}
	store.Ingest(sample)

	// Wait briefly for async webhook goroutine
	deadline := time.Now().Add(2 * time.Second)
	for webhookHits.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if webhookHits.Load() != 1 {
		t.Fatalf("expected 1 webhook notification, got %d", webhookHits.Load())
	}

	// Verify Prometheus /metrics output
	metW := httptest.NewRecorder()
	h.ServeHTTP(metW, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if metW.Code != http.StatusOK {
		t.Fatalf("expected 200 from /metrics, got %d", metW.Code)
	}
	prom := metW.Body.String()
	if !strings.Contains(prom, `hostwatch_cpu_percent{host_id="db-01",os="linux"} 75.50`) {
		t.Fatalf("missing cpu metric in /metrics:\n%s", prom)
	}
	if !strings.Contains(prom, "hostwatch_alerts_active 1") {
		t.Fatalf("expected 1 active alert in /metrics:\n%s", prom)
	}

	// Acknowledge alert
	alerts := store.Alerts()
	ackReq := httptest.NewRequest(http.MethodPost, "/api/alerts/"+alerts[0].ID+"/ack", nil)
	ackReq.Header.Set("Authorization", "Bearer tok")
	ackW := httptest.NewRecorder()
	h.ServeHTTP(ackW, ackReq)
	if ackW.Code != http.StatusOK {
		t.Fatalf("expected 200 on alert ack, got %d", ackW.Code)
	}

	metW2 := httptest.NewRecorder()
	h.ServeHTTP(metW2, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(metW2.Body.String(), "hostwatch_alerts_active 0") {
		t.Fatalf("expected 0 active alerts after ack:\n%s", metW2.Body.String())
	}
}
