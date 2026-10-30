package inv

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// builder accumulates items while Build applies each source's rules.
type builder struct {
	env      *Env
	snap     *Snapshot
	items    map[string]*Item
	order    []string
	dangling map[*Item][]string // python.org links whose framework is gone
	words    map[string]string  // distinctive dotfiles words -> first file mentioning them
	appReal  map[*Item]string   // resolved bundle paths for desktop items
}

// Build turns raw observations into display items: pure, so it is cheap to rerun after any rescan.
func Build(env *Env, s Snapshot) []Item {
	b := &builder{env: env, snap: &s, items: map[string]*Item{}, dangling: map[*Item][]string{}, appReal: map[*Item]string{}}
	b.apps()
	b.formulae()
	b.runtimes()
	b.bins()
	b.npm()
	b.fonts()
	b.extensions()
	b.services()
	b.missing()
	b.finish()
	out := make([]Item, 0, len(b.order))
	for _, id := range b.order {
		out = append(out, *b.items[id])
	}
	SortItems(out)
	return out
}

// add registers an item; the first registration of an ID wins.
func (b *builder) add(it Item) *Item {
	if existing, ok := b.items[it.ID]; ok {
		return existing
	}
	b.items[it.ID] = &it
	b.order = append(b.order, it.ID)
	return &it
}

func (b *builder) hasDotfiles() bool { return b.snap.Dotfiles.Repo != "" }

// declare marks an item tracked when where is non-empty, otherwise untracked.
func (b *builder) declare(it *Item, where string) {
	switch {
	case !b.hasDotfiles():
		it.Declared = DeclUnknown
	case where != "":
		it.Declared, it.DeclaredIn = DeclTracked, where
	default:
		it.Declared = DeclUntracked
	}
}

// Section: Desktop apps

func (b *builder) caskForApp(bundlePath string) *BrewCask {
	base := filepath.Base(bundlePath)
	for i, c := range b.snap.Brew.Casks {
		for _, a := range c.Apps {
			if filepath.Base(a) == base {
				return &b.snap.Brew.Casks[i]
			}
		}
	}
	return nil
}

func (b *builder) declaredApp(bd Bundle) *DeclaredApp {
	for i, a := range b.snap.Dotfiles.Apps {
		if a.ID == bd.ID || a.Bundle == filepath.Base(bd.Path) {
			return &b.snap.Dotfiles.Apps[i]
		}
	}
	return nil
}

func (b *builder) apps() {
	for _, bd := range b.snap.Apps {
		apple := strings.HasPrefix(bd.ID, "com.apple.")
		if apple && !bd.AppStore {
			continue // Safari and other OS-managed apps are not custom installs.
		}
		it := Item{ID: "app:" + bd.Path, Name: bd.Name, Kind: Desktop, Version: bd.Version, Path: bd.Path,
			BundleID: bd.ID, InstalledAt: bd.InstalledAt, LastUsed: bd.LastUsed, Category: desktopCategory(bd)}
		if e, ok := desktopCatalog[bd.ID]; ok {
			it.Desc = e.desc
		}
		decl := b.declaredApp(bd)
		switch cask := b.caskForApp(bd.Path); {
		case cask != nil:
			it.Source, it.SourceRef = SrcCask, cask.Token
			it.Desc = firstNonEmpty(cask.Desc, it.Desc)
			it.Homepage = cask.Homepage
			if cask.Time > 0 {
				it.InstalledAt = time.Unix(cask.Time, 0)
			}
			if v := caskVersion(cask.Installed); v != "" && v != "latest" && bd.Version != "" && !SameVersion(v, bd.Version) {
				it.Notes = append(it.Notes, fmt.Sprintf("Self-updated since brew installed %s", v))
			}
		case bd.AppStore:
			it.Source, it.SourceRef = SrcAppStore, bd.AppStoreID
			if bd.AppStoreID != "" {
				it.Homepage = "https://apps.apple.com/app/id" + bd.AppStoreID
			}
		case decl != nil && (decl.URL != "" || decl.Build != ""):
			it.Source = SrcDotfiles
			if decl.Build != "" {
				it.SourceRef = "built from source"
			} else {
				it.SourceRef = "direct download"
			}
		default:
			it.Source = SrcDirect
		}
		if decl != nil && decl.Cask != "" && it.Source != SrcCask {
			it.Notes = append(it.Notes, "Installed outside Homebrew; dotfiles would install cask "+decl.Cask+" if it went missing")
		}
		if apple {
			it.Apple, it.Category = true, "Apple"
			it.Desc = firstNonEmpty(appleAppDesc[bd.ID], it.Desc)
		}
		if strings.Contains(filepath.Base(bd.Path), ".previous") {
			it.Notes = append(it.Notes, "Previous build kept by an installer for rollback")
		}
		where := ""
		switch {
		case decl != nil:
			where = "nix/macos-apps.json"
		case it.Source == SrcCask && contains(b.snap.Dotfiles.Casks, it.SourceRef):
			where = "nix/homebrew.nix"
		default:
			where = b.snap.Dotfiles.Mentions(strings.TrimSuffix(filepath.Base(bd.Path), ".previous.app"))
			if where == "" {
				where = b.snap.Dotfiles.Mentions(bd.ID)
			}
		}
		b.declare(&it, where)
		b.add(it)
	}
}

