# inv agent guide

This is a living document: keep it accurate, concise, and deduplicated as the project evolves.
Verify guidance against current code and user decisions, correct stale claims, and prune superseded instructions.
Record only durable project knowledge, never session notes.

## Product decisions

- inv is a macOS-only terminal UI listing every custom install, built with Go, Bubble Tea v2, Lip Gloss v2, and Bubbles v2.
- Desktop apps and terminal tools must stay visually separate: distinct tabs, accent colors, and icons per kind.
- Tabs are Desktop, Terminal, Runtimes, Fonts, Extensions, and Services; items group by purpose by default, with the source shown as a badge.
- The theme is Rose Pine Moon, matching the user's dotfiles `theme/palette.json`; do not paint a global background.
- Actions stay non-destructive: open, reveal, homepage, copy path, copy reinstall command; no install, update, or uninstall.
- Homebrew dependencies are hidden until toggled; macOS system apps (non-App Store `com.apple.*`) are excluded, while Apple's App Store apps appear in an Apple group.
- CLIs that ship inside apps appear in Terminal with a `via <App>` badge and link back to their app.
- Dotfiles drift (◆ declared, ◇ untracked, ✗ missing) appears only when a Nix dotfiles repo is found.
- The UI updates live from filesystem events and must keep working at 80 × 24.

## Architecture

- `internal/inv` observes and classifies; it never imports the UI.
  Collectors fill a `Snapshot` per `Group`; `Build` is pure and turns a snapshot into items, so it reruns after every partial rescan.
- Scans can overlap, so results merge with `Snapshot.Merge` for only the groups they collected.
- Network work (`CheckUpdates`) and `--version` probes (`ProbeVersions`) run after collection and are cached with the snapshot.
- `watch.go` maps watched directories to groups, debounces bursts, and re-runs once more after a settle delay for slow copies.
- `internal/ui` owns state, rendering, keys, and actions; `rows.go` turns items into grouped, filtered rows.
- Add purpose groups and descriptions in `catalog.go`; prefer package metadata, and curate only what metadata lacks.
- Never execute dotfiles-managed launchers (for example `~/.local/bin/claude`), since running them can trigger installs; read versions from their metadata instead.

## Conventions

- Start files with a purpose comment, mark responsibilities with `Section:` comments, and keep comments to two lines.
- Keep `View` output within the terminal size at every width; extend `internal/ui/view_test.go` when changing layout.
- Use normal hyphens, never em dashes, in prose and UI copy.

## Verification

- Run `nix develop -c go vet ./...` and `nix develop -c go test ./...`.
- `nix develop -c go test ./internal/inv -bench BuildFromCache` measures `Build` against the local cache; keep it in the low milliseconds.
- For visual checks, run the app in tmux with `env -u TMUX COLORTERM=truecolor ./inv` so Lip Gloss keeps full color, and test 80 × 24 as well as wide terminals.
- Check classification against the real machine with `./inv --json`.
