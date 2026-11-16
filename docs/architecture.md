# Architecture — HostWatch

## Agent-Server Architecture

- **Agent (`cmd/hostwatch-agent`, `internal/metrics/`)**:
  - Build-tagged native collectors (`collector_linux.go` reading `/proc/stat`, `/proc/meminfo`, `syscall.Statfs`, `/proc/net/dev`, `/proc/uptime`, `/proc/<pid>/comm`; `collector_windows.go` invoking `kernel32.dll` APIs without CGo).
  - Pushes JSON `HostSample` payloads periodically with optional Bearer authentication.
- **Server (`cmd/hostwatch-server`, `internal/server/`)**:
  - Validates bearer tokens with constant-time comparison (`subtle.ConstantTimeCompare`), stores rolling time-series samples per host, evaluates CPU/Memory/Disk/service thresholds, and serves an SVG sparkline web dashboard.