// caskVersion drops the build suffix brew appends after a comma.
func caskVersion(v string) string { s, _, _ := strings.Cut(v, ","); return s }

// appForPath finds the desktop item whose bundle contains p.
func (b *builder) appForPath(p string) *Item {
	for _, id := range b.order {
		it := b.items[id]
		if it.Kind != Desktop || it.Declared == DeclMissing {
			continue
		}
		if p == it.Path || strings.HasPrefix(p, it.Path+"/") {
			return it
		}
		real, ok := b.appReal[it]
		if !ok {
			real, _ = filepath.EvalSymlinks(it.Path)
			b.appReal[it] = real
		}
		if real != "" && (p == real || strings.HasPrefix(p, real+"/")) {
			return it
		}
	}
	return nil
}

// Section: Homebrew formulae

func (b *builder) formulae() {
	for _, f := range b.snap.Brew.Formulae {
		it := Item{ID: "brew:" + f.Name, Name: f.Name, Kind: Terminal, Source: SrcBrew, SourceRef: f.Name,
			Version: f.Version, Desc: f.Desc, Homepage: f.Homepage, Dependency: !f.OnRequest,
			Commands: f.Bins, Deps: f.Deps, Path: filepath.Join(b.env.BrewPrefix, "Cellar", f.Name)}
		if f.Time > 0 {
			it.InstalledAt = time.Unix(f.Time, 0)
		}
		if f.Tap != "" && f.Tap != "homebrew/core" {
			it.SourceRef = f.Tap + "/" + f.Name
		}
		if lang, ok := runtimeFormula(f.Name); ok {
			it.Kind, it.Category = Runtime, lang
			if lang != "Other" {
				it.Name = runtimeName(lang, f.Name, f.Version)
			}
		} else if e, ok := terminalEntry(append([]string{f.Name}, f.Bins...)...); ok {
			it.Category = e.cat
			it.Desc = firstNonEmpty(it.Desc, e.desc)
		} else {
			it.Category = "Other"
		}
		if it.Dependency {
			it.Category = "Dependencies"
		}
		where := ""
		if contains(b.snap.Dotfiles.Brews, f.Name) {
			where = "nix/homebrew.nix"
		}
		b.declare(&it, where)
		if it.Dependency && it.Declared == DeclUntracked {
			it.Declared = DeclUnknown // dependencies are pulled in, never declared
		}
		b.add(it)
	}
}

// runtimeName formats a runtime as "Python 3.14" from either a formula name or a full version.
func runtimeName(lang, formula, version string) string {
	if _, v, ok := strings.Cut(formula, "@"); ok {
		return lang + " " + v
	}
	parts := strings.Split(version, ".")
	if len(parts) >= 2 {
		return lang + " " + parts[0] + "." + parts[1]
	}
	return lang
}

// Section: Standalone binaries

