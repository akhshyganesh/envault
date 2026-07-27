---
description: "envault — .env file backup daemon. Use this for all development tasks on this project."
applyTo: "**"
---

# envault — Development Guide

## What it does

CLI tool + background daemon (Go) that auto-discovers, versions, and backs up `.env` files using SHA-256 content-addressed storage. Supports macOS (launchd) and Linux (systemd).

## Stack

- **Language**: Go
- **CLI**: Cobra (`github.com/spf13/cobra`)
- **TUI**: BubbleTea + Lipgloss + Bubbles (`github.com/charmbracelet/*`)
- **Platforms**: darwin/amd64, darwin/arm64, linux/amd64, linux/arm64

## Project Structure

```
main.go                          — calls cmd.Execute()
cmd/
  root.go                        — rootCmd + Execute(); no-arg run → setup wizard (first time) or TUI (returning)
  helpers.go                     — resolveFileArg, sanitizeExtractPath, getLatestVersion
  init.go                        — init command
  scan.go                        — scan command
  vault.go                       — list, history, show commands
  restore.go                     — restore command
  daemon.go                      — start, stop, status, watch commands
  install.go                     — install, uninstall commands
  transfer.go                    — export, import commands
  peek.go                        — peek command (read-only archive browsing)
  upgrade.go                     — version, upgrade commands
  ui.go                          — ui command
internal/
  config/config.go               — Config struct, Load/Save (~/.envault/config.json)
  format/format.go               — ShortenPath, TimeAgo, HumanSize (shared by cmd + tui)
  store/store.go                 — SHA-256 blob store + JSON index
  scanner/scanner.go             — recursive .env file discovery
  daemon/daemon.go               — background loop, PID management
  daemon/service.go              — launchd/systemd service install
  setup/setup.go                 — interactive install wizard (3-step BubbleTea)
  transfer/transfer.go           — vault export/import (zip)
  transfer/archive.go            — read-only reader for an export zip (used by peek)
  tui/tui.go                     — interactive TUI browser (3-view BubbleTea state machine)
cmd/maintenance.go               — gc, forget commands
.github/workflows/release.yml   — CI/CD: build + publish on v* tags
```

## Build

```bash
make build     # go build -o envault .
make test      # go test ./...
make install   # sudo cp to /usr/local/bin/
make clean
```

## Commands

| Command | Flags | Purpose |
|---------|-------|---------|
| `init` | — | Create `~/.envault/` with defaults |
| `scan [dirs...]` | — | Discover and back up .env files |
| `list` / `ls` | — | Show tracked files with numbered index |
| `history <file\|#>` | — | Show version history |
| `show <file\|#>` | `-v` version | Print backed-up content to stdout |
| `restore <file\|#>` | `-v` version, `-o` output path | Restore from backup |
| `forget <file\|#>` | — | Stop tracking a file, delete its history |
| `gc` | — | Delete unreferenced blobs to reclaim disk |
| `watch <dir>` | — | Add directory to watch list |
| `start` / `stop` / `status` | — | Control background daemon |
| `install` | — | Interactive TUI wizard — dirs, interval, scan, service install |
| `uninstall` | `--prune` | Remove OS service (optionally delete all data) |
| `export` | `-o` path | Export vault as zip |
| `import <zip>` | `--force` | Import vault from zip |
| `peek <zip> [file\|#]` | `-v` version, `--show`, `-o` path, `--force` | Browse an export zip read-only; never writes to the vault |
| `upgrade` | — | Self-update from GitHub |
| `version` | — | Print version + credits |
| `ui` | — | Launch interactive TUI |

`<file|#>` accepts either a file path or the numeric index shown by `envault list`.

## Storage Layout

```
~/.envault/
├── config.json      — WatchDirs, ScanIntervalSecs, MaxVersions
├── blobs/           — content files named by SHA-256 hash (deduplicated)
├── index/           — per-file JSON with snapshot history
├── envault.pid      — daemon PID (runtime only)
└── envault.log      — daemon log (runtime only)
```

## Code Conventions

- **Permissions**: `0600` for files, `0700` for directories
- **Error wrapping**: `fmt.Errorf("context: %w", err)` throughout
- **Best-effort exec calls**: use `_ = exec.Command(...).Run()` with a comment
- **Env file patterns**: `.env`, `.env.*`, `*.env`
- **Skip dirs**: `node_modules`, `.git`, `.svn`, `.hg`, `vendor`, `__pycache__`, `.venv`, `venv`, `.tox`, `dist`, `build`, `.envault`
- **Deduplication**: same content → same blob, no duplicate snapshots
- **Version pruning**: `max_versions` > 0 trims each file's history to the N newest snapshots; `envault gc` reclaims the orphaned blobs

## UI Style

Both the TUI and the setup wizard share one identity (`internal/tui` and `internal/setup`):
a warm amber accent (256-color `180`/`215`) on neutral grays, a 🔒 brand mark, breadcrumb
headers over a hairline rule, an accent gutter bar (`▌`) for the selected row, and a status
bar whose hint keys are rendered in the accent color. Palette lives in the `col*` constants
at the top of each package's style block — change colors there, not at call sites.

## TUI Views

Four-view state machine (`fileListView → historyView → contentView`, plus a modal
`inputView` for paths and confirmations):
- `↑↓` / `jk` — navigate · `Enter` / `l` / `→` — open · `Esc` / `h` / `←` — back
- `g` / `G` — top / bottom · `q` — quit
- File list actions: `s` scan · `d` toggle daemon · `w` watch dir · `e` export · `i` import
- History actions: `r` restore · `R` restore to path · content view: `c` copy

## Release

```bash
git tag v<X.Y.Z>
git push origin v<X.Y.Z>
# GitHub Actions builds 4 binaries + checksums.txt and publishes a Release
```
