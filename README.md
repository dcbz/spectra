# Spectra Watch

Spectra Watch is a lush Bubble Tea / Lip Gloss terminal interface tailor-made for security-focused log watching. It tails multiple Linux log files, runs every line through a modular regex ruleset, then paints the terminal with neon gradients, animated cues, and severity-driven accents—perfect for rice screenshots.

## Features

- 🚨 Regex-based rule engine with YAML configuration and capture groups
- 🌈 Multiple hand-tuned themes (`vapor`, `midnight`, `dusk`) with live switching (`t` key)
- 🪟 Split-pane layout: spacious log viewport, animated sidebar pulse, status ribbon
- 🎯 Focused feed that only displays rule hits by default (pass `--show-all` to stream every line)
- 📉 Severity floor via `--min-severity` so you can ignore low-priority chatter (default `medium`)
- 👁️ Animated ANSI “sentinel” eye in the header so you know the watcher is alive
- 🔦 Inline highlight fragments for matched substrings plus tag pills and rule badges
- 🪄 Smooth auto-follow with optional pause (`p`) and follow toggle (`f`)
- ♻️ Robust file tailer (`github.com/nxadm/tail`) that survives rotations/restarts

## Quick Start

```bash
go run ./cmd/watcher --files=/var/log/auth.log,/var/log/syslog --config=configs/example.rules.yaml --theme=vapor
```

Keys: `q` quit, `p` pause (freezes viewport but keeps collecting data), `f` toggle auto-follow, `t` cycle theme.

Navigation: `↑`/`↓` move selection, `PgUp`/`PgDn` page through results, `Enter` opens the alert detail modal (press `Enter` or `Esc` again to dismiss).

Add `--show-all` to include every log line, and `--min-severity=high` (or similar) to dial-in the signal you want.

## Rules Configuration

Rules live in YAML (`configs/example.rules.yaml`). Each rule supports:

```yaml
- name: ssh brute force
  pattern: 'Failed password for (?P<user>\S+) from (?P<ip>\d+\.\d+\.\d+\.\d+)'
  severity: critical   # critical|high|medium|low|normal
  color: "#FF5E5B"     # optional hex accent for future themes
  tags: [ssh, brute]   # inform sidebar badges and downstream hooks
```

Order matters; rules of the same severity trigger based on declaration order. Captured named groups are available for future alert hooks.

## Project Layout

- `cmd/watcher`: CLI wiring, flag parsing, graceful shutdown.
- `internal/watch`: resilient tailer per log file.
- `internal/rules`: YAML loader, compiler, and matcher.
- `internal/highlight`: splits matched indices into fragments for styling.
- `internal/pipeline`: links raw log events to highlighted events consumed by the UI.
- `internal/tui`: Bubble Tea model, layout, and theming.

## Development

- Standard Go workflow: `go build ./...`, `go test ./...` (after adding tests).
- Linting compatible with `golangci-lint`.
- Theme tweaks live in `internal/tui/theme.go`—use Lip Gloss to craft new palettes.

Enjoy painting your terminal like a synthwave SOC console! ✨
