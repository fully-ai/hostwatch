$ErrorActionPreference = "Stop"
Set-Location (Split-Path -Parent $PSScriptRoot)
New-Item -ItemType Directory -Force -Path bin | Out-Null
go build -o bin/hostwatch-server.exe ./cmd/hostwatch-server
go build -o bin/hostwatch-agent.exe ./cmd/hostwatch-agent
