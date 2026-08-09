---
description: "envault — .env file backup daemon. Use this for all development tasks on this project."
applyTo: "**"
---

# envault — Development Guide

## What it does

CLI tool + background daemon (Go) that auto-discovers, versions, and backs up `.env` files using SHA-256 content-addressed storage. Supports macOS (launchd) and Linux (systemd).

## Stack

- **Language**: Go (`go 1.26.1` in `go.mod`; CI builds with Go `1.26`)
- **CLI**: Cobra (`github.com/spf13/cobra`)
- **TUI**: BubbleTea + Lipgloss + Bubbles (`github.com/charmbracelet/*`)
- **Platforms**: darwin/amd64, darwin/arm64, linux/amd64, linux/arm64

## Project Structure

```
main.go                          — calls cmd.Execute()
cmd/
  root.go                        — rootCmd + Execute(); Version/BuildDate ldflag vars;
                                   no-arg run → setup wizard (first time) or TUI (returning)
  helpers.go                     — resolveFileArg, getLatestVersion
  init.go                        — init command
  scan.go                        — scan command
  vault.go                       — list, history, show commands
  restore.go                     — restore command
  maintenance.go                 — gc, forget commands
  daemon.go                      — start, stop, status, watch commands
  install.go                     — install, uninstall commands
  transfer.go                    — export, import commands
  peek.go                        — peek command (read-only archive browsing)
  upgrade.go                     — version, upgrade commands
  ui.go                          — ui command
  web.go                         — web command (local browser UI server)
internal/
  config/config.go               — Config struct, Load/Save, VaultDir, AddWatchDir
  format/format.go               — ExpandPath, ShortenPath, TimeAgo, HumanSize (shared by cmd + tui)
  store/store.go                 — SHA-256 blob store + JSON index; SaveSnapshot, GetHistory,
                                   RestoreSnapshot, ListTrackedFiles, GC, Forget
  scanner/scanner.go             — recursive .env discovery: ScanDirectory, ScanDirectories
  daemon/daemon.go               — background loop, PID management
  daemon/service.go              — launchd/systemd service install
  setup/setup.go                 — interactive install wizard (3-step BubbleTea)
  transfer/transfer.go           — vault export/import (zip); owns sanitizeExtractPath
  transfer/archive.go            — read-only reader for an export zip (used by peek)
  tui/tui.go                     — interactive TUI browser (BubbleTea state machine)
  web/server.go                  — loopback HTTP server + JSON API behind a session token
  web/index.html                 — single-page browser UI (embedded, no external assets)
install.sh                       — curl-able installer that fetches a release binary
.github/workflows/ci.yml         — fmt + vet + race tests + build, on push/PR
.github/workflows/release.yml    — build 4 binaries + checksums, publish on v* tags
```

## Build

```bash
make build     # go build with -ldflags injecting cmd.Version + cmd.BuildDate → ./envault
make test      # go test ./...
make install   # build, then install into $(INSTALL_DIR), default /usr/local/bin
make uninstall # remove the binary from $(INSTALL_DIR)
make run       # build, then ./envault scan .
make clean     # rm -f ./envault
```

`VERSION` defaults to `git describe --tags --always --dirty`, falling back to `dev`. A plain
`go build .` produces a binary reporting version `dev` — use `make build` when the version
string matters.