var pathVersionRe = regexp.MustCompile(`/v?(\d+\.\d+(?:\.\d+)*)(?:[-/]|$)`)

func (b *builder) bins() {
	home := b.env.Home
	brewBin := filepath.Join(b.env.BrewPrefix, "bin")
	seenPath := map[string]bool{}
	for _, e := range b.snap.Bins {
		if seenPath[e.Path] || strings.Contains(e.Name, ".backup.") {
			continue
		}
		seenPath[e.Path] = true
		target := e.Target
		appRef := e.ShimApp
		if appRef == "" && strings.HasPrefix(target, "/Applications/") {
			appRef = target
		}
		inBrewBin := b.env.BrewPrefix != "" && filepath.Dir(e.Path) == brewBin
		caskroom := filepath.Join(b.env.BrewPrefix, "Caskroom")
		switch {
		case appRef != "" || (inBrewBin && strings.HasPrefix(target, caskroom)):
			b.bundledCLI(e, appRef)
		case inBrewBin:
			continue // formula links are covered by formulae()
		case strings.Contains(target+e.Link, "Python.framework") || strings.Contains(target+e.Link, "PythonT.framework"):
			b.attachToRuntime(e)
		case strings.HasPrefix(target, filepath.Join(home, ".local/share/uv/python")):
			continue // uv-managed interpreter shims; listed under Runtimes
		case strings.Contains(target, "/lib/node_modules/"):
			continue // npm global; listed by npm()
		case strings.Contains(e.Link, "-home-manager-files/") || (b.env.Dotfiles != "" && strings.HasPrefix(target, b.env.Dotfiles+"/")):
			b.dotfilesBin(e)
		default:
			b.looseBin(e)
		}
	}
}

func (b *builder) bundledCLI(e BinEntry, appRef string) {
	app := b.appForPath(firstNonEmpty(appRef, e.Target))
	if app == nil && strings.Contains(e.Target, "/Caskroom/") {
		// Caskroom wrappers belong to the cask whose token is in the path.
		token := strings.Split(strings.SplitN(e.Target, "/Caskroom/", 2)[1], "/")[0]
		for _, id := range b.order {
			if it := b.items[id]; it.Kind == Desktop && it.SourceRef == token {
				app = it
				break
			}
		}
	}
	it := Item{ID: "bin:" + e.Name, Name: e.Name, Kind: Terminal, Source: SrcBundled, Path: e.Path, InstalledAt: e.InstalledAt}
	if app != nil {
		it.ID = "bundled:" + app.BundleID + ":" + e.Name
		it.SourceRef, it.ProvidedBy, it.Version = app.Name, app.ID, app.Version
		it.Declared, it.DeclaredIn = app.Declared, app.DeclaredIn
		it.Homepage = app.Homepage
	}
	if en, ok := terminalEntry(e.Name); ok {
		it.Category, it.Desc = en.cat, en.desc
	} else if app != nil {
		it.Category = "Bundled with apps"
		it.Desc = "Command-line tool shipped inside " + app.Name
	}
	if it.Category == "" {
		it.Category = "Other"
	}
	if e.Broken {
		it.Notes = append(it.Notes, "Broken symlink → "+e.Link)
	}
	added := b.add(it)
	if app != nil && !contains(app.Provides, added.ID) {
		app.Provides = append(app.Provides, added.ID)
	}
}

// attachToRuntime folds python.org's /usr/local/bin links into its runtime item.
func (b *builder) attachToRuntime(e BinEntry) {
	var first *Item
	for _, id := range b.order {
		it := b.items[id]
		if it.Kind != Runtime || it.Source != SrcPythonOrg {
			continue
		}
		if first == nil {
			first = it
		}
		if !e.Broken && strings.HasPrefix(e.Target, it.Path+"/") {
			it.Commands = appendOnce(it.Commands, e.Name)
			return
		}
	}
	if !e.Broken {
		return
	}
	if first != nil {
		b.dangling[first] = append(b.dangling[first], e.Name)
		return
	}
	b.looseBin(e)
}

