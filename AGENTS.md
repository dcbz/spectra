# Spectra Watch – Agent Guide

Practical playbook for autonomous agents working in this repository. Read once, keep handy. Nothing here excuses ignoring the repo history—always skim existing code before editing.

## Repository Snapshot
- Language/toolchain: Go 1.24.2 (`go.mod`).
- UI stack: Bubble Tea, Lip Gloss, bubbles viewport.
- Streaming: `github.com/nxadm/tail` handles follow + reopen.
- Config: YAML rule files under `configs/`.
- Binaries: built to `bin/spectra-watch` via Makefile.
- No Cursor/Copilot rule files are present today; follow this doc plus in-code conventions.

## Build & Run Commands
- `make build` → compiles `./cmd/watcher` into `bin/spectra-watch` with modules on.
- `make run` → rebuilds then launches default config (reads real logs; override flags when testing).
- `go build ./cmd/watcher` for direct compilation without Make targets.
- `go run ./cmd/watcher --files=... --config=...` for ad-hoc experiments.
- Clean artifacts with `make clean` (drops the entire `bin` dir) before packaging releases.
- Keep go.sum tidy: `make tidy` (i.e., `go mod tidy`).

## Testing Playbook
- Global suite (once tests exist): `go test ./...`.
- Package scope: `go test ./internal/rules` (swap package path as needed).
- Single test focus: `go test ./internal/rules -run TestParseSeverity` (regex accepted, case-sensitive).
- Race checks when touching concurrency (`internal/watch`, `internal/pipeline`): `go test -race ./internal/...`.
- Use `GO111MODULE=on` implicitly (default for Go ≥1.13); environment only required in Makefile.
- Snapshot behavior manually: run the TUI, then use `p`, `f`, `t`, `q` to confirm keystroke handling.

## Linting & Formatting
- Default formatter is `gofmt`; run `make fmt` (formats `cmd` + `internal`).
- Keep imports grouped stdlib/third-party/local with a blank line between groups (see `cmd/watcher/main.go`).
- No auto-lint config in repo, but GolangCI-Lint must stay green; prioritize `govet`, `staticcheck`, `ineffassign` heuristics when editing.
- Favor `goimports` locally to maintain sorted import lists, but do not add import comments.
- Align Lip Gloss style chains across files (one fluent call per line max 1–2 segments; see `internal/tui/theme.go`).
- Avoid introducing `gofumpt`-style spacing unless consistent everywhere.

## Naming & Types
- Export only when you expect another package to consume the type (`rules.RuleSet`, `tui.Model`, etc.).
- Use concrete structs with constructor helpers (`tui.NewModel`, `pipeline.New`).
- Keep zero-value safe designs: e.g., `tui.Model` handles nil channels by returning no-op `tea.Cmd` from `listen`.
- Severity constants live in `internal/rules/types.go`; do not redefine severity strings downstream—import and reuse `rules.Severity*`.
- Channel payload types (`watch.LogEvent`, `pipeline.HighlightedEvent`) should stay immutable once emitted; copy slices when mutating.
- Named return values are rare in this repo—return explicit tuples.

## Error Handling & Logging
- Propagate errors with context via `fmt.Errorf("action: %w", err)` (see `internal/watch/tailer.go`).
- CLI fatal paths rely on `log.Fatalf`; reserve `log.Fatal` for truly unrecoverable init failures.
- Never `panic` for user/config errors; return errors up the stack.
- Keep user-facing CLI errors short, but log internal diagnostics with rule/file names.
- In the TUI, surface transient issues through `notification` text rather than spamming the viewport.

## Imports & Modules
- Standard library block first, then third-party, finally local `watcher/...` packages.
- Alias imports only for widely recognized packages (`tea` for Bubble Tea) or when names collide.
- Keep go.mod minimal; remove unused deps via `go mod tidy` before committing.
- When adding UI libs, prefer Charmbracelet ecosystem for consistency.

## Concurrency & Context
- Every goroutine that watches files must respect the `context.Context` provided by `cmd/watcher/main.go`.
- Always stop tailers when contexts cancel; `tail.TailFile` already exposes `.Cleanup()`—call it in `defer`.
- When bridging channels, close the outgoing channel exactly once (see `pipeline.Stream.Connect`).
- Use buffered channels only when there is measurable backpressure; default is unbuffered to preserve ordering.
- Avoid shared state without locks; where counts are needed (`tui.Model.counts`), mutate on the UI goroutine only.

## TUI & UX Expectations
- Layout: main pane + sidebar; do not let new content alter the fundamental geometry.
- Sidebar width (default 30 incl. frame) must remain large enough to keep the sentinel eye inside its border—no overflow, no blank filler rows.
- Always recompute viewport dimensions when handling `tea.WindowSizeMsg`; adjust for Lip Gloss frame sizes before setting width/height.
- Keep header/status heights synchronized with body so terminal bounds are honored; never rely on padding with empty lines.
- Animations: `pulse()` tick toggles shimmer + sentinel frame index. Reuse this message loop for new subtle animations rather than introducing new timers per effect.
- Themes live in `internal/tui/theme.go`; add new palettes through helper functions returning a full `Theme`, then wire them into `themeByName` + `nextTheme` order.
- Any new keybindings must be advertised inside `renderStatus()` to remain discoverable.