`install`/`uninstall` create `$(INSTALL_DIR)` if it is missing (a stock macOS has no
`/usr/local/bin`, and copying into a missing directory fails with a bare "No such file or
directory") and reach for `sudo` only when the nearest existing ancestor isn't writable —
so `make install INSTALL_DIR=$HOME/.local/bin` never leaves root-owned files. `install.sh`
follows the same rules, keyed off `ENVAULT_INSTALL_DIR`, and verifies the download against
the release `checksums.txt` before writing anything.

## Commands

| Command | Flags | Purpose |
|---------|-------|---------|
| `init` | — | Create `~/.envault/` with defaults |
| `scan [dirs...]` | — | Discover and back up .env files |
| `list` / `ls` | — | Show tracked files with numbered index |
| `history <file\|#>` | — | Show version history |
| `show <file\|#>` | `-v/--version` | Print backed-up content to stdout |
| `restore <file\|#>` | `-v/--version`, `-o/--output` | Restore from backup |
| `forget <file\|#>` | — | Stop tracking a file, delete its history |
| `gc` | — | Delete unreferenced blobs to reclaim disk |
| `watch <dir>` | — | Add directory to watch list |
| `start` / `stop` / `status` | — | Control background daemon |
| `install` | — | Interactive TUI wizard — dirs, interval, scan, service install |
| `uninstall` | `--prune` | Remove OS service (optionally delete all data) |
| `export` | `-o/--output` | Export vault as zip |
| `import <zip>` | `--force` | Import vault from zip |
| `peek <zip> [file\|#]` | `-v/--version`, `--show`, `-o/--out`, `--force` | Browse an export zip read-only; never writes to the vault |
| `upgrade` | — | Self-update from GitHub |
| `version` | — | Print version + credits |
| `ui` | — | Launch interactive TUI |
| `web` | `-p/--port` (7391), `--no-open` | Serve the browser UI on 127.0.0.1 (token-authenticated) |

`<file|#>` accepts either a file path or the numeric index shown by `envault list`.
Note `peek`'s output flag is `--out`, not `--output` — it is the one exception.

## Storage Layout

```
~/.envault/
├── config.json      — WatchDirs, ScanIntervalSecs, MaxVersions
├── blobs/           — content files named by SHA-256 hash (deduplicated)
├── index/           — per-file JSON with snapshot history
├── envault.pid      — daemon PID (runtime only)
└── envault.log      — daemon log (runtime only)
```

`config.DefaultConfig()`: `WatchDirs` = `[$HOME]`, `ScanIntervalSecs` = `60`,
`MaxVersions` = `0` (unlimited). All paths derive from `os.UserHomeDir()`, so tests can
relocate the whole vault with `t.Setenv("HOME", t.TempDir())`.

## Code Conventions

- **Permissions**: `0600` for files, `0700` for directories — everything under `~/.envault/`
  and every restored/extracted `.env`. Two deliberate exceptions: OS service definitions
  (`0644` plist/unit in `0755` dirs, since launchd/systemd must read them) and the
  downloaded upgrade binary (`0755`, it must be executable).
- **Error wrapping**: `fmt.Errorf("context: %w", err)` throughout
- **Best-effort exec calls**: use `_ = exec.Command(...).Run()` with a comment
- **Env file patterns**: `.env`, `.env.*`, `*.env`
- **Skip dirs**: `node_modules`, `.git`, `.svn`, `.hg`, `vendor`, `__pycache__`, `.venv`, `venv`, `.tox`, `dist`, `build`, `.envault`
- **Deduplication**: same content → same blob, no duplicate snapshots
- **Version pruning**: `max_versions` > 0 trims each file's history to the N newest snapshots; `envault gc` reclaims the orphaned blobs

## Testing

`go test ./...`. Tests live beside the code as `*_test.go`; today that means
`internal/format`, `internal/scanner`, `internal/store`, `internal/transfer`,
`internal/tui`, `internal/web`, and `cmd` (upgrade preflight only). `internal/config`,
`internal/daemon`, and `internal/setup` have no tests yet — new work in those packages
should add them rather than inherit the gap.

Tests must never touch the real vault: set `t.Setenv("HOME", t.TempDir())` before calling
anything that resolves `config.VaultDir()`, and use `t.TempDir()` for scanned fixtures.

## UI Style

Both the TUI and the setup wizard share one identity (`internal/tui` and `internal/setup`):
a warm amber accent (256-color `180`/`215`) on neutral grays, a 🔒 brand mark, breadcrumb
headers over a hairline rule, an accent gutter bar (`▌`) for the selected row, and a status
bar whose hint keys are rendered in the accent color. Palette lives in the `col*` constants
at the top of each package's style block — change colors there, not at call sites.

The browser UI deliberately does **not** share that identity. `internal/web/index.html` is
a light interface (`--page: #fbfbfa` on `--text: #1b1f23`) with a single blue accent
(`--accent: #2563eb`) reserved for selection, focus, and the active version — never for
decoration. Key names in the ledger stay neutral so color always means "this one". Green
(`--green`) and amber (`--amber`) are semantic only: a key added or changed since the
previous version. Every value lives in `:root`; change colors there, not at call sites.

## TUI Views

Four-view state machine (`fileListView → historyView → contentView`, plus a modal
`inputView` for paths and confirmations; `inputView` returns to whichever view opened it
via `m.prevView`):
- `↑↓` / `jk` — navigate · `Enter` / `l` / `→` — open · `Esc` / `h` / `←` / `Backspace` — back
- `g` / `Home` — top · `G` / `End` — bottom · `q` / `Ctrl+C` — quit
- File list actions: `s` scan · `d` toggle daemon · `w` watch dir · `e` export · `i` import
- History actions: `r` restore · `R` restore to path
- Content view: `c` copy · `r` restore · `R` restore to path

## Browser UI

`envault web` serves `internal/web` on 127.0.0.1 with a random per-session token required
in the `X-Envault-Token` header (the page bootstraps it from `?token=`). Non-loopback
`Host` headers are rejected to block DNS rebinding, and responses are `no-store`. The
handlers delegate to the same `store`/`config`/`scanner`/`daemon`/`transfer` calls the CLI
uses — add behavior there, not in the HTTP layer. The page builds its DOM node by node
(no `innerHTML`), so `.env` contents can never be interpreted as markup.

Layout is an app shell, not a document: `body` is a `100dvh` grid with `overflow: hidden`,
and only the two inner `.scroll` panes move. Every nested grid track carries `min-height: 0`
— drop it and the panes stop scrolling and the page grows instead. The divider between them
is a real `<button role="separator">` that drags, takes arrow keys, and persists its width
to `localStorage`.

The ledger renders each version as `KEY → value` rows with values masked by a **fixed-width**
dot string — the mask length must never track the secret's length. Reveal is per row (or `r`
for all) and resets on every version change. `t` shows the verbatim file, which is the escape
hatch for anything the dotenv parser counts as unreadable rather than silently dropping.

## Release

```bash
git tag v<X.Y.Z>
git push origin v<X.Y.Z>
# release.yml: `release` job cross-compiles 4 static binaries (CGO_ENABLED=0, -s -w,
# version = the tag), then `publish` collects them, writes checksums.txt, cuts the Release
```

## CI

Two workflows:

- **`ci.yml`** — on push and PR to `develop`/`main`: `gofmt -l` check, `go vet ./...`,
  `go test -race ./...`, then `make build`. Run these locally before pushing; a `gofmt`
  miss fails the build.
- **`release.yml`** — on `v*` tags only, as described above. It does **not** run tests, so
  a tag inherits whatever `ci.yml` last verified on the branch.
