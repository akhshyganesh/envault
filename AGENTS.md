# envault — Development Guide

## What it does

CLI tool plus background daemon (Go) that discovers, versions and backs up `.env`
files using SHA-256 content-addressed storage. Three front ends over one core:
a CLI, a terminal UI, and a local browser UI. macOS (launchd) and Linux (systemd).

## Stack

- **Language**: Go (`go 1.26.1` in `go.mod`; CI builds with Go `1.26`)
- **CLI**: Cobra
- **TUI**: BubbleTea + Lipgloss + Bubbles
- **Browser UI**: embedded HTML/CSS/JS, no dependencies, no build step
- **Platforms**: darwin/amd64, darwin/arm64, linux/amd64, linux/arm64

## Layout

```
main.go                       — calls cmd.Execute()

cmd/                          — one file per group of commands; all thin
  root.go                     — rootCmd, Execute, Version/BuildDate ldflag vars
  shared.go                   — resolveFileArg, loadHistory, pickSnapshot, tables, output helpers
  init.go  scan.go  list.go  restore.go  maintenance.go
  daemon.go  setup.go  transfer.go  peek.go  web.go  upgrade.go

internal/
  config/
    paths.go                  — every path under ~/.envault derives from here
    config.go                 — Config, Load, Save, AddWatchDir
  store/
    store.go                  — Store, SaveSnapshot, Content, Restore
    history.go                — Snapshot, FileHistory, History, ListTrackedFiles, prune
    gc.go                     — GC, Forget
  scanner/
    match.go                  — isEnvFile, shouldSkipDir
    scanner.go                — ScanDirectory, ScanDirectories
  daemon/
    daemon.go                 — IsRunning, Run, Stop
    service.go                — Install/Uninstall dispatch, template writing
    service_launchd.go        — macOS
    service_systemd.go        — Linux
  transfer/
    transfer.go               — safeExtractPath, runtime-file list
    export.go  import.go      — vault <-> zip
    archive.go                — read-only reader used by peek
  format/format.go            — ExpandPath, ShortenPath, TimeAgo, HumanSize
  ui/theme/theme.go           — the terminal identity, shared by tui and setup
  tui/                        — the interactive browser
    tui.go                    — model, Run, prompt plumbing
    messages.go               — messages and the tea.Cmds that produce them
    update.go                 — key handling and result folding
    view.go                   — rendering
  setup/                      — the three-step wizard
    setup.go  update.go  view.go  install.go
  web/
    server.go                 — Server, token, loopback and auth middleware
    routes.go                 — the whole route table
    api_vault.go  api_peek.go — handlers
    httpx.go                  — JSON, entries, version/file resolution
    assets.go                 — embed.FS
    assets/index.html         — markup and the SVG icon sprite
    assets/app.css            — every colour and metric, as custom properties
    assets/app.js             — all behaviour

install.sh                    — curl-able installer
.github/workflows/ci.yml      — fmt + vet + race tests + build, on push/PR
.github/workflows/release.yml — 4 binaries + checksums, on v* tags
```

## Build

```bash
make build      # -ldflags injects cmd.Version + cmd.BuildDate → ./envault
make check      # what CI runs: gofmt + go vet + go test -race
make test       # go test ./...
make install    # into $(INSTALL_DIR), default /usr/local/bin
make uninstall
make run        # build, then ./envault scan .
make clean
```

`VERSION` defaults to `git describe --tags --always --dirty`, falling back to
`dev`. A plain `go build .` reports version `dev` — use `make build` when the
version string matters.

`install`/`uninstall` create `$(INSTALL_DIR)` when it is missing (a stock macOS
has no `/usr/local/bin`, and copying into a missing directory fails with a bare
"No such file or directory") and reach for `sudo` only when the nearest existing
ancestor isn't writable, so `make install INSTALL_DIR=$HOME/.local/bin` never
leaves root-owned files. `install.sh` follows the same rules via
`ENVAULT_INSTALL_DIR`, and verifies the download against the release
`checksums.txt` before writing anything.

## Commands

| Command | Flags | Purpose |
|---------|-------|---------|
| `init` | — | Create `~/.envault/` with defaults |
| `scan [dirs...]` | — | Discover and back up .env files |
| `list` / `ls` | — | Tracked files with a numbered index |
| `history <file\|#>` | — | Version history |
| `show <file\|#>` | `-v/--version` | Print a version to stdout |
| `restore <file\|#>` | `-v/--version`, `-o/--output` | Restore from a backup |
| `forget <file\|#>` | — | Stop tracking, delete history |
| `gc` | — | Delete unreferenced blobs |
| `watch <dir>` | — | Add to the watch list |
| `start` / `stop` / `status` | — | Control the daemon |
| `install` | — | Setup wizard |
| `uninstall` | `--prune` | Remove the service, optionally the data |
| `export` | `-o/--output` | Vault → zip |
| `import <zip>` | `--force` | Zip → vault |
| `peek <zip> [file\|#]` | `-v/--version`, `--show`, `-o/--out`, `--force` | Read a zip; never writes to the vault |
| `upgrade` | — | Self-update from GitHub |
| `version` | — | Version and build info |
| `ui` | — | Terminal UI |
| `web` | `-p/--port` (7391), `--no-open` | Browser UI on 127.0.0.1 |