## Rules Engine & Pipeline
- YAML schema defined in `internal/rules/types.go`; keep backward compatibility when extending fields.
- Rule compilation sorts by severity rank, then declaration order; do not break stable matching.
- Highlight spans are `[start,end)` byte offsets; keep them bounds-checked via `clamp` to avoid panics.
- `pipeline.Stream.Connect` enforces `showAll` + `minSeverity`; respect these flags when adding new filtering logic.
- Highlight fragments merge adjacent ranges with matching emphasis; re-use `highlight.BuildFragments` instead of reimplementing merges.
- Tests to add later should cover severity parsing, rule ordering, and fragment splitting (see `internal/highlight/highlight.go`).

## Config & Assets
- Example rules: `configs/example.rules.yaml`. Keep new sample rules production-realistic and severity-balanced.
- Document new flags or config fields in both README and `cmd/watcher/main.go` flag help text.
- Keep binary-safe strings ASCII unless theming truly benefits from Unicode glyphs (current UI already uses ✧/✦).
- When adding assets (fonts, art), store them under `assets/` or similar and load as strings; never ship binary blobs directly in Go files without compression justification.

## CLI Flags & Runtime Behavior
- `--files` accepts a comma-separated list; `splitFiles` trims whitespace and drops empties—mirror that logic for new inputs.
- `--config` must point to YAML following `ruleFile`; validate early and wrap errors (`load rules: %w`).
- `--theme` currently cycles `vapor`, `midnight`, `dusk`; extend `themeByName` + `nextTheme` together.
- `--scrollback` defaults to 800; clamp to sane positive values before applying to `Model`.
- `--show-all` toggles unmatched lines; when false, only matched events meeting `minSeverity` should reach the UI.
- `--min-severity` flows through `rules.ParseSeverity`; accept lowercase inputs plus `med` alias.
- All flag additions must be described in README + `renderStatus()` if they affect runtime controls.

## File Reference
- `cmd/watcher/main.go` – CLI entry point, flag parsing, program start.
- `internal/watch/tailer.go` – file tailer producing log events.
- `internal/rules/` – rule types, YAML loader, severity helpers.
- `internal/pipeline/pipeline.go` – highlight pipeline logic.
- `internal/highlight/highlight.go` – fragment builder for matched spans.
- `internal/tui/model.go` – Bubble Tea model, layout logic, sentinel eye, sidebar.
- `internal/tui/theme.go` – Lip Gloss themes and style helpers.
- `configs/example.rules.yaml` – default rule definitions (keep it realistic and severity-balanced).
- `README.md` – project overview, usage, config instructions (update on user-facing changes).

## Debugging & Observation
- Use temporary log files (`mktemp`) plus `tail -f` to feed synthetic lines while observing TUI behavior.
- Enable Bubble Tea logging via `export DEBUG=1` and `tea.NewProgram(..., tea.WithDebug(writer))` only in local experiments—do not commit debug flags.
- When diagnosing layout, print viewport widths/heights with `lipgloss.Width` helpers rather than guessing.
- For rule issues, log `match.Rule.Name`, severity, and `match.HighlightSpans` before deciding to filter.
- Prefer writing focused Go tests around helpers (e.g., `rules.ParseSeverity`, `highlight.BuildFragments`) to reproduce regressions.

## Documentation Expectations
- README should always mirror available flags, themes, and keyboard shortcuts.
- Inline comments only where behavior is non-obvious (e.g., sentinel animation math, highlight merging).
- Update `configs/example.rules.yaml` comments if semantics change.
- Keep AGENTS.md current whenever toolchain, UI requirements, or workflow steps evolve.
- If a change impacts end-user configuration, add a short rationale to the PR body for future discovery.

## Dependency Hygiene
- Run `go mod tidy` after adding/removing imports; ensure go.sum stays deterministic.
- Avoid adding large UI deps unless they align with Charmbracelet ecosystem.
- Prefer standard library solutions before reaching for third-party helpers.
- Vendor patches should land upstream; do not fork deps inside this repo.
- Pin dependency versions in go.mod; avoid replacing modules unless absolutely required, and document the reason here.

## Performance & Resource Use
- TUI scrollback defaults to 800 lines; ensure new features honor `--scrollback` to avoid unbounded memory.
- Avoid per-line heap allocations where possible; reuse builders (`strings.Builder`) and pre-size slices.
- Tailers already block on I/O; no need to spawn worker pools for log parsing.
- Keep CSS-like Lip Gloss style construction outside render loops—compute them once when building themes.

## Workflow Expectations
- Always run `make fmt` + targeted `go test` before opening PRs.
- Describe UI changes with screenshots/gifs when possible; automated tests can’t cover layout.
- Respect the sentinel eye requirement when adjusting the sidebar—the animation must stay visible and within bounds across terminal sizes.
- Update README whenever CLI flags, keybindings, or theme names change.
- If you add integration scripts, document them here so future agents know they exist.

## Git Workflow
- After every code or doc change, run `git status` to confirm what changed, then stage and commit (or stash) before starting the next edit so history stays granular and the working tree is clean.

## Ready Checklist Before Submitting Changes
- Code formatted via `gofmt`/`make fmt`.
- go.mod/go.sum tidy.
- Tests (if any) pass locally with `go test ./...` and targeted `-run` invocations.
- Manual smoke test: run `go run ./cmd/watcher --files=<tmp log> --config=configs/example.rules.yaml` and exercise `p`, `f`, `t`, `q`.
- Confirm TUI layout on both narrow (~80 cols) and wide (>160 cols) terminals.
- Ensure notifications clear after ~5 seconds (respect existing timer logic when editing).

Keep this document updated whenever tooling or style expectations shift—future agents rely on it.