func (b *builder) dotfilesBin(e BinEntry) {
	d := b.snap.Dotfiles
	it := Item{ID: "bin:" + e.Name, Name: e.Name, Kind: Terminal, Source: SrcDotfiles, Path: e.Path, InstalledAt: e.InstalledAt}
	if contains(d.Tools, e.Name) {
		it.SourceRef = "managed tool"
		it.Version = d.ToolVersions[e.Name]
		it.Path = filepath.Join(b.env.Home, ".local/share/dotfiles/tools", e.Name)
		b.declare(&it, "scripts/managed-tools.json")
	} else {
		it.SourceRef = "home-manager link"
		b.declare(&it, "nix/home.nix")
	}
	if en, ok := terminalEntry(e.Name); ok {
		it.Category, it.Desc = en.cat, en.desc
	} else {
		it.Category = "Dotfiles helpers"
	}
	b.add(it)
}

func (b *builder) looseBin(e BinEntry) {
	home := b.env.Home
	it := Item{ID: "bin:" + e.Name, Name: e.Name, Kind: Terminal, Path: e.Path, InstalledAt: e.InstalledAt}
	switch dir := filepath.Dir(e.Path); {
	case e.Broken:
		it.Source, it.SourceRef = SrcManual, "dangling link"
		it.Notes = append(it.Notes, "Broken symlink → "+e.Link)
	case strings.HasPrefix(e.Target, filepath.Join(home, ".config/herdr/plugins")):
		it.Source, it.SourceRef = SrcPlugin, "herdr plugin"
	case dir == filepath.Join(home, ".cargo/bin"):
		it.Source, it.SourceRef = SrcInstaller, "cargo install"
	case dir == filepath.Join(home, "go/bin"):
		it.Source, it.SourceRef = SrcInstaller, "go install"
	case dir == filepath.Join(home, ".bun/bin"):
		it.Source, it.SourceRef = SrcInstaller, "bun"
	case e.Target != e.Path && strings.HasPrefix(e.Target, home+"/"):
		// A link into a tool's own home directory means a vendor installer placed it.
		it.Source, it.SourceRef = SrcInstaller, b.env.Tilde(installRoot(home, e.Target))
		if m := pathVersionRe.FindStringSubmatch(strings.TrimPrefix(e.Target, home)); m != nil {
			it.Version = m[1]
		}
	default:
		it.Source, it.SourceRef = SrcManual, "standalone binary"
	}
	if v := b.snap.BinVersions[probeKey(e)]; v != "" && it.Version == "" {
		it.Version = v
	}
	if en, ok := terminalEntry(e.Name); ok {
		it.Category, it.Desc = en.cat, en.desc
	} else {
		it.Category = "Other"
	}
	b.declare(&it, b.mentionWord(e.Name))
	b.add(it)
}

// installRoot returns the top tool directory under home, e.g. ~/.no-mistakes or ~/.local/share/devin.
func installRoot(home, target string) string {
	rel := strings.Split(strings.TrimPrefix(target, home+"/"), "/")
	n := 1
	if len(rel) > 2 && rel[0] == ".local" && rel[1] == "share" {
		n = 3
	}
	return filepath.Join(append([]string{home}, rel[:min(n, len(rel))]...)...)
}

var wordRe = regexp.MustCompile(`[\w.-]+`)

// mentionWord finds a distinctive name in dotfiles; short names are skipped to avoid false matches.
func (b *builder) mentionWord(name string) string {
	if len(name) < 5 {
		return ""
	}
	if b.words == nil {
		b.words = map[string]string{}
		for file, text := range b.snap.Dotfiles.Corpus {
			for _, w := range wordRe.FindAllString(text, -1) {
				w = strings.Trim(w, ".")
				if prev, ok := b.words[w]; !ok || file < prev {
					b.words[w] = file
				}
			}
		}
	}
	return b.words[name]
}

// Section: npm globals

