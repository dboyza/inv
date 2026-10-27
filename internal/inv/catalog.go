package inv

import "strings"

// Section: Category order per tab

// CategoryOrder sorts groups within a tab; unlisted categories sort before "Other".
var CategoryOrder = map[Kind][]string{
	Desktop: {"Development", "Terminals", "AI", "Browsers", "Communication", "Productivity", "Media & creative",
		"Menu bar & utilities", "Networking", "Customization", "Web apps", "Apple", "Other"},
	Terminal: {"AI agents", "Agent tooling", "Cloud & infra", "Git & GitHub", "Shell & terminal", "Editors",
		"Search & text", "Languages & build", "Code quality", "Networking", "Security", "System", "Bundled with apps", "Dotfiles helpers", "Other"},
	Runtime:   {"Python", "Node.js", "Go", "Rust", "Ruby", "Java", "Apple toolchains", "Other"},
	Font:      {"Nerd Fonts", "Fonts"},
	Extension: {"VS Code", "Cursor", "Windsurf", "VSCodium", "Neovim"},
	Service:   {},
}

// CategoryRank returns a sort key for a category within a kind.
func CategoryRank(k Kind, cat string) int {
	order := CategoryOrder[k]
	for i, c := range order {
		if c == cat {
			return i
		}
	}
	if cat == "Other" || cat == "Missing" {
		return 1000
	}
	return len(order)
}

// Section: Curated entries

type entry struct{ cat, desc string }

