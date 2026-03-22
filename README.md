# envault

**Your `.env` files, safely vaulted.**

envault is a lightweight CLI tool and background daemon that automatically discovers, versions, and backs up all `.env` files on your machine. It runs silently in the background, scanning your project directories and creating versioned snapshots of every environment file it finds.

Accidentally deleted your `.env.production`? Formatted your drive? Just `envault restore` and you're back.

## Why envault?

Environment files hold secrets — API keys, database passwords, service credentials. They're excluded from git (`.gitignore`), which means:

- **No version history** — change a value wrong? Gone forever.
- **No backup** — delete it by accident? Start from scratch.
- **No recovery** — format your machine? Good luck remembering every key.

envault solves all three. Silently. Automatically.

## Features

- **Auto-discovery** — Finds `.env`, `.env.local`, `.env.production`, `.env.*`, `*.env`, and any variant recursively
- **Git-like versioning** — Content-addressed storage with SHA-256; identical content is never stored twice
- **Background daemon** — Runs silently on a configurable interval, zero maintenance
- **Instant restore** — Recover any version of any `.env` file with one command
- **Cross-platform** — Native startup support for macOS (launchd) and Ubuntu/Linux (systemd)
- **Interactive setup** — `envault install` asks which directories to watch — no manual config editing
- **Clean uninstall** — `envault uninstall --prune` removes everything, or keep backups when removing the service
- **Portable backup** — Export/import your entire vault as a zip for system migrations
- **Lightweight** — Single ~5MB binary, no runtime dependencies

## Install

### macOS

```bash
# Apple Silicon (M1/M2/M3/M4)
curl -L https://github.com/akhshyganesh/envault/releases/latest/download/envault-darwin-arm64 -o envault
sudo install -m 755 envault /usr/local/bin/envault

# Intel Mac
curl -L https://github.com/akhshyganesh/envault/releases/latest/download/envault-darwin-amd64 -o envault
sudo install -m 755 envault /usr/local/bin/envault
```

### Ubuntu / Linux

```bash
# x86_64
curl -L https://github.com/akhshyganesh/envault/releases/latest/download/envault-linux-amd64 -o envault
sudo install -m 755 envault /usr/local/bin/envault

# ARM64 (e.g. Raspberry Pi, AWS Graviton)
curl -L https://github.com/akhshyganesh/envault/releases/latest/download/envault-linux-arm64 -o envault
sudo install -m 755 envault /usr/local/bin/envault
```

### Build From Source