func (b *builder) npm() {
	for _, p := range b.snap.Npm {
		it := Item{ID: "npm:" + p.Name, Name: p.Name, Kind: Terminal, Source: SrcNpm, Version: p.Version,
			Desc: p.Desc, Homepage: p.Homepage, Path: p.Path, Commands: p.Bins, InstalledAt: p.InstalledAt,
			SourceRef: "prefix " + b.env.Tilde(p.Prefix)}
		if it.Homepage == "" {
			it.Homepage = "https://www.npmjs.com/package/" + p.Name
		}
		if e, ok := terminalEntry(append([]string{p.Name}, p.Bins...)...); ok {
			it.Category = e.cat
			it.Desc = firstNonEmpty(it.Desc, e.desc)
		} else {
			it.Category = "Other"
		}
		b.declare(&it, b.mentionWord(p.Name))
		b.add(it)
	}
}

// Section: Runtimes

func (b *builder) runtimes() {
	for _, r := range b.snap.Runtimes {
		it := Item{ID: "rt:" + r.Path, Kind: Runtime, Version: r.Version, Source: r.Source, SourceRef: r.Manager,
			Path: r.Path, InstalledAt: r.InstalledAt, Category: r.Name}
		it.Name = runtimeName(r.Name, "", r.Version)
		switch r.Name {
		case "Command Line Tools":
			it.Name, it.Category, it.Desc = "Xcode Command Line Tools", "Apple toolchains", "clang, git, make, and SDK headers from Apple"
		case "PyPy":
			it.Category = "Python"
		case "Python (free-threaded)":
			it.Category, it.Name = "Python", "Python "+r.Version+" (free-threaded)"
		}
		if it.Desc == "" {
			it.Desc = fmt.Sprintf("%s interpreter managed by %s", r.Name, r.Manager)
		}
		where := ""
		if r.Source == SrcUv {
			where = b.uvDeclared(minorVersion(r.Version))
		}
		b.declare(&it, where)
		if r.Source == SrcSystem {
			it.Declared = DeclUnknown
		}
		b.add(it)
	}
}

var uvInstallRe = regexp.MustCompile(`uv python install[^\n]*`)

// uvDeclared finds a dotfiles line like "uv python install 3.12 3.13" naming this minor version.
func (b *builder) uvDeclared(minor string) string {
	versionRe := regexp.MustCompile(`(^|[\s"'])` + regexp.QuoteMeta(minor) + `([\s"'.]|$)`)
	best := ""
	for file, text := range b.snap.Dotfiles.Corpus {
		for _, line := range uvInstallRe.FindAllString(text, -1) {
			if versionRe.MatchString(line) && (best == "" || file < best) {
				best = file
			}
		}
	}
	return best
}

// Section: Fonts

var fontStyleRe = regexp.MustCompile(`(?i)[-_ ]?(Thin|ExtraLight|Light|Regular|Medium|SemiBold|Bold|ExtraBold|Black|Heavy|Italic|Oblique)+$`)

