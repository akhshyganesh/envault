# envault

**Your `.env` files, safely vaulted.**

envault finds every `.env` file on your machine, versions it, and keeps backing it
up in the background. Content is stored by SHA-256 hash, so identical files are
stored once and an unchanged file costs nothing to re-scan.

## Why

Environment files hold your secrets — API keys, database passwords, tokens. They
are excluded from git by design, which means no history, no backup, and no way
back when one gets clobbered.

envault fixes all three, quietly.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/akhshyganesh/envault/develop/install.sh | sh
```

This detects your OS and architecture, verifies the download against the release
checksums, and installs to `/usr/local/bin` — creating that directory if your
system doesn't have one (a stock macOS doesn't). You'll be asked for your sudo
password.

To install somewhere you own and skip sudo entirely:

```bash
curl -fsSL https://raw.githubusercontent.com/akhshyganesh/envault/develop/install.sh \
  | ENVAULT_INSTALL_DIR="$HOME/.local/bin" sh
```

**From source** (Go 1.26+):

```bash
git clone https://github.com/akhshyganesh/envault.git && cd envault
make build && make install
```

`make install` honours the same override: `make install INSTALL_DIR="$HOME/.local/bin"`.

## Quick start

```bash
envault
```

On first run that walks you through setup — which folders to watch, how often to
check, and whether to start on login — then takes a first backup. After that,
plain `envault` opens the browser of your backups. Re-run the wizard any time
with `envault install`.

## Interfaces

There are three, and they do the same things.

**Terminal UI** — `envault` or `envault ui`. A warm amber accent on neutral
grays, breadcrumbs (`envault › ~/app/.env › v3`), and a status bar where every
shortcut key is highlighted.

- `↑↓`/`jk` move · `Enter`/`→` open · `Esc`/`←` back · `g`/`G` top/bottom · `q` quit
- Files: `s` scan · `d` toggle the daemon · `w` watch a folder · `e` export · `i` import
- Versions: `r` restore in place · `R` restore to a path
- Contents: `c` copy · `r` restore · `R` restore to a path

**Browser UI** — `envault web`.

```bash
envault web                      # opens http://127.0.0.1:7391
envault web --port 8080 --no-open
```

Each version renders as a ledger of `KEY → value` rather than a wall of text.
Values are masked until you reveal them — one row at a time, or `r` for all —
and keys added or changed since the previous version are marked, so you can see
what a snapshot actually did. `t` shows the verbatim file, `/` filters, `?` lists
every key.

It binds `127.0.0.1` only, rejects any request whose `Host` isn't loopback, and
needs the one-time token from the URL it prints. It does serve your `.env`
contents in the clear over local HTTP, so leave it running only while you're
using it, and press Ctrl+C when you're done.

**Command line** — everything below.

## Commands

| Command | Description |
|---------|-------------|
| `init` | Create `~/.envault/` and a default config |
| `scan [dirs...]` | Find .env files and back them up (defaults to the watch list) |
| `list` / `ls` | List tracked files with their index numbers |
| `history <file\|#>` | Show a file's version history |
| `show <file\|#> [-v N]` | Print a stored version to stdout |
| `restore <file\|#> [-v N] [-o path]` | Restore a file from a backup |
| `forget <file\|#>` | Stop tracking a file and delete its history |
| `gc` | Reclaim disk space by deleting unreferenced content |
| `watch <dir>` | Add a directory to the watch list |
| `start` / `stop` / `status` | Control the background daemon |
| `install` | Interactive setup, including the OS startup service |
| `uninstall [--prune] [--all]` | Remove the service, and optionally the backups and the binary |
| `export [-o file.zip]` | Export the whole vault as a zip |
| `import <file.zip> [--force]` | Restore a vault from a zip |
| `peek <file.zip> [file\|#]` | Read a zip without importing it |
| `upgrade` | Self-update from the latest GitHub release |
| `version` | Print the version and build info |
| `ui` | Browse backups in the terminal |
| `web [-p port] [--no-open]` | Serve the browser UI on localhost |

Anywhere a file is expected you can pass the `#` from `envault list` instead of a
path.

### Removing envault

```bash
envault uninstall           # stops the daemon and removes the startup service,
                            # then asks about your backups and the binary
envault uninstall --prune   # service and backups, keeps the binary
envault uninstall --all     # service, backups and the binary
```

Nothing is deleted without being asked for, and whatever survives is named at
the end — so if it says "fully uninstalled", nothing is left behind.

## How it works

```
~/.envault/
├── config.json      — watch directories, interval, versions to keep
├── blobs/           — file contents, each named by its SHA-256 (deduplicated)
├── index/           — one JSON file of version history per tracked file
├── envault.pid      — daemon PID (runtime)
└── envault.log      — daemon log (runtime)
```

1. The **scanner** walks your watch directories for `.env`, `.env.*` and `*.env`,
   skipping `node_modules`, `.git`, `vendor`, `dist`, `build` and similar noise.
2. The **store** hashes each file. Same content, same hash, same blob — identical
   files are never stored twice, and an unchanged file creates no new version.
3. The **daemon** repeats that every 60 seconds by default.

## Configuration

`~/.envault/config.json`:

```json
{
  "watch_dirs": ["~/projects", "~/work"],
  "scan_interval_secs": 60,
  "max_versions": 0
}
```

Edit it directly, use `envault watch <dir>`, or open Settings in the browser UI.

- `max_versions: 0` keeps unlimited history. A positive number keeps only the N
  newest versions per file; older ones are dropped on the next backup.
- Dropping a version doesn't free disk space, because the same content may be
  shared with another file. Run `envault gc` to reclaim it.

## Moving between machines

```bash
# before wiping the old machine
envault export -o ~/backup.zip

# on the new one
envault import ~/backup.zip
envault list
```

Need one file out of a backup without importing the whole thing? `peek` reads
the zip in place and never touches `~/.envault`:

```bash
envault peek ~/backup.zip                 # what's inside
envault peek ~/backup.zip 3               # version history for file #3
envault peek ~/backup.zip 3 --show -v 2   # print version 2
envault peek ~/backup.zip 3 -o ./.env     # pull one version out
```

## License

MIT — made by [Akhshy](https://www.youtube.com/@code_wid_mapla)
