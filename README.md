<div align="center">
  <img src="docs/assets/logo.svg" width="72" height="72" alt="inv logo: a declared and an untracked item in a list">

  <h1>inv</h1>

  <p><strong>See every app, command-line tool, and runtime you installed on your Mac, in one live terminal view.</strong></p>

  <p>
    inv finds custom installs across Homebrew, the App Store, npm, uv, vendor installers, and hand-copied binaries.
    It separates desktop apps from terminal tools, flags drift from your dotfiles, and updates as you install and remove things.
  </p>

  <p>
    <a href="#quick-start">Quick start</a> ·
    <a href="#preview">Preview</a> ·
    <a href="#how-it-works">How it works</a> ·
    <a href="#development">Development</a>
  </p>
</div>

## Quick start

You need macOS and either Nix with flakes or Go 1.26+.
Homebrew, `mas`, and `npm` are optional; inv uses them when they are installed.

From the repository root, build and run:

```sh
nix develop -c go build -o inv .
./inv
```

Without Nix, run `go build -o inv .` instead.
The first scan takes a few seconds, then six tabs appear with counts for desktop apps, terminal tools, runtimes, fonts, extensions, and services.
Later launches paint instantly from a cache in `~/Library/Caches/inv/` and refresh in the background.

inv only reads your system.
It does check for updates over the network with `brew outdated --greedy`, `mas outdated`, and `npm outdated -g`, and it runs `--version` on standalone binaries that have no other version source.

For scripting, `./inv --json` prints the full inventory and exits.

## Preview

<p align="center">
  <a href="docs/assets/screenshot.png"><img src="docs/assets/screenshot.png" width="860" alt="inv showing the Desktop tab grouped by purpose, with Docker selected: an available update, its Homebrew cask source, and the six command-line tools it bundles"></a>
</p>

## How it works

inv collects raw observations from each install source in parallel, then classifies them into one normalized list.

- **Desktop and terminal stay separate.** Each kind gets its own tab, accent color, and purpose groups such as AI agents, Cloud & infra, or Terminals.
- **Every row shows where it came from.** Source badges distinguish brew, cask, App Store, npm, uv, dotfiles, vendor installers, and manual copies.
- **Apps and their CLIs are linked.** Commands that ship inside an app, like `docker` in Docker.app, show `via Docker`, and Enter jumps between them.
- **Drift is visible.** ◆ marks items your dotfiles declare, ◇ marks hand installs, and ✗ marks declared items that are missing.
- **It stays current.** A file watcher rescans only the affected sources and announces installs, removals, and upgrades as they happen.

The details pane adds versions and available updates, install and last-used dates, zsh history run counts, commands shared between installs, dangling links, and a reinstall command.

Dotfiles drift reads `nix/homebrew.nix`, `nix/macos-apps.json`, and `scripts/managed-tools.json` from a Nix dotfiles repository.
inv finds the repository by following Home Manager links such as `~/.zshrc`, or you can set `INV_DOTFILES` to its path.
Without one, drift markers are hidden.

<details>
<summary>Keyboard and mouse</summary>

| Keys | Action |
| --- | --- |
| `←` `→` `tab` `1`-`6` | Switch tabs (or click a tab) |
| `↑` `↓` `j` `k` `g` `G` | Move through the list (or scroll and click) |
| `/` | Fuzzy search across every tab |
| `enter` | Jump between an app and its bundled CLIs; opens details on narrow screens |
| `b` / `s` | Group by purpose, source, or nothing; sort by name, newest, or last used |
| `t` / `u` / `d` | Show only untracked, only updatable, or include brew dependencies |
| `o` `f` `w` | Open the app, reveal in Finder, open the homepage |
| `c` / `i` | Copy the path or the reinstall command |
| `r` / `U` | Rescan now, recheck updates |
| `?` | Full help |

</details>

## Development

```text
package-tui/
├── main.go             Flags, --json output, program start
├── internal/inv/       Discovery and classification, no UI
│   ├── snapshot.go     Collector groups, parallel scans, cache
│   ├── build.go        Snapshot to display items: drift, links, updates
│   ├── catalog.go      Curated purpose groups and descriptions
│   └── watch.go        Debounced filesystem watching
├── internal/ui/        Bubble Tea model, views, keys, Rose Pine theme
├── docs/assets/        Logo and screenshot
├── flake.nix           Dev shell that pins Go
└── AGENTS.md           Conventions for contributors and coding agents
```

Run the checks from the repository root:

```sh
nix develop -c go vet ./...
nix develop -c go test ./...
```

## Next steps

- **Use it:** press `?` inside inv for every shortcut, or pipe `./inv --json` into `jq` for scripts.
- **Change it:** read [AGENTS.md](AGENTS.md) for the architecture boundary, catalog conventions, and verification steps.
- **Report a problem:** include your macOS version, the install source involved, and the matching entry from `./inv --json`.
