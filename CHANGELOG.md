# Changelog

## [0.2.0] - 2026-11-16

### Added
- Optional JSON state snapshot persistence (`-state` flag / `EnablePersistence`) for host histories and alerts.
- Service status display in the web dashboard cards.
- Persistence reload test and comprehensive maintainer README with Mermaid diagram.

## [0.1.0] - 2026-11-16

### Added
- Cross-platform metrics agent (`hostwatch-agent`) with native Linux `/proc` & `Statfs` collectors and Windows `kernel32.dll` API collectors.
- Central collection server (`hostwatch-server`) with constant-time bearer token authentication, multi-host history, threshold alerting, and SVG dashboard.