// terminalCatalog classifies CLIs by purpose; desc fills in where no package metadata exists.
var terminalCatalog = map[string]entry{
	"claude":   {"AI agents", "Anthropic's agentic coding CLI"},
	"codex":    {"AI agents", "OpenAI's coding agent for the terminal"},
	"opencode": {"AI agents", "Open-source terminal coding agent"},
	"pi":       {"AI agents", "Minimal, extensible terminal coding agent"},
	"omp":      {"AI agents", "Oh My Pi coding agent with multi-model routing"},
	"devin":    {"AI agents", "Cognition's Devin agent CLI"},
	"aider":    {"AI agents", ""}, "gemini": {"AI agents", ""}, "amp": {"AI agents", ""}, "goose": {"AI agents", ""},
	"herdr":               {"Agent tooling", "Organizes agent workspaces inside tmux sessions"},
	"herdr-reviewr":       {"Agent tooling", "Herdr plugin for reviewing agent changes"},
	"treehouse":           {"Agent tooling", "Pool of pre-warmed git worktrees for parallel agents"},
	"no-mistakes":         {"Agent tooling", "Background guardrail daemon for coding agents"},
	"gh-axi":              {"Agent tooling", "Agent-ergonomic wrapper around the GitHub CLI"},
	"chrome-devtools-axi": {"Agent tooling", "Agent-ergonomic Chrome DevTools controller"},
	"lavish-axi":          {"Agent tooling", "Turns agent HTML artifacts into review surfaces"},
	"quota-axi":           {"Agent tooling", "Agent-ergonomic usage quota checker"},
	"tasks-axi":           {"Agent tooling", "Agent-ergonomic task tracker"},
	"adrafinil":           {"Agent tooling", "Keeps the Mac awake while agents work"},
	"terraform":           {"Cloud & infra", "Infrastructure as code from HashiCorp"},
	"kubernetes-cli":      {"Cloud & infra", ""}, "kubectl": {"Cloud & infra", "Kubernetes command-line client"},
	"docker": {"Cloud & infra", "Docker container CLI"}, "docker-compose": {"Cloud & infra", "Multi-container Docker orchestration"},
	"docker-credential-desktop": {"Cloud & infra", "Docker credential helper"}, "docker-credential-osxkeychain": {"Cloud & infra", "Docker credential helper for the keychain"},
	"docker-credential-ecr-login": {"Cloud & infra", "Docker credential helper for Amazon ECR"},
	"kubectl.docker":              {"Cloud & infra", "Docker Desktop's bundled kubectl"},
	"tailscale":                   {"Networking", "Tailscale VPN command-line client"},
	"awscli":                      {"Cloud & infra", ""}, "azure-cli": {"Cloud & infra", ""}, "helm": {"Cloud & infra", ""}, "k9s": {"Cloud & infra", ""},
	"git": {"Git & GitHub", ""}, "gh": {"Git & GitHub", ""}, "git-lfs": {"Git & GitHub", ""}, "lazygit": {"Git & GitHub", ""},
	"pre-commit": {"Git & GitHub", ""}, "git-delta": {"Git & GitHub", ""},
	"zsh": {"Shell & terminal", ""}, "zsh-autosuggestions": {"Shell & terminal", ""}, "zsh-syntax-highlighting": {"Shell & terminal", ""},
	"starship": {"Shell & terminal", ""}, "zoxide": {"Shell & terminal", ""}, "direnv": {"Shell & terminal", ""},
	"fzf": {"Shell & terminal", ""}, "tmux": {"Shell & terminal", ""}, "bash": {"Shell & terminal", ""},
	"wezterm": {"Shell & terminal", "WezTerm's command-line interface"}, "wezterm-gui": {"Shell & terminal", "WezTerm GUI launcher"},
	"wezterm-mux-server": {"Shell & terminal", "WezTerm multiplexer server"}, "strip-ansi-escapes": {"Shell & terminal", "Strips ANSI escape codes from text"},
	"neovim": {"Editors", ""}, "vim": {"Editors", ""}, "helix": {"Editors", ""}, "code": {"Editors", "Opens files in VS Code"},
	"ripgrep": {"Search & text", ""}, "bat": {"Search & text", ""}, "jq": {"Search & text", ""}, "tree": {"Search & text", ""},
	"fd": {"Search & text", ""}, "yq": {"Search & text", ""},
	"node": {"Languages & build", ""}, "uv": {"Languages & build", ""}, "make": {"Languages & build", ""},
	"cmake": {"Languages & build", ""}, "pnpm": {"Languages & build", ""},
	"shellcheck": {"Code quality", ""}, "shfmt": {"Code quality", ""}, "ruff": {"Code quality", ""},
	"curl": {"Networking", ""}, "wget": {"Networking", ""}, "bind": {"Networking", ""}, "socat": {"Networking", ""},
	"gnupg": {"Security", ""},
	"btop":  {"System", ""}, "fastfetch": {"System", ""}, "mas": {"System", ""}, "unzip": {"System", ""},
	"dotfiles-clipboard": {"Dotfiles helpers", "Cross-platform clipboard bridge from dotfiles"},
	"dotfiles-tool.mjs":  {"Dotfiles helpers", "Shared installer and launcher for managed agent CLIs"},
	"tbo":                {"Other", "Alias for theboringfloor"},
}