func (b *builder) fonts() {
	byFile := map[string]*BrewCask{}
	for i, c := range b.snap.Brew.Casks {
		for _, f := range c.Fonts {
			byFile[f] = &b.snap.Brew.Casks[i]
		}
	}
	type group struct {
		item   Item
		styles map[string]bool
		files  int
	}
	groups := map[string]*group{}
	var keys []string
	for _, f := range b.snap.Fonts {
		base := strings.TrimSuffix(f.File, filepath.Ext(f.File))
		family, style, _ := strings.Cut(base, "-")
		if style == "" {
			if loc := fontStyleRe.FindStringIndex(base); loc != nil && loc[0] > 0 {
				family, style = base[:loc[0]], strings.TrimLeft(base[loc[0]:], "-_ ")
			} else {
				style = "Regular"
			}
		}
		key, it := "font:"+family, Item{Name: family, Kind: Font, Source: SrcManual, SourceRef: "copied into Fonts"}
		if c := byFile[f.File]; c != nil {
			key = "font:cask:" + c.Token
			it = Item{Name: fontCaskName(firstNonEmpty(c.Name, c.Token)), Kind: Font, Source: SrcCask, SourceRef: c.Token,
				Version: caskVersion(c.Installed), Homepage: c.Homepage, Desc: c.Desc}
			if c.Time > 0 {
				it.InstalledAt = time.Unix(c.Time, 0)
			}
		} else if strings.Contains(f.Path, "/HomeManager/") {
			it.Source, it.SourceRef = SrcNix, "home-manager"
		}
		g := groups[key]
		if g == nil {
			it.ID, it.Path = key, filepath.Dir(f.Path)
			if it.InstalledAt.IsZero() {
				it.InstalledAt = f.InstalledAt
			}
			g = &group{item: it, styles: map[string]bool{}}
			groups[key] = g
			keys = append(keys, key)
		}
		g.files++
		g.item.Files = append(g.item.Files, f.Path)
		g.styles[style] = true
		g.item.Commands = appendOnce(g.item.Commands, family)
	}
	for _, k := range keys {
		g := groups[k]
		it := g.item
		it.Category = "Fonts"
		if strings.Contains(strings.ReplaceAll(it.Name, " ", ""), "NerdFont") {
			it.Category = "Nerd Fonts"
		}
		sort.Strings(it.Commands)
		if len(it.Commands) < 2 {
			it.Commands = nil // a single family adds nothing beyond the name
		}
		for s := range g.styles {
			it.Members = append(it.Members, s)
		}
		sort.Slice(it.Members, func(i, j int) bool { return styleRank(it.Members[i]) < styleRank(it.Members[j]) })
		if it.Desc == "" {
			it.Desc = fmt.Sprintf("%d font files", g.files)
		} else {
			it.Desc += fmt.Sprintf(" · %d files", g.files)
		}
		where := ""
		if it.Source == SrcCask && contains(b.snap.Dotfiles.Casks, it.SourceRef) {
			where = "nix/homebrew.nix"
		} else if it.Source == SrcNix {
			where = "nix/home.nix"
		}
		b.declare(&it, where)
		b.add(it)
	}
}

var fontNameNoiseRe = regexp.MustCompile(`\s*\(.*\)$|\s+families\b`)

// fontCaskName trims cask display noise: "JetBrainsMono Nerd Font families (JetBrains Mono)".
func fontCaskName(s string) string { return strings.TrimSpace(fontNameNoiseRe.ReplaceAllString(s, "")) }

var styleWeights = []string{"Thin", "ExtraLight", "Light", "Regular", "Medium", "SemiBold", "Bold", "ExtraBold", "Black"}

func styleRank(s string) int {
	weight := strings.TrimSuffix(s, "Italic")
	if weight == "" {
		weight = "Regular"
	}
	for i, w := range styleWeights {
		if w == weight {
			r := i * 2
			if strings.HasSuffix(s, "Italic") {
				r++
			}
			return r
		}
	}
	return 100
}

// Section: Editor extensions

func (b *builder) extensions() {
	nvimTracked := ""
	if real, err := filepath.EvalSymlinks(filepath.Join(b.env.Home, ".config", "nvim")); err == nil && b.env.Dotfiles != "" && strings.HasPrefix(real, b.env.Dotfiles) {
		nvimTracked = "nvim/lazy-lock.json"
	}
	for _, x := range b.snap.Exts {
		it := Item{ID: "ext:" + x.Editor + ":" + x.ID, Name: x.Name, Kind: Extension, Category: x.Editor, Version: x.Version,
			Desc: x.Desc, Homepage: x.Homepage, Path: x.Path, InstalledAt: x.InstalledAt, SourceRef: x.ID}
		switch x.Editor {
		case "Neovim":
			it.Source = SrcLazy
			it.Version = "@" + x.Version
			if x.Publisher != "" {
				it.Desc = firstNonEmpty(it.Desc, "Neovim plugin by "+x.Publisher)
			}
			b.declare(&it, nvimTracked)
		default:
			it.Source = SrcVSCode
			if x.Publisher != "" && it.Desc == "" {
				it.Desc = "By " + x.Publisher
			}
			b.declare(&it, "")
		}
		b.add(it)
	}
}

// Section: Background services

var serviceVendors = map[string]string{"citrix": "Citrix", "google": "Google", "nixos": "Nix", "docker": "Docker",
	"microsoft": "Microsoft", "adobe": "Adobe", "zoom": "Zoom", "tailscale": "Tailscale", "openai": "OpenAI", "anthropic": "Anthropic"}

