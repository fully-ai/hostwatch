package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/fully-ai/hostwatch/internal/metrics"
)

func main() {
	serverURL := flag.String("server", "http://127.0.0.1:8096", "HostWatch server URL")
	token := flag.String("token", os.Getenv("HOSTWATCH_TOKEN"), "Agent bearer token")
	hostID := flag.String("host-id", "", "Override host identifier (defaults to hostname)")
	services := flag.String("services", "", "Comma-separated process names to monitor")
	disk := flag.String("disk", "", "Disk mount/drive path to measure")
	interval := flag.Duration("interval", 5*time.Second, "Reporting interval")
	once := flag.Bool("once", false, "Collect and print one sample, then exit")
	flag.Parse()

	var svcList []string
	for _, s := range strings.Split(*services, ",") {
		if t := strings.TrimSpace(s); t != "" {
			svcList = append(svcList, t)
		}
	}

	if *once {
		sample, err := metrics.CollectSample(*hostID, svcList, *disk)
		if err != nil {
			fmt.Fprintf(os.Stderr, "collect error: %v\n", err)
			os.Exit(1)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(sample)
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	endpoint := strings.TrimRight(*serverURL, "/") + "/api/ingest"
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	push := func() {
		sample, err := metrics.CollectSample(*hostID, svcList, *disk)
		if err != nil {
			fmt.Fprintf(os.Stderr, "collect error: %v\n", err)
			return
		}
		body, _ := json.Marshal(sample)
		req, _ := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if *token != "" {
			req.Header.Set("Authorization", "Bearer "+*token)
		}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "push error: %v\n", err)
			return
		}
		_ = resp.Body.Close()
	}

	push()
	for {
		select {
		case <-ticker.C:
			push()
		case <-sigCh:
			return
		}
	}
}
