# Code Journal

## Knowledge

- `internal/store/archive.go` — archiving moves a file's history JSON from
  `index/` to `archives/`; blobs never move. That is why `GC()` must count
  references from both listings — pinned by `TestArchivedContentSurvivesGC`.
  Unarchive merges with any fresh live history (dedupe by blob ID) because a
  rescan of an archived path starts a new index file.
- `internal/web` — reads against the shelf carry `archived=1`
  (`resolveEntry`), and `/api/state` returns `archives` beside `files`, both
  rendered by the same `fileEntry` shape.

## Log

- 2026-08-23 — archive feature end to end: store (`Archive`/`Unarchive`/
  `ListArchived` + GC respecting archives), CLI (`envault archive`,
  `unarchive`, `list --archived`), web API (`/api/archive`, `/api/unarchive`,
  archived reads/restores) and browser UI (shelf toggle, archive/unarchive
  tools), TUI shelf (`A` toggle, `a` archive, `u` unarchive). TDD throughout;
  tests written red first per layer. #feature
