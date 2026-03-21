# envault

**Your `.env` files, safely vaulted.**

envault is a lightweight background daemon that automatically discovers, versions, and backs up all `.env` files on your machine. Accidentally deleted your `.env.production`? Formatted your drive? Just `envault restore` and you're back.

## Features

- **Auto-discovery** — Finds all `.env`, `.env.local`, `.env.production`, `.env.*` files recursively
- **Git-like versioning** — Content-addressed storage with SHA-256, deduplicates identical content
- **Background daemon** — Runs silently, scans periodically, zero maintenance
- **Instant restore** — Recover any version of any `.env` file with one command
- **Cross-platform** — Works on macOS (launchd) and Ubuntu/Linux (systemd)
- **Simple CLI** — No config files needed, sensible defaults, just works
- **Lightweight** — Single binary, no dependencies, minimal disk usage

## Install

### From Source

```bash
git clone https://github.com/akhshyganesh/envault.git
cd envault
go build -o envault .
sudo mv envault /usr/local/bin/
```

### Quick Start

```bash
# 1. Initialize (creates ~/.envault/)
envault init

# 2. Run your first scan
envault scan

# 3. Install as startup service (runs on boot)
envault install

# Done! envault will now protect your .env files automatically.
```

## Usage

### Scan for .env files

```bash
# Scan configured directories (default: home directory)
envault scan

# Scan specific directories
envault scan ~/projects ~/work
```

### List tracked files

```bash
envault list
# FILE                                 VERSIONS  LAST BACKUP
# ~/projects/api/.env                  5         2h ago
# ~/projects/api/.env.production       3         1d ago
# ~/projects/web/.env.local            2         5m ago
```

### View version history

```bash
envault history ~/projects/api/.env
# History for ~/projects/api/.env (5 versions):
#
# #  ID            DATE              SIZE    COMMENT
# 1  4237d8ff62eb  2026-03-20 09:15  256B    auto
# 2  a1b2c3d4e5f6  2026-03-20 14:22  312B    auto
# 3  d1f201bece7b  2026-03-21 10:00  298B    auto
# ...
```

### Restore a file

```bash
# Restore latest version
envault restore ~/projects/api/.env

# Restore a specific version
envault restore ~/projects/api/.env --version 2
```

### View file contents without restoring

```bash
# Show latest backed-up content
envault show ~/projects/api/.env

# Show a specific version
envault show ~/projects/api/.env --version 1
```

### Manage watched directories

```bash
# Add a directory to watch
envault watch ~/new-project

# The daemon will scan this directory on its next cycle
```

### Daemon control

```bash
# Start the daemon (foreground)
envault start

# Check if daemon is running
envault status

# Stop the daemon
envault stop
```

### Startup service

```bash
# Install as startup service
# macOS: creates ~/Library/LaunchAgents/com.envault.daemon.plist
# Linux: creates ~/.config/systemd/user/envault.service
envault install

# Remove startup service
envault uninstall
```

## How It Works

```
~/.envault/
├── config.json          # Watch directories, scan interval
├── blobs/               # Content-addressed file storage (SHA-256)
│   ├── 4237d8ff62eb...  # Actual file contents
│   └── d1f201bece7b...
├── index/               # Per-file version history (JSON)
│   ├── a1b2c3d4.json   # Maps file path → list of snapshots
│   └── e5f6a7b8.json
├── envault.pid          # Daemon PID file
└── envault.log          # Daemon log file
```

1. **Scanner** walks your configured directories, skipping `node_modules`, `.git`, `vendor`, etc.
2. **Store** hashes file contents with SHA-256 — if the content hasn't changed, no new snapshot is created
3. **Index** maintains a per-file history of all snapshots with timestamps
4. **Daemon** runs the scanner on a configurable interval (default: 60s)

## Configuration

Config lives at `~/.envault/config.json`:

```json
{
  "watch_dirs": ["/Users/you"],
  "scan_interval_secs": 60,
  "max_versions": 0
}
```

| Field | Description | Default |
|-------|-------------|---------|
| `watch_dirs` | Directories to scan recursively | Home directory |
| `scan_interval_secs` | Seconds between daemon scans | 60 |
| `max_versions` | Max snapshots per file (0 = unlimited) | 0 |

## Project Structure

```
.
├── main.go                     # Entry point
├── cmd/
│   └── root.go                 # CLI commands (cobra)
├── internal/
│   ├── config/config.go        # Configuration management
│   ├── store/store.go          # Versioned content-addressed storage
│   ├── scanner/scanner.go      # .env file discovery
│   └── daemon/
│       ├── daemon.go           # Background daemon loop
│       └── service.go          # OS service install (launchd/systemd)
├── go.mod
└── README.md
```

## License

MIT
