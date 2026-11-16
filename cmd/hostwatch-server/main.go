package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/fully-ai/hostwatch/internal/server"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8096", "Server listen address")
	token := flag.String("token", os.Getenv("HOSTWATCH_TOKEN"), "Optional bearer token required from agents")
	maxCPU := flag.Float64("max-cpu", 90.0, "CPU alert threshold percent")
	maxMem := flag.Float64("max-mem", 90.0, "Memory alert threshold percent")
	maxDisk := flag.Float64("max-disk", 90.0, "Disk alert threshold percent")
	flag.Parse()

	store := server.NewStore(*token, server.Thresholds{
		MaxCPUPercent:  *maxCPU,
		MaxMemPercent:  *maxMem,
		MaxDiskPercent: *maxDisk,
	}, 180)

	fmt.Printf("HostWatch server listening on http://%s\n", *addr)
	if err := http.ListenAndServe(*addr, store.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
