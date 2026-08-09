# envault

**Your `.env` files, safely vaulted.**

envault finds every `.env` file on your machine, versions it, and keeps backing
it up in the background. Content is addressed by SHA-256, so identical files are
stored once and an unchanged file costs nothing to re-scan.

Your secrets live in files git is told to ignore — no history, no backup, no way
back when one gets clobbered. envault fixes that, quietly.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/akhshyganesh/envault/develop/install.sh | sh
```

Detects your OS and architecture, verifies the download against the release
checksums, and installs to `/usr/local/bin` — creating it if missing, as on a
stock macOS. Asks for sudo.

To skip sudo, install somewhere you own:

```bash
curl -fsSL https://raw.githubusercontent.com/akhshyganesh/envault/develop/install.sh \
  | ENVAULT_INSTALL_DIR="$HOME/.local/bin" sh
```

From source (Go 1.26+):

```bash
git clone https://github.com/akhshyganesh/envault.git && cd envault
make build && make install    # honours INSTALL_DIR too
```

## Quick start

```bash
envault
```

First run opens the setup wizard: pick the folders to watch and a scan interval,
and it takes a first backup and registers itself to start at login. After that,
plain `envault` opens the terminal UI. Re-run the wizard any time with
`envault install`.

Installing the binary alone doesn't start anything — the login service comes
from the wizard. Check it with `envault status`.

## Interfaces

Three, all doing the same things.

**Terminal UI** — `envault` or `envault ui`.

- `↑↓`/`jk` move · `Enter`/`→` open · `Esc`/`←` back · `g`/`G` top/bottom · `q` quit
- Files: `s` scan · `d` toggle the daemon · `w` watch a folder · `e` export · `i` import
- Versions: `r` restore in place · `R` restore to a path
- Contents: `c` copy · `r` restore · `R` restore to a path

**Browser UI** — `envault web` (`--port`, `--no-open`).

Each version renders as a ledger of `KEY → value`, masked until you reveal them
— one row at a time, or `r` for all. Keys added or changed since the previous
version are marked, so you can see what a snapshot actually did. `t` shows the
verbatim file, `/` filters, `?` lists every key.

It binds `127.0.0.1`, rejects any request whose `Host` isn't loopback, and needs
the one-time token from the URL it prints. It does serve your secrets in the
clear over local HTTP — leave it running only while you're using it.

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
| `start` / `stop` | Run or stop the background daemon |
| `status` | Daemon, login service and watch list at a glance |
| `install` | Interactive setup, including the login service |
| `uninstall [--prune] [--all]` | Remove the service, and optionally the backups and the binary |
| `export [-o file.zip]` | Export the whole vault as a zip |
| `import <file.zip> [--force]` | Restore a vault from a zip |
| `peek <file.zip> [file\|#]` | Read a zip without importing it |
| `upgrade` | Self-update from the latest GitHub release |
| `version` | Print the version and build info |
| `ui` | Browse backups in the terminal |
| `web [-p port] [--no-open]` | Serve the browser UI on localhost |

Anywhere a file is expected you can pass the `#` from `envault list` instead of
a path.

## Removing envault

```bash
envault uninstall           # service only; then asks about backups and the binary
envault uninstall --prune   # service and backups
envault uninstall --all     # service, backups and the binary
```

Nothing is deleted without being asked for, and whatever survives is named at
the end — so "fully uninstalled" never means less than it says.

## How it works

```
~/.envault/
├── config.json      — watch directories, interval, versions to keep
├── blobs/           — file contents, each named by its SHA-256 (deduplicated)
├── index/           — one JSON file of version history per tracked file
├── envault.pid      — daemon PID (runtime)
└── envault.log      — daemon log (runtime)
```

The scanner walks your watch directories for `.env`, `.env.*` and `*.env`,
skipping `node_modules`, `.git`, `vendor`, `dist`, `build` and similar. The store
hashes each file — same content, same hash, same blob, no new version. The
daemon repeats that every 60 seconds by default.

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

- `max_versions: 0` keeps unlimited history. A positive number keeps the N
  newest versions per file; older ones are dropped on the next backup.
- Dropping a version doesn't free disk space, because the same content may be
  shared with another file. Run `envault gc` to reclaim it.
- Keep the watch list to the folders you actually keep code in. Pointing it at
  your whole home directory makes the daemon walk macOS-protected folders it
  can never read, and the log fills with permission warnings.

## Moving between machines

```bash
envault export -o ~/backup.zip    # before wiping the old machine
envault import ~/backup.zip       # on the new one
```

Need one file out of a backup without importing it? `peek` reads the zip in
place and never touches `~/.envault`:

```bash
envault peek ~/backup.zip                 # what's inside
envault peek ~/backup.zip 3               # version history for file #3
envault peek ~/backup.zip 3 --show -v 2   # print version 2
envault peek ~/backup.zip 3 -o ./.env     # pull one version out
```

## License

MIT — made by [Akhshy](https://www.youtube.com/@code_wid_mapla)
