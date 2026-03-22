---
description: "envault — .env file backup daemon. Use this for all development tasks on this project."
applyTo: "**"
---

# envault Development Agent

## Project Purpose

envault is a cross-platform CLI tool and background daemon (Go) that automatically discovers, versions, and backs up `.env` files. It uses SHA-256 content-addressed storage (like git) and supports macOS (launchd) and Linux (systemd).

## Tech Stack

- **Language**: Go
- **CLI Framework**: Cobra (`github.com/spf13/cobra`)
- **TUI Framework**: BubbleTea (`github.com/charmbracelet/bubbletea`) + Lipgloss + Bubbles viewport
- **Platforms**: macOS (darwin/amd64, darwin/arm64), Linux (linux/amd64, linux/arm64)
- **CI/CD**: GitHub Actions (`.github/workflows/release.yml`)

## Project Structure

```
main.go                     — entry point, calls cmd.Execute()
cmd/root.go                 — all 13 CLI commands (single-file pattern)
internal/config/config.go   — Config struct, Load/Save (~/.envault/config.json)
internal/store/store.go     — SHA-256 blob store + JSON index, versioning
internal/scanner/scanner.go — recursive .env file discovery
internal/daemon/daemon.go   — background daemon loop, PID management
internal/daemon/service.go  — launchd (macOS) / systemd (Linux) service install
internal/tui/tui.go         — interactive TUI (BubbleTea, 3 views)
Makefile                    — build, install, clean, test targets
scripts/build-release.sh    — cross-compile for 4 platforms
.github/workflows/release.yml — CI/CD release on v* tags
```

## Build & Test

```bash
make build          # go build -o envault .
make test           # go test ./...
make install        # sudo cp to /usr/local/bin/
make clean          # rm binary
```

## CLI Commands

| Command | Args / Flags | Purpose |
|---------|-------------|---------|
| `init` | — | Create `~/.envault/` with default config |
| `scan [dirs...]` | optional dirs (defaults to config watch dirs) | Discover and back up .env files |
| `list` / `ls` | — | Show tracked files with numbered index |
| `history <file\|#>` | file path or index number | Show version history |
| `restore <file\|#>` | `--version/-v` (int, default=latest) | Restore file from backup |
| `show <file\|#>` | `--version/-v` (int, default=latest) | Print file content to stdout |
| `watch <dir>` | directory path | Add directory to watch list |
| `start` | — | Start daemon in foreground |
| `stop` | — | Send SIGTERM to daemon |
| `status` | — | Check if daemon is running |
| `install` | — | Interactive setup + OS service install |
| `uninstall` | `--prune` (deletes all backups) | Remove OS service |
| `ui` | — | Launch interactive TUI |
| `upgrade` | — | Self-update to latest GitHub release |
| `version` | — | Print version, build date, and credits |

Commands accepting `<file|#>` resolve numeric args as indices from `envault list`.

## Storage Layout (`~/.envault/`)

```
~/.envault/
├── config.json        — WatchDirs, ScanIntervalSecs, MaxVersions
├── blobs/             — content-addressed files (SHA-256 hash as filename)
├── index/             — per-file JSON with snapshot history
├── envault.pid        — daemon PID (when running)
└── envault.log        — daemon log output
```

## Commit & Release Workflow

1. **Build and verify**: `make build`
2. **Commit** with conventional prefix: `feat:`, `fix:`, `docs:`, `refactor:`, `release:`
3. **Push**: `git push origin develop`
4. **Release** (when asked):
   - Check latest tag: `git tag --list 'v*' --sort=-v:refname | head -1`
   - Tag and push: `git tag v<NEXT> && git push origin v<NEXT>`
   - GitHub Actions builds 4 binaries + checksums and publishes a Release
5. **Update local binary**: `make install`

## Code Conventions

- **Single-file CLI**: all commands live in `cmd/root.go`
- **Internal packages**: everything under `internal/` — not importable externally
- **File permissions**: `0600` for sensitive files, `0700` for directories
- **Skip dirs**: node_modules, .git, .svn, .hg, vendor, __pycache__, .venv, venv, .tox, dist, build, .envault
- **Env file patterns**: `.env` (exact), `.env.*` (prefix), `*.env` (suffix)
- **Content dedup**: identical file content shares the same blob (SHA-256)

## TUI Architecture (internal/tui/)

Three-view state machine using BubbleTea:
1. **fileListView** — browse tracked files (j/k/↑↓ nav, Enter to open)
2. **historyView** — browse versions for a file (Esc/← to go back)
3. **contentView** — scrollable viewport showing file content

Navigation: `q` quits, `Esc`/`h`/`←` goes back, `Enter`/`l`/`→` opens, `g`/`G` for top/bottom.

## Key Docs

- [README.md](README.md) — install instructions, command reference, feature overview