// desktopCatalog classifies apps by bundle ID; desc fills in where casks have none.
var desktopCatalog = map[string]entry{
	"com.microsoft.VSCode":                  {"Development", "Code editor from Microsoft"},
	"com.docker.docker":                     {"Development", "Containers and Kubernetes on your Mac"},
	"sh.brew.app":                           {"Development", "Native Homebrew package manager app"},
	"com.mitchellh.ghostty":                 {"Terminals", "Fast, native, feature-rich terminal"},
	"com.github.wez.wezterm":                {"Terminals", "GPU-accelerated terminal and multiplexer"},
	"dev.warp.Warp-Stable":                  {"Terminals", "Agentic development environment terminal"},
	"com.googlecode.iterm2":                 {"Terminals", "Classic macOS terminal replacement"},
	"com.dboyza.wezterm-floating-tabs":      {"Terminals", "Floating tab strip companion for WezTerm"},
	"com.openai.codex":                      {"AI", "OpenAI's ChatGPT and Codex desktop app"},
	"com.anthropic.claudefordesktop":        {"AI", "Anthropic's Claude desktop app"},
	"com.anthropic.claude-code-url-handler": {"AI", "Opens claude-cli:// links in Claude Code"},
	"ru.starmel.OpenSuperWhisper":           {"AI", "Local speech-to-text dictation with Whisper"},
	"com.google.Chrome":                     {"Browsers", "Google's web browser"},
	"com.hnc.Discord":                       {"Communication", "Voice, video, and text chat"},
	"com.citrix.receiver.nomas":             {"Communication", "Remote desktops and virtual apps"},
	"org.libreoffice.script":                {"Productivity", "Free and open-source office suite"},
	"eu.exelban.Stats":                      {"Menu bar & utilities", "System monitor in the menu bar"},
	"com.if.Amphetamine":                    {"Menu bar & utilities", "Keeps your Mac awake on your terms"},
	"glass.kagerou.adrafinil":               {"Menu bar & utilities", "Keeps your Mac awake while agents work"},
	"theboringteam.boringnotch":             {"Customization", "Turns the notch into a dynamic music and widget area"},
	"sandimax.Wallper":                      {"Customization", "Live video wallpapers"},
	"com.rileycx.strafe":                    {"Menu bar & utilities", "Accessibility-powered utility, built from pinned source"},
	"io.tailscale.ipn.macsys":               {"Networking", "Zero-config WireGuard mesh VPN"},
	"com.microsoft.vdi2.teams.citrix":       {"Communication", "Microsoft Teams optimization for Citrix desktops"},
}

// appleAppDesc covers Apple's App Store apps, which have no cask metadata.
var appleAppDesc = map[string]string{
	"com.apple.dt.Xcode":     "Apple's IDE for every Apple platform",
	"com.apple.garageband10": "Music creation studio",
	"com.apple.iMovieApp":    "Video editing",
	"com.apple.Keynote":      "Presentations",
	"com.apple.Numbers":      "Spreadsheets",
	"com.apple.Pages":        "Word processing and page layout",
}

// lsCategory maps LSApplicationCategoryType to a desktop group.
var lsCategory = map[string]string{
	"developer-tools": "Development", "productivity": "Productivity", "business": "Productivity",
	"social-networking": "Communication", "utilities": "Menu bar & utilities", "music": "Media & creative",
	"video": "Media & creative", "photography": "Media & creative", "graphics-design": "Media & creative",
	"entertainment": "Media & creative", "games": "Media & creative", "education": "Productivity",
	"finance": "Productivity", "news": "Other", "reference": "Productivity", "lifestyle": "Other",
}

// desktopCategory picks a group for an app from the curated map, then its own metadata.
func desktopCategory(b Bundle) string {
	if e, ok := desktopCatalog[b.ID]; ok {
		return e.cat
	}
	if b.WebApp {
		return "Web apps"
	}
	if c, ok := lsCategory[strings.TrimPrefix(b.Category, "public.app-category.")]; ok {
		return c
	}
	if b.MenuBar {
		return "Menu bar & utilities"
	}
	return "Other"
}

// terminalEntry looks a CLI up by any of its names.
func terminalEntry(names ...string) (entry, bool) {
	for _, n := range names {
		if e, ok := terminalCatalog[n]; ok {
			return e, true
		}
	}
	return entry{}, false
}

// runtimeFormula reports whether a Homebrew formula is a language runtime and which group it belongs to.
func runtimeFormula(name string) (string, bool) {
	base, _, _ := strings.Cut(name, "@")
	switch base {
	case "python", "pypy3":
		return "Python", true
	case "node":
		return "Node.js", true
	case "go":
		return "Go", true
	case "rust", "rustup":
		return "Rust", true
	case "ruby":
		return "Ruby", true
	case "openjdk", "temurin":
		return "Java", true
	case "deno", "bun", "lua", "luajit", "php", "erlang", "elixir", "zig":
		return "Other", true
	}
	return "", false
}