`<file|#>` takes a path or the number from `envault list`. Note `peek`'s output
flag is `--out`, not `--output` — the one exception, and there is a test pinning
it.

## Storage

```
~/.envault/
├── config.json      — WatchDirs, ScanIntervalSecs, MaxVersions
├── blobs/           — content, named by SHA-256 (deduplicated)
├── index/           — per-file JSON history
├── envault.pid      — runtime only
└── envault.log      — runtime only
```

`config.DefaultConfig()`: `WatchDirs` = `[$HOME]`, `ScanIntervalSecs` = `60`,
`MaxVersions` = `0` (unlimited). Every path derives from `config/paths.go`, which
derives from `os.UserHomeDir()` — so a test relocates the entire vault with
`t.Setenv("HOME", t.TempDir())`.

## Conventions

- **Permissions**: `0600` files, `0700` directories, everywhere under
  `~/.envault/` and for every restored or extracted `.env`. Two deliberate
  exceptions, both tested: OS service definitions (`0644` in `0755`, because
  launchd and systemd must read them) and the downloaded upgrade binary (`0755`,
  it must execute).
- **Error wrapping**: `fmt.Errorf("context: %w", err)`.
- **Best-effort exec**: `_ = exec.Command(...).Run()` with a comment saying why.
- **Libraries don't print.** `internal/daemon` returns strings for the caller to
  display; a stray `Println` corrupts a BubbleTea alternate screen.
- **Env patterns**: `.env`, `.env.*`, `*.env`.
- **Skipped dirs**: `node_modules`, `.git`, `.svn`, `.hg`, `vendor`,
  `__pycache__`, `.venv`, `venv`, `.tox`, `dist`, `build`, `.envault`.
- **Comments explain why, not what.** Prefer one sentence on a non-obvious
  decision over a restatement of the code.

## Testing

`make check`. Tests live beside the code; every package has them. Two rules:

- Never touch the real vault — `t.Setenv("HOME", t.TempDir())` before anything
  that resolves `config.VaultDir()`, and `t.TempDir()` for fixtures.
- The security properties are pinned by tests, not just by comments: loopback-only
  Host checking, token auth, the page never carrying the token, the page never
  building markup from strings, the fixed-width mask, peek refusing to write into
  the vault, and file permissions.

## Terminal identity

`internal/ui/theme` is the single source: a warm amber accent (256-colour
`180`/`215`) on neutral grays, a breadcrumb over a hairline rule, an accent
gutter bar (`▌`) on the selected row, and a status bar whose hint keys are the
only coloured thing in it. Both `internal/tui` and `internal/setup` render from
it — change a colour or a glyph there, never at a call site.

Glyphs are typographic, not emoji: they must be one cell wide or the box-drawn
layouts break. A test enforces that.

## Browser identity

`internal/web/assets` deliberately does **not** share the terminal's look. It is
a classic desk: warm paper (`--paper: #f7f5f0`) and near-black ink
(`--ink: #1a1a18`), serif for the wordmark and headings, one walnut accent
(`--accent: #7a5c3e`) used *only* for selection, focus and the active version —
never decoration. Green and amber are semantic only: a key added or changed since
the previous version. Every value lives in `:root`.

Icons are inline SVG `<symbol>`s in the sprite at the top of `index.html`,
16×16 strokes on `currentColor`. Add one there and reference it with
`icon("name")`; a test fails if the script names an icon the sprite lacks.

Layout is an app shell, not a document: `body` is a `100dvh` grid with
`overflow: hidden`, and only the two `.scroll` panes move. Every nested grid
track carries `min-height: 0` — drop it and the panes stop scrolling and the page
grows instead. The divider is a real `<button role="separator">` that drags,
takes arrow keys, and remembers its width in `localStorage`.

The ledger renders each version as `KEY → value` with values masked by a
**fixed-width** dot string — the mask must never track the secret's length.
Reveal is per row (or `r` for all) and resets on every version change. `t` shows
the verbatim file, which is the escape hatch for anything the dotenv parser
counts as unreadable rather than silently dropping.

## Web security model

`envault web` serves the contents of every `.env` on the machine, so three things
hold it in: it binds `127.0.0.1`; it rejects any `Host` that is not loopback (DNS
rebinding); and every request carries a random per-session token in
`X-Envault-Token`, which a cross-origin page cannot send without a preflight this
server never approves. The page bootstraps the token from `?token=` and strips it
from the URL, so it is never baked into the served bytes. Responses are
`no-store`. `app.css` and `app.js` are the only unauthenticated routes — they are
identical for every user and hold no vault data.

Handlers delegate to the same `store`/`config`/`scanner`/`daemon`/`transfer`
calls the CLI uses. Add behaviour there, not in the HTTP layer.

## Release

```bash
git tag v<X.Y.Z>
git push origin v<X.Y.Z>
```

`release.yml` cross-compiles four static binaries (`CGO_ENABLED=0`, `-s -w`,
version = the tag), writes `checksums.txt`, and publishes the Release. It does
**not** run tests, so a tag inherits whatever `ci.yml` last verified on the
branch — check CI is green before tagging.