Requires [Go 1.22+](https://go.dev/dl/).

```bash
git clone https://github.com/akhshyganesh/envault.git
cd envault
go build -o envault .
sudo mv envault /usr/local/bin/
```

## Quick Start

```bash
# 1. Initialize envault (creates ~/.envault/)
envault init

# 2. Scan and backup all .env files now
envault scan

# 3. Install as a startup service — it will ask which directories to watch
envault install
#   Which directories should envault watch for .env files?
#   Enter one directory per line. Press Enter on an empty line when done.
#
#   [1] Directory path (or Enter to finish): ~/projects
#       ✓ Added /Users/you/projects
#   [2] Directory path (or Enter to finish):
#
#   ✓ Installed launchd service
#   envault will start automatically on login.

# Done! Your .env files are now protected automatically.
```

## Commands

### `envault init`

Creates the `~/.envault/` directory and default config.

```bash
envault init
```

### `envault scan [directories...]`

Discovers all `.env` files in the given directories (or configured watch dirs) and creates versioned backups.

```bash
# Scan configured watch directories
envault scan

# Scan specific directories
envault scan ~/projects ~/work /opt/apps
```

### `envault list` (alias: `envault ls`)

Shows all tracked `.env` files with version count and last backup time.

```bash
envault list
# FILE                                 VERSIONS  LAST BACKUP
# ~/projects/api/.env                  5         2h ago
# ~/projects/api/.env.production       3         1d ago
# ~/projects/web/.env.local            2         5m ago
```

### `envault history <file>`

Shows version history for a specific `.env` file.

```bash
envault history ~/projects/api/.env
# #  ID            DATE              SIZE    COMMENT
# 1  4237d8ff62eb  2026-03-20 09:15  256B    auto
# 2  a1b2c3d4e5f6  2026-03-20 14:22  312B    auto
# 3  d1f201bece7b  2026-03-21 10:00  298B    auto
```

### `envault restore <file> [--version N]`

Restores a `.env` file from backup. Defaults to the latest version.

```bash
# Restore latest version
envault restore ~/projects/api/.env

# Restore a specific version
envault restore ~/projects/api/.env --version 2
```

### `envault show <file> [--version N]`

Prints the content of a backed-up version to stdout without modifying any files.

```bash
# Show latest backed-up content
envault show ~/projects/api/.env

# Show a specific version
envault show ~/projects/api/.env --version 1
```

### `envault watch <directory>`

Adds a directory to the watch list in config. The daemon picks it up on the next scan cycle.

```bash
envault watch ~/new-project
# ✓ Now watching /Users/you/new-project
```

### `envault start` / `envault stop` / `envault status`

Manually control the background daemon.

```bash
envault start    # Start daemon (foreground)
envault status   # Check if running
envault stop     # Stop the daemon
```

### `envault install`

Installs envault as a startup service. Interactively asks which directories to watch.

- **macOS**: Creates `~/Library/LaunchAgents/com.envault.daemon.plist` (runs on login)
- **Linux**: Creates `~/.config/systemd/user/envault.service` (runs via systemd user service)

```bash
envault install
```

### `envault uninstall [--prune]`

Removes the startup service. Asks whether to delete all backup data.

```bash
# Remove service, keep backups
envault uninstall

# Remove service AND delete all data
envault uninstall --prune
```

### `envault export [--output FILE]`

Exports your entire envault vault (all backups, version history, and config) as a portable zip archive. Use this before formatting your system or migrating to a new machine.

```bash
# Export with auto-generated timestamped filename
envault export
# ✓ Exported 66 files to /Users/you/envault-backup-20260322-143015.zip (34.4KB)

# Export to a specific path
envault export -o ~/Desktop/my-envault-backup.zip
```

### `envault import <zipfile> [--force]`

Restores envault data from a previously exported zip archive. Use this after a fresh OS install.

```bash
# Import on a fresh system
envault import envault-backup-20260322-143015.zip
# ✓ Imported 66 files to ~/.envault

# Overwrite existing vault data
envault import backup.zip --force
```

**Migration workflow:**
1. Before formatting: `envault export -o /Volumes/USB/envault-backup.zip`
2. After fresh install: Install envault, then `envault import /Volumes/USB/envault-backup.zip`
3. Run `envault list` to verify all backups are restored

### `envault upgrade`

Upgrades envault to the latest GitHub release. Automatically detects your OS and architecture.

```bash
envault upgrade
# Checking for latest version...
#   Current: v0.4.0
#   Latest:  v0.5.0
# Downloading envault-darwin-arm64...
#
# ✓ Upgraded envault to v0.5.0
```

If running from `/usr/local/bin/`, you may need `sudo envault upgrade`.

### `envault version`

Prints version info and credits.

```bash
envault version
# Also works: envault --version
```

## How It Works

```
~/.envault/
├── config.json          # Watch directories, scan interval
├── blobs/               # Content-addressed file storage (SHA-256)
│   ├── 4237d8ff62eb...  # Actual file contents (deduplicated)
│   └── d1f201bece7b...
├── index/               # Per-file version history (JSON)
│   ├── a1b2c3d4.json   # Maps original file path → list of snapshots
│   └── e5f6a7b8.json
├── envault.pid          # Daemon PID file
└── envault.log          # Daemon log
```

1. **Scanner** walks configured directories recursively, skipping `node_modules`, `.git`, `vendor`, `build`, etc.
2. **Store** hashes file contents with SHA-256. If the hash matches the latest snapshot, nothing is stored (deduplication).
3. **Index** maintains a per-file history of snapshots with timestamps, sizes, and content hashes.
4. **Daemon** runs the scanner on a configurable interval (default: 60 seconds).

### What files does it back up?

Any file matching these patterns:

| Pattern | Examples |
|---------|----------|
| `.env` | `.env` |
| `.env.*` | `.env.local`, `.env.production`, `.env.sample`, `.env.development` |
| `*.env` | `production.env`, `staging.env`, `app.env` |

### What directories does it skip?

`node_modules`, `.git`, `.svn`, `.hg`, `vendor`, `__pycache__`, `.venv`, `venv`, `.tox`, `dist`, `build`, `.envault`

## Configuration

Config lives at `~/.envault/config.json`:

```json
{
  "watch_dirs": ["/Users/you/projects", "/Users/you/work"],
  "scan_interval_secs": 60,
  "max_versions": 0
}
```

| Field | Description | Default |
|-------|-------------|---------|
| `watch_dirs` | Directories to scan recursively | Home directory |
| `scan_interval_secs` | Seconds between daemon scans | `60` |
| `max_versions` | Max snapshots per file (`0` = unlimited) | `0` |

You can edit this file directly, or use `envault watch <dir>` to add directories.

## Project Structure

```
.
├── main.go                     # Entry point
├── cmd/
│   └── root.go                 # All CLI commands (cobra)
├── internal/
│   ├── config/config.go        # Configuration management
│   ├── store/store.go          # Versioned content-addressed storage
│   ├── scanner/scanner.go      # .env file discovery and pattern matching
│   └── daemon/
│       ├── daemon.go           # Background daemon loop
│       └── service.go          # OS service install (launchd/systemd)
├── scripts/
│   └── build-release.sh        # Cross-compile for all platforms
├── .github/
│   └── workflows/
│       └── release.yml         # CI/CD: auto-build + publish on git tag
├── go.mod
└── README.md
```

## Contributing

```bash
git clone https://github.com/akhshyganesh/envault.git
cd envault
go build -o envault .
./envault --help
```

## License

MIT
