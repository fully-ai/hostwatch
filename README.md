# HostWatch

Cross-platform Linux and Windows system monitoring platform built around a lightweight agent-server architecture in Go.

## Features

- **Cross-Platform Agent (`hostwatch-agent`)**:
  - **Linux**: Reads `/proc/stat` (CPU), `/proc/meminfo` (RAM), `syscall.Statfs` (disk), `/proc/net/dev` (network RX/TX), `/proc/uptime`, and `/proc/<pid>/comm` process checks.
  - **Windows**: Uses `kernel32.dll` (`GetSystemTimes`, `GlobalMemoryStatusEx`, `GetDiskFreeSpaceExW`, `GetTickCount64`) with zero CGo dependencies.
- **Central Server (`hostwatch-server`)**: Authenticated HTTP ingestion (`Authorization: Bearer <token>`), multi-host registry, per-host time-series ring buffers, CPU/RAM/Disk/service alerting, and embedded SVG sparkline dashboard.

## Installation & Usage

Build server and agent:

```bash
git clone https://github.com/fully-ai/hostwatch.git
cd hostwatch
go build -o hostwatch-server ./cmd/hostwatch-server
go build -o hostwatch-agent ./cmd/hostwatch-agent
```

Cross-compile Windows agent from Linux:

```bash
GOOS=windows GOARCH=amd64 go build -o hostwatch-agent.exe ./cmd/hostwatch-agent
```

Run server and agent:

```bash
./hostwatch-server -addr 127.0.0.1:8096 -token my-secret
./hostwatch-agent -server http://127.0.0.1:8096 -token my-secret -services sshd,nginx
```

## Testing

```bash
go vet ./...
go test -v -race ./...
GOOS=windows GOARCH=amd64 go build ./...
```

## License

Apache License 2.0 — see [LICENSE](LICENSE).
