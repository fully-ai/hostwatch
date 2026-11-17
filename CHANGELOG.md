# Changelog

## [0.3.0] - 2026-11-18

### Added
- **Prometheus `/metrics` Endpoint**: Exposes per-host CPU, memory, disk, network counters, service states, and active alert counts in Prometheus text exposition format (`0.0.4`).
- **Alert Acknowledgment & Cooldown Webhooks**: Added `POST /api/alerts/{id}/ack`, alert IDs, and cooldown-deduplicated webhook notifications (`-webhook` / `HOSTWATCH_WEBHOOK_URL`).
- **Dynamic Thresholds & Liveness Status**: Added `GET`/`PUT /api/thresholds` (`0..100` validated), host `online`/`degraded`/`offline` status badges, threshold controls in dashboard UI, and `scripts/{install,dev,test}.{sh,ps1}`.

## [0.2.0] - 2026-11-16

### Added
- Optional JSON state snapshot persistence (`-state` flag / `EnablePersistence`) for host histories and alerts.
- Service status display in the web dashboard cards.
- Persistence reload test and comprehensive maintainer README with Mermaid diagram.

## [0.1.0] - 2026-11-16

### Added
- Cross-platform metrics agent (`hostwatch-agent`) with native Linux `/proc` & `Statfs` collectors and Windows `kernel32.dll` API collectors.
- Central collection server (`hostwatch-server`) with constant-time bearer token authentication, multi-host history, threshold alerting, and SVG dashboard.