func (b *builder) services() {
	for _, s := range b.snap.Services {
		it := Item{ID: "svc:" + s.Label, Name: s.Label, Kind: Service, Source: SrcSystem, SourceRef: s.Scope,
			Path: s.Path, InstalledAt: s.InstalledAt, Running: s.Running, Commands: []string{b.env.Tilde(s.Program)}}
		if s.Program == "" {
			it.Commands = nil
		}
		when := []string{}
		if s.RunAtLoad {
			when = append(when, "starts at login")
		}
		if s.KeepAlive {
			when = append(when, "kept alive")
		}
		if s.Disabled {
			when = append(when, "disabled")
		}
		it.Desc = strings.Join(append([]string{strings.ToUpper(s.Scope[:1]) + s.Scope[1:]}, when...), " · ")
		parts := strings.Split(s.Label, ".")
		if len(parts) > 1 {
			it.Category = serviceVendors[strings.ToLower(parts[1])]
		}
		if app := b.appForPath(s.Program); app != nil {
			it.Category = firstNonEmpty(it.Category, app.Name)
			it.ProvidedBy = app.ID
		}
		where := ""
		switch {
		case strings.HasPrefix(s.Label, "org.nixos."):
			where = "nix-darwin"
		default:
			where = b.snap.Dotfiles.Mentions(s.Label)
			if where == "" && len(parts) > 2 {
				where = b.mentionWord(parts[len(parts)-1])
			}
		}
		if it.Category == "" {
			switch {
			case where != "" && where != "nix-darwin":
				it.Category = "Dotfiles"
			case strings.HasPrefix(s.Program, b.env.Home+"/."):
				it.Category = strings.TrimPrefix(strings.Split(strings.TrimPrefix(s.Program, b.env.Home+"/"), "/")[0], ".")
			case len(parts) > 1:
				it.Category = strings.ToUpper(parts[1][:1]) + parts[1][1:]
			default:
				it.Category = "Other"
			}
		}
		b.declare(&it, where)
		b.add(it)
	}
}

// Section: Declared but missing

func (b *builder) missing() {
	if !b.hasDotfiles() {
		return
	}
	d := b.snap.Dotfiles
	have := map[string]bool{}
	for _, it := range b.items {
		have[it.ID] = true
		if it.SourceRef != "" {
			have[string(it.Source)+":"+it.SourceRef] = true
		}
		if it.BundleID != "" {
			have["bundle:"+it.BundleID] = true
		}
	}
	miss := func(it Item, where string) {
		it.Declared, it.DeclaredIn, it.Category = DeclMissing, where, "Missing"
		it.Desc = "Declared in dotfiles but not installed"
		b.add(it)
	}
	for _, f := range d.Brews {
		if !have["brew:"+f] {
			kind := Terminal
			if _, ok := runtimeFormula(f); ok {
				kind = Runtime
			}
			miss(Item{ID: "missing:brew:" + f, Name: f, Kind: kind, Source: SrcBrew, SourceRef: f}, "nix/homebrew.nix")
		}
	}
	for _, c := range d.Casks {
		if !have["cask:"+c] {
			kind := Desktop
			if strings.HasPrefix(c, "font-") {
				kind = Font
			}
			miss(Item{ID: "missing:cask:" + c, Name: c, Kind: kind, Source: SrcCask, SourceRef: c}, "nix/homebrew.nix")
		}
	}
	for _, a := range d.Apps {
		if !have["bundle:"+a.ID] {
			miss(Item{ID: "missing:app:" + a.ID, Name: strings.TrimSuffix(a.Bundle, ".app"), Kind: Desktop, BundleID: a.ID,
				Source: SrcDotfiles, SourceRef: firstNonEmpty(a.Cask, a.URL)}, "nix/macos-apps.json")
		}
	}
	for _, t := range d.Tools {
		if !have["bin:"+t] {
			miss(Item{ID: "missing:tool:" + t, Name: t, Kind: Terminal, Source: SrcDotfiles, SourceRef: "managed tool"}, "scripts/managed-tools.json")
		}
	}
}

