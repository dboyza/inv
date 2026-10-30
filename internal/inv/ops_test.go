package inv

import (
	"strings"
	"testing"
	"time"
)

func TestPlansUseTheOwningPackageManager(t *testing.T) {
	env := &Env{Home: "/Users/me", BrewPrefix: "/opt/homebrew", Dotfiles: "/Users/me/dotfiles"}
	cases := []struct {
		item          Item
		upgrade, drop string // expected commands; "" means blocked
	}{
		{Item{Source: SrcBrew, SourceRef: "hashicorp/tap/terraform", Version: "1.16", Latest: "1.17"}, "brew upgrade hashicorp/tap/terraform", "brew uninstall hashicorp/tap/terraform"},
		{Item{Source: SrcCask, SourceRef: "docker-desktop", Version: "4.91", Latest: "4.97"}, "brew upgrade --cask docker-desktop", "brew uninstall --cask docker-desktop"},
		{Item{Source: SrcAppStore, SourceRef: "497799835", Version: "27.1", Latest: "27.2"}, "mas upgrade 497799835", "sudo mas uninstall 497799835"},
		{Item{Source: SrcNpm, Name: "gh-axi", Path: "/Users/me/.local/lib/node_modules/gh-axi", Version: "0.1", Latest: "0.2"},
			"npm install -g --prefix /Users/me/.local gh-axi@latest", "npm uninstall -g --prefix /Users/me/.local gh-axi"},
		{Item{Source: SrcUv, Version: "3.13.15", Latest: "3.13.16"}, "uv python upgrade 3.13", "uv python uninstall 3.13.15"},
		{Item{Source: SrcVSCode, SourceRef: "hashicorp.hcl"}, "code --install-extension hashicorp.hcl --force", "code --uninstall-extension hashicorp.hcl"},
		{Item{Source: SrcBundled, SourceRef: "Docker"}, "", ""},
		{Item{Source: SrcDotfiles, SourceRef: "managed tool", DeclaredIn: "scripts/managed-tools.json"}, "", ""},
		{Item{Source: SrcLazy}, "", ""},
	}
	for _, c := range cases {
		up, rm := PlanUpgrade(env, c.item, nil), PlanUninstall(env, c.item, nil)
		if got := allowedCommand(up); got != c.upgrade {
			t.Errorf("%s upgrade = %q (blocked %q), want %q", c.item.Source, got, up.Blocked, c.upgrade)
		}
		if got := allowedCommand(rm); got != c.drop {
			t.Errorf("%s uninstall = %q (blocked %q), want %q", c.item.Source, got, rm.Blocked, c.drop)
		}
	}
}

func allowedCommand(p Plan) string {
	if !p.Allowed() {
		return ""
	}
	return p.Command()
}

func TestUpgradeNeedsANewerVersion(t *testing.T) {
	p := PlanUpgrade(&Env{}, Item{Source: SrcBrew, SourceRef: "jq", Version: "1.8"}, nil)
	if p.Allowed() || p.Blocked != "Already up to date" {
		t.Errorf("plan = %+v", p)
	}
}

func TestUninstallWarnsAboutDeclarationsAndBundledCommands(t *testing.T) {
	env := &Env{Home: "/Users/me"}
	app := Item{ID: "app:docker", Source: SrcCask, SourceRef: "docker-desktop", Declared: DeclTracked, DeclaredIn: "nix/macos-apps.json"}
	all := []Item{app, {Name: "docker", Kind: Terminal, ProvidedBy: "app:docker"}}
	p := PlanUninstall(env, app, all)
	text := strings.Join(p.Warnings, "\n")
	if !strings.Contains(text, "nix/macos-apps.json") || !strings.Contains(text, "docker") {
		t.Errorf("warnings = %q", text)
	}
}

func TestTrashNeverTargetsSharedFolders(t *testing.T) {
	env := &Env{Home: "/Users/me", BrewPrefix: "/opt/homebrew", Dotfiles: "/Users/me/dotfiles"}
	for _, bad := range []string{"/Users/me", "/Users/me/Library/Fonts", "/Applications", "/usr/local/bin", "/opt/homebrew/bin", "/Users/me/.local/bin", "/Users/me/dotfiles/scripts/x", "relative/path", ""} {
		if safeToTrash(env, bad) {
			t.Errorf("safeToTrash(%q) = true", bad)
		}
	}
	for _, ok := range []string{"/Applications/Discord.app", "/Users/me/.local/bin/omp", "/Users/me/.no-mistakes", "/Users/me/Library/Fonts/Foo-Bold.ttf"} {
		if !safeToTrash(env, ok) {
			t.Errorf("safeToTrash(%q) = false", ok)
		}
	}
}

func TestManualFontTrashesFilesNotTheFontsFolder(t *testing.T) {
	env := &Env{Home: "/Users/me"}
	font := Item{Kind: Font, Source: SrcManual, Path: "/Users/me/Library/Fonts", Files: []string{"/Users/me/Library/Fonts/Foo-Bold.ttf"}}
	p := PlanUninstall(env, font, nil)
	if !p.Allowed() || strings.Contains(p.Command(), `POSIX file "/Users/me/Library/Fonts"}`) || !strings.Contains(p.Command(), "Foo-Bold.ttf") {
		t.Errorf("plan = %q blocked=%q", p.Command(), p.Blocked)
	}
	font.Files = nil
	if PlanUninstall(env, font, nil).Allowed() {
		t.Error("a font without known files must not be trashable")
	}
}

func TestInstallerUninstallAlsoTrashesItsFolder(t *testing.T) {
	env := &Env{Home: "/Users/me"}
	it := Item{Kind: Terminal, Source: SrcInstaller, SourceRef: "~/.no-mistakes", Path: "/Users/me/.local/bin/no-mistakes"}
	p := PlanUninstall(env, it, nil)
	if !p.Allowed() || !strings.Contains(p.Command(), "/Users/me/.no-mistakes") {
		t.Errorf("plan = %q blocked=%q", p.Command(), p.Blocked)
	}
}

func TestExportUsesDotfilesFormats(t *testing.T) {
	items := []Item{
		{Name: "fastfetch", Source: SrcBrew, SourceRef: "fastfetch", Declared: DeclUntracked, Kind: Terminal},
		{Name: "Docker", Source: SrcCask, SourceRef: "docker-desktop", Kind: Desktop, Path: "/Applications/Docker.app", BundleID: "com.docker.docker", Declared: DeclUntracked},
		{Name: "Xcode", Source: SrcAppStore, SourceRef: "497799835", Kind: Desktop, Path: "/Applications/Xcode.app", BundleID: "com.apple.dt.Xcode", Declared: DeclUntracked},
		{Name: "omp", Source: SrcManual, Kind: Terminal, Path: "/Users/me/.local/bin/omp", Declared: DeclUntracked},
		{Name: "jq", Source: SrcBrew, SourceRef: "jq", Declared: DeclTracked},
		{Name: "libuv", Source: SrcBrew, SourceRef: "libuv", Declared: DeclUntracked, Dependency: true},
	}
	out := ExportUntracked(items, "mac", time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))
	for _, want := range []string{`"fastfetch"`, `"cask": "docker-desktop"`, `"app_store_id": 497799835`, "omp"} {
		if !strings.Contains(out, want) {
			t.Errorf("export missing %s:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{`"jq"`, "libuv"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("export should skip %s", unwanted)
		}
	}
}
