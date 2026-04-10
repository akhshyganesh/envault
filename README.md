# envault

**Your `.env` files, safely vaulted.**

envault is a CLI tool and background daemon that automatically discovers, versions, and backs up every `.env` file on your machine. It runs silently, creating SHA-256 content-addressed snapshots so you can restore any version with a single command.

## Why

Environment files hold secrets — API keys, database passwords, tokens. They're excluded from git, which means no history, no backup, no recovery if something goes wrong.

envault solves all three. Silently. Automatically.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/akhshyganesh/envault/develop/install.sh | sh
```

This auto-detects your OS and architecture, downloads the correct binary, and installs it to `/usr/local/bin` (you'll be prompted for your sudo password).

**Build from source** (requires Go 1.22+):
```bash
git clone https://github.com/akhshyganesh/envault.git && cd envault
make build && sudo make install
```

## Quick Start

```bash
envault
```

That's it. On first run, envault detects your project directories and walks you through setup — watch directories, scan interval, and startup service — then backs everything up immediately.

After setup, running `envault` opens the interactive file browser. Use `envault install` to re-run the wizard at any time.

## Commands

| Command | Description |
|---------|-------------|
| `init` | Create `~/.envault/` and default config |
| `scan [dirs...]` | Discover and back up .env files (defaults to watch dirs) |
| `list` / `ls` | List tracked files with index numbers |
| `history <file\|#>` | Show version history for a file |
| `show <file\|#> [-v N]` | Print a backed-up version to stdout |
| `restore <file\|#> [-v N] [-o path]` | Restore a file from backup |
| `watch <dir>` | Add a directory to the watch list |
| `start` / `stop` / `status` | Control the background daemon |
| `install` | Interactive setup + install as OS startup service |
| `uninstall [--prune]` | Remove the service (optionally delete all backups) |
| `export [-o file.zip]` | Export entire vault as a zip archive |
| `import <file.zip> [--force]` | Import vault from a zip archive |
| `upgrade` | Self-update to the latest GitHub release |
| `version` | Print version and build info |
| `ui` | Launch interactive TUI browser |

Commands accepting `<file|#>` work with either the file path or the `#` index from `envault list`.

## How It Works

```
~/.envault/
├── config.json      — watch directories and scan interval
├── blobs/           — file contents stored by SHA-256 hash (deduplicated)
├── index/           — per-file version history as JSON
├── envault.pid      — daemon PID (runtime)
└── envault.log      — daemon log (runtime)
```

1. **Scanner** walks configured directories, matching `.env`, `.env.*`, and `*.env` files — skipping `node_modules`, `.git`, `vendor`, `dist`, `build`, and similar noise.
2. **Store** hashes each file's content. Identical content is never stored twice.
3. **Daemon** reruns the scanner every 60 seconds (configurable), creating a new snapshot only when content changes.

## Configuration

`~/.envault/config.json`:

```json
{
  "watch_dirs": ["~/projects", "~/work"],
  "scan_interval_secs": 60,
  "max_versions": 0
}
```

Edit directly or use `envault watch <dir>` to add directories. `max_versions: 0` means unlimited.

## Migration

```bash
# Before formatting / switching machines
envault export -o ~/backup.zip

# After fresh install
envault import ~/backup.zip
envault list   # verify everything is restored
```

## License

MIT — Made with ❤ by [Akhshy](https://www.youtube.com/@code_wid_mapla)
