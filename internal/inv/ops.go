package inv

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Section: Operation plans

// OpKind is a change inv can make to an installed item.
type OpKind int

const (
	OpUpgrade OpKind = iota
	OpUninstall
)

func (k OpKind) String() string { return [...]string{"Upgrade", "Uninstall"}[k] }

// Plan is the exact command an operation would run, decided without side effects.
type Plan struct {
	Kind     OpKind
	Item     Item
	Argv     []string // command to run; empty when Blocked
	Label    string   // button text, e.g. "Upgrade to 1.17.0"
	Warnings []string // shown in the confirmation dialog
	Blocked  string   // why the operation is unavailable
	Groups   []Group  // collectors to rescan afterwards
}

// Allowed reports whether the plan can run.
func (p Plan) Allowed() bool { return p.Blocked == "" && len(p.Argv) > 0 }

// Command renders the argv as a copyable shell line.
func (p Plan) Command() string {
	parts := make([]string, len(p.Argv))
	for i, a := range p.Argv {
		if strings.ContainsAny(a, " '\"$`\\") {
			a = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
		parts[i] = a
	}
	return strings.Join(parts, " ")
}

// PlanUpgrade decides how to upgrade it, given every item for cross-references.
func PlanUpgrade(env *Env, it Item, all []Item) Plan {
	p := Plan{Kind: OpUpgrade, Item: it, Label: "Upgrade"}
	if it.Latest != "" {
		p.Label = "Upgrade to " + it.Latest
	}
	switch it.Source {
	case SrcBrew:
		p.Argv, p.Groups = []string{"brew", "upgrade", it.SourceRef}, []Group{GBrew, GBins}
	case SrcCask:
		p.Argv, p.Groups = []string{"brew", "upgrade", "--cask", it.SourceRef}, []Group{GBrew, GApps, GFonts, GBins}
	case SrcAppStore:
		if it.SourceRef == "" {
			p.Blocked = "App Store ID unknown; update it from the App Store"
		}
		p.Argv, p.Groups = []string{"mas", "upgrade", it.SourceRef}, []Group{GApps}
	case SrcNpm:
		p.Argv, p.Groups = []string{"npm", "install", "-g", "--prefix", npmPrefix(it), it.Name + "@latest"}, []Group{GNpm}
	case SrcUv:
		p.Argv, p.Groups = []string{"uv", "python", "upgrade", minorVersion(it.Version)}, []Group{GRuntimes}
	case SrcVSCode:
		p.Label = "Update"
		p.Argv, p.Groups = []string{"code", "--install-extension", it.SourceRef, "--force"}, []Group{GExts}
	case SrcBundled:
		p.Blocked = "Ships inside " + it.SourceRef + "; upgrade the app instead"
	case SrcDotfiles:
		p.Blocked = "Managed by dotfiles; it updates itself or through bootstrap"
	case SrcLazy:
		p.Blocked = "Pinned in lazy-lock.json; update it with :Lazy update"
	default:
		p.Blocked = "No package manager owns this; update it from the vendor"
	}
	if p.Blocked == "" && !it.Updatable() && it.Source != SrcVSCode {
		p.Blocked = "Already up to date"
		if it.Latest == "" && it.Source != SrcBrew && it.Source != SrcCask {
			p.Blocked = "No newer version known"
		}
	}
	if it.Declared == DeclMissing {
		p.Blocked = "Not installed"
	}
	return p
}

// PlanUninstall decides how to remove it; package managers first, Finder's Trash otherwise.
func PlanUninstall(env *Env, it Item, all []Item) Plan {
	p := Plan{Kind: OpUninstall, Item: it, Label: "Uninstall"}
	switch it.Source {
	case SrcBrew:
		p.Argv, p.Groups = []string{"brew", "uninstall", it.SourceRef}, []Group{GBrew, GBins}
		if it.Dependency {
			p.Warnings = append(p.Warnings, "Installed as a dependency; brew refuses if something still needs it")
		}
	case SrcCask:
		p.Argv, p.Groups = []string{"brew", "uninstall", "--cask", it.SourceRef}, []Group{GBrew, GApps, GFonts, GBins}
	case SrcAppStore:
		if it.SourceRef == "" {
			p.Blocked = "App Store ID unknown; drag it to the Trash in Finder"
		}
		// mas needs root to remove App Store apps; Touch ID or a password prompt appears.
		p.Argv, p.Groups = []string{"sudo", "mas", "uninstall", it.SourceRef}, []Group{GApps}
	case SrcNpm:
		p.Argv, p.Groups = []string{"npm", "uninstall", "-g", "--prefix", npmPrefix(it), it.Name}, []Group{GNpm}
	case SrcUv:
		p.Argv, p.Groups = []string{"uv", "python", "uninstall", it.Version}, []Group{GRuntimes, GBins}
	case SrcVSCode:
		p.Argv, p.Groups = []string{"code", "--uninstall-extension", it.SourceRef}, []Group{GExts}
	case SrcBundled:
		p.Blocked = "Ships inside " + it.SourceRef + "; uninstall the app to remove it"
	case SrcDotfiles, SrcNix:
		p.Blocked = "Managed by dotfiles; remove it from " + firstNonEmpty(it.DeclaredIn, "your dotfiles") + " instead"
	case SrcLazy:
		p.Blocked = "Remove the plugin from your Neovim config instead"
	case SrcSystem, SrcPythonOrg:
		p.Blocked = "Use the vendor's uninstaller for system-level installs"
	case SrcPlugin:
		p.Blocked = "Managed by its host's plugin system; remove it there"
	case SrcDirect, SrcManual, SrcInstaller:
		paths := []string{it.Path}
		if it.Kind == Font {
			paths = it.Files // a font family's Path is the shared Fonts folder
		}
		if it.Source == SrcInstaller && strings.HasPrefix(it.SourceRef, "~/") {
			root := filepath.Join(env.Home, strings.TrimPrefix(it.SourceRef, "~/"))
			paths = append(paths, root)
			p.Warnings = append(p.Warnings, "Also trashes its install folder "+it.SourceRef)
		}
		for _, path := range paths {
			if !safeToTrash(env, path) {
				p.Blocked = "Refusing to trash " + env.Tilde(path) + "; remove it by hand"
			}
		}
		if len(paths) == 0 {
			p.Blocked = "No files on disk"
		}
		p.Argv, p.Groups = trashArgv(paths), []Group{GApps, GBins, GFonts}
		p.Label = "Move to Trash"
		p.Warnings = append(p.Warnings, "No package manager owns this; it moves to the Trash, where Put Back restores it")
	default:
		p.Blocked = "inv cannot remove this kind of install"
	}
	if it.Kind == Service {
		p.Blocked = "Background services belong to their app; remove the app instead"
	}
	if it.Declared == DeclMissing {
		p.Blocked = "Not installed"
	}
	if p.Blocked != "" {
		return p
	}
	if it.Declared == DeclTracked {
		p.Warnings = append([]string{"Declared in " + it.DeclaredIn + "; the next bootstrap reinstalls it unless you remove it there"}, p.Warnings...)
	}
	var cmds []string
	for _, other := range all {
		if other.ProvidedBy == it.ID && other.Kind == Terminal {
			cmds = append(cmds, other.Name)
		}
	}
	if len(cmds) > 0 {
		p.Warnings = append(p.Warnings, fmt.Sprintf("Also removes its commands: %s", listPreview(cmds, 4)))
	}
	return p
}

// Section: Helpers

// trashArgv moves paths to the Trash through Finder so Put Back works and admin prompts appear.
func trashArgv(paths []string) []string {
	files := make([]string, len(paths))
	for i, p := range paths {
		files[i] = fmt.Sprintf("POSIX file %q", p)
	}
	return []string{"osascript", "-e", fmt.Sprintf(`tell application "Finder" to delete {%s}`, strings.Join(files, ", "))}
}

// safeToTrash refuses shared folders, so a bad plan can never trash a whole directory of installs.
func safeToTrash(env *Env, path string) bool {
	clean := filepath.Clean(path)
	if path == "" || !filepath.IsAbs(clean) {
		return false
	}
	protected := []string{"/", "/Applications", "/Library", "/usr", "/usr/local", "/usr/local/bin", "/opt", env.Home}
	for _, rel := range []string{"Applications", "Library", "Library/Fonts", ".local", ".local/bin", ".local/share", ".local/lib",
		".config", "bin", "go", "go/bin", ".cargo", ".cargo/bin", "code"} {
		protected = append(protected, filepath.Join(env.Home, rel))
	}
	if env.BrewPrefix != "" {
		protected = append(protected, env.BrewPrefix, filepath.Join(env.BrewPrefix, "bin"))
	}
	if env.Dotfiles != "" && (clean == env.Dotfiles || strings.HasPrefix(clean, env.Dotfiles+"/")) {
		return false
	}
	for _, p := range protected {
		if clean == filepath.Clean(p) {
			return false
		}
	}
	return true
}

// npmPrefix recovers the install prefix from a package path like <prefix>/lib/node_modules/name.
func npmPrefix(it Item) string {
	if i := strings.Index(it.Path, "/lib/node_modules/"); i > 0 {
		return it.Path[:i]
	}
	return strings.TrimPrefix(it.SourceRef, "prefix ")
}

func minorVersion(v string) string {
	parts := strings.Split(v, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return v
}
