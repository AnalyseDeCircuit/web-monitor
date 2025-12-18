# Changelog

All notable changes for this project will be documented in this file.

## v0.1.0

### What it is
A lightweight, modular web-based system monitor designed to scale
from embedded devices to small server clusters.

### Features
- CPU / Memory / Disk / Network monitoring
- Modular feature toggles
- Multiple run modes via Makefile:
  - full (default)
  - minimal
  - server
  - no-docker
- Low frontend CPU usage (<15%)

### Supported environments
- Linux (bare metal / VPS)
- Docker
- WSL (partial)
- macOS (limited, experimental)

### Known limitations
- No long-term historical storage
- macOS / WSL lack some system metrics
- No authentication hardening by default

### Philosophy
Keep it simple, observable, and cheap to run.

## Who this is for
- Small servers / VPS
- Home lab
- Embedded or low-power machines
- Users who want instant visibility, not heavy dashboards

## Who this is NOT for
- Enterprise monitoring
- Long-term metrics storage
- Alerting / SLA systems
