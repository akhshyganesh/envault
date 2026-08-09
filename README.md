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

This auto-detects your OS and architecture, downloads the correct binary, verifies it against
the release checksums, and installs it to `/usr/local/bin` — creating that directory if your
system doesn't have it (you'll be prompted for your sudo password).

To install somewhere you own and skip sudo entirely, set `ENVAULT_INSTALL_DIR`:

```bash
curl -fsSL https://raw.githubusercontent.com/akhshyganesh/envault/develop/install.sh \
  | ENVAULT_INSTALL_DIR="$HOME/.local/bin" sh
```

**Build from source** (requires Go 1.26+):
```bash
git clone https://github.com/akhshyganesh/envault.git && cd envault
make build && make install
```

`make install` also honours `INSTALL_DIR`: `make install INSTALL_DIR="$HOME/.local/bin"`.

## Quick Start

```bash
envault
```

That's it. On first run, envault detects your project directories and walks you through setup — watch directories, scan interval, and startup service — then backs everything up immediately.

After setup, running `envault` opens the interactive file browser. Use `envault install` to re-run the wizard at any time.

## Interactive UI

`envault` and `envault ui` open a terminal interface styled like a modern command tool — a
warm amber accent on neutral grays, breadcrumb navigation (`envault › ~/app/.env › v3`),
and a status bar where every shortcut key is highlighted. Browse tracked files, drill into a
file's version history, and view any snapshot's contents without leaving the keyboard.

From the file list you can scan (`s`), toggle the daemon (`d`), add a watch dir (`w`), and
export/import (`e`/`i`). In history, `r` restores in place and `R` restores to a chosen path;
in the content view, `c` copies to the clipboard. `Enter`/`→` drills in, `Esc`/`←` goes back,
`q` quits.

## Browser UI

Prefer clicking to typing? `envault web` starts a small local server and opens it in your
browser:

```bash
envault web              # opens http://127.0.0.1:7391 automatically
envault web --port 8080 --no-open
```

The page does everything the terminal does — browse tracked files and their versions, view
and copy contents, restore in place or to a chosen path, forget a file, scan, add watch
directories, edit settings, start/stop the daemon, reclaim space, export a zip (downloads to
your browser), import one, and peek inside a backup zip read-only.

Each version renders as a ledger of `KEY` and value rather than a wall of text. Values are
masked until you reveal them — one row at a time, or `r` for all of them — and keys that
changed since the previous version are marked, so you can see what a snapshot actually did.
Press `t` for the verbatim file, `/` to filter, and `?` for the full keyboard map; the
motions match the terminal UI. Drag the divider to resize the file list.

It binds to `127.0.0.1` only and every request needs the one-time token in the URL it prints,
so nothing else on your network — or on a web page you happen to be visiting — can reach it.
It serves your `.env` contents in the clear over local HTTP, so leave it running only while
you're using it, and press Ctrl+C when you're done.

## Commands

| Command | Description |
|---------|-------------|
| `init` | Create `~/.envault/` and default config |
| `scan [dirs...]` | Discover and back up .env files (defaults to watch dirs) |
| `list` / `ls` | List tracked files with index numbers |
| `history <file\|#>` | Show version history for a file |
| `show <file\|#> [-v N]` | Print a backed-up version to stdout |
| `restore <file\|#> [-v N] [-o path]` | Restore a file from backup |
| `forget <file\|#>` | Stop tracking a file and delete its history |
| `gc` | Reclaim disk space by deleting unreferenced blobs |
| `watch <dir>` | Add a directory to the watch list |
| `start` / `stop` / `status` | Control the background daemon |
| `install` | Interactive setup + install as OS startup service |
| `uninstall [--prune]` | Remove the service (optionally delete all backups) |
| `export [-o file.zip]` | Export entire vault as a zip archive |
| `import <file.zip> [--force]` | Import vault from a zip archive |
| `peek <file.zip> [file\|#]` | Browse an exported zip read-only, without importing it |
| `upgrade` | Self-update to the latest GitHub release |
| `version` | Print version and build info |
| `ui` | Launch interactive TUI browser |
| `web [-p port] [--no-open]` | Serve the browser UI on localhost |

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

Edit directly or use `envault watch <dir>` to add directories.

- `max_versions: 0` keeps unlimited history. Set it to a positive number to keep only the
  N newest snapshots per file — older ones are pruned automatically on the next backup.
- After pruning (or `envault forget`), run `envault gc` to delete the now-unreferenced
  content blobs and reclaim disk space.

## Migration

```bash
# Before formatting / switching machines
envault export -o ~/backup.zip

# After fresh install
envault import ~/backup.zip
envault list   # verify everything is restored
```

Need something out of a backup without importing it? `peek` reads the zip in place and
never touches `~/.envault`:

```bash
envault peek ~/backup.zip                 # what's inside
envault peek ~/backup.zip 3               # version history for file #3
envault peek ~/backup.zip 3 --show -v 2   # print version 2
envault peek ~/backup.zip 3 -o ./.env     # pull one version out
```

## License

MIT — Made with ❤ by [Akhshy](https://www.youtube.com/@code_wid_mapla)
