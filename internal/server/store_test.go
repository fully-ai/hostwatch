package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
}