// Section: Cross-cutting enrichment

func (b *builder) finish() {
	b.collisions()
	u := b.snap.Updates
	for _, id := range b.order {
		it := b.items[id]
		switch it.Source {
		case SrcBrew:
			it.Latest = u.Formulae[it.SourceRefName()]
		case SrcCask:
			if v := caskVersion(u.Casks[it.SourceRef]); v != "" && v != "latest" && !SameVersion(v, it.Version) {
				it.Latest = v
			}
		case SrcAppStore:
			it.Latest = u.AppStore[it.SourceRef]
		case SrcNpm:
			it.Latest = u.Npm[it.Name]
		}
		if it.Latest != "" && !NewerVersion(it.Latest, it.Version) {
			it.Latest = ""
		}
		if it.Kind == Terminal {
			for _, n := range append([]string{it.Name}, it.Commands...) {
				it.ShellRuns = max(it.ShellRuns, b.snap.History[n])
			}
		}
		if it.Declared == DeclMissing {
			it.Path = ""
		}
	}
}

// collisions notes commands that more than one installed item puts on PATH.
func (b *builder) collisions() {
	owners := map[string][]*Item{}
	for _, id := range b.order {
		it := b.items[id]
		if it.Kind != Terminal && it.Kind != Runtime || it.Declared == DeclMissing || it.Dependency {
			continue
		}
		names := it.Commands
		if len(names) == 0 && it.Kind == Terminal {
			names = []string{it.Name}
		}
		for _, n := range names {
			owners[n] = append(owners[n], it)
		}
	}
	shared := map[*Item]map[*Item][]string{}
	for cmd, its := range owners {
		for _, it := range its {
			for _, other := range its {
				if other != it {
					if shared[it] == nil {
						shared[it] = map[*Item][]string{}
					}
					shared[it][other] = append(shared[it][other], cmd)
				}
			}
		}
	}
	for it, others := range shared {
		for other, cmds := range others {
			sort.Strings(cmds)
			it.Notes = append(it.Notes, fmt.Sprintf("Shares %s with %s (%s)", listPreview(cmds, 3), other.Name, other.Source))
		}
		sort.Strings(it.Notes)
	}
	for it, names := range b.dangling {
		sort.Strings(names)
		it.Notes = append(it.Notes, fmt.Sprintf("%d dangling links in /usr/local/bin (%s) point at a removed framework", len(names), listPreview(names, 3)))
	}
}

// listPreview joins up to n names and summarizes the rest.
func listPreview(names []string, n int) string {
	if len(names) <= n {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s +%d more", strings.Join(names[:n], ", "), len(names)-n)
}

// SourceRefName is the bare package name of a possibly tap-qualified reference.
func (it Item) SourceRefName() string { return it.SourceRef[strings.LastIndex(it.SourceRef, "/")+1:] }

var digitsRe = regexp.MustCompile(`\d+`)

// SameVersion compares numeric components, so "0.2026.10.21.08.25.02" matches "0.2026.10.21.08.25.stable_02".
func SameVersion(a, b string) bool {
	x, y := digitsRe.FindAllString(a, -1), digitsRe.FindAllString(b, -1)
	if len(x) > len(y) {
		x, y = y, x
	}
	if len(x) == 0 {
		return a == b
	}
	for i := range x {
		if strings.TrimLeft(x[i], "0") != strings.TrimLeft(y[i], "0") {
			return false
		}
	}
	return true
}

// NewerVersion reports whether a is a later version than b, comparing numeric components.
func NewerVersion(a, b string) bool {
	x, y := digitsRe.FindAllString(a, -1), digitsRe.FindAllString(b, -1)
	for i := 0; i < len(x) && i < len(y); i++ {
		xi, yi := strings.TrimLeft(x[i], "0"), strings.TrimLeft(y[i], "0")
		if len(xi) != len(yi) {
			return len(xi) > len(yi)
		}
		if xi != yi {
			return xi > yi
		}
	}
	return len(x) > len(y)
}

func appendOnce(list []string, s string) []string {
	if contains(list, s) {
		return list
	}
	return append(list, s)
}
