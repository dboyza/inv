package inv

import (
	"testing"
	"time"
)

// Section: Fixture

// fixture models a small machine: a cask app with bundled CLIs, a declared formula,
// a dependency, managed and loose binaries, an npm global, and a declared-but-missing app.
func fixture() (*Env, Snapshot) {
	env := &Env{Home: "/Users/me", BrewPrefix: "/opt/homebrew", Dotfiles: "/Users/me/dotfiles"}
	s := Snapshot{
		Brew: BrewData{
			Formulae: []BrewFormula{
				{Name: "terraform", Tap: "hashicorp/tap", Version: "1.16.2", OnRequest: true, Bins: []string{"terraform"}},
				{Name: "kubernetes-cli", Version: "1.37.0", OnRequest: true, Bins: []string{"kubectl"}},
				{Name: "libuv", Version: "1.52.1"},
				{Name: "python@3.14", Version: "3.14.7", OnRequest: true},
			},
			Casks: []BrewCask{{Token: "docker-desktop", Installed: "4.91.0,239619", Apps: []string{"Docker.app"}}},
		},
		Apps: []Bundle{
			{Path: "/Applications/Docker.app", Name: "Docker", ID: "com.docker.docker", Version: "4.91.0"},
			{Path: "/Applications/Safari.app", Name: "Safari", ID: "com.apple.Safari"},
			{Path: "/Applications/Xcode.app", Name: "Xcode", ID: "com.apple.dt.Xcode", AppStore: true, AppStoreID: "497799835"},
		},
		Bins: []BinEntry{
			{Name: "docker", Path: "/usr/local/bin/docker", Target: "/Applications/Docker.app/Contents/Resources/bin/docker"},
			{Name: "kubectl", Path: "/usr/local/bin/kubectl", Target: "/Applications/Docker.app/Contents/Resources/bin/kubectl"},
			{Name: "claude", Path: "/Users/me/.local/bin/claude", Link: "/nix/store/x-home-manager-files/.local/bin/claude", Target: "/Users/me/dotfiles/scripts/claude"},
			{Name: "treehouse", Path: "/Users/me/.local/bin/treehouse", Target: "/Users/me/.local/bin/treehouse", Size: 10},
			{Name: "tbo", Path: "/Users/me/.local/bin/tbo", Link: "theboringfloor", Broken: true},
			{Name: "terraform", Path: "/opt/homebrew/bin/terraform", Target: "/opt/homebrew/Cellar/terraform/1.16.2/bin/terraform"},
		},
		Npm: []NpmPkg{{Name: "gh-axi", Version: "0.1.35", Prefix: "/Users/me/.local", Bins: []string{"gh-axi"}}},
		Dotfiles: DotfilesData{
			Repo:         "/Users/me/dotfiles",
			Brews:        []string{"terraform", "kubernetes-cli", "jq"},
			Apps:         []DeclaredApp{{Bundle: "Stats.app", ID: "eu.exelban.Stats", Cask: "stats"}},
			Tools:        []string{"claude"},
			ToolVersions: map[string]string{"claude": "2.1.311"},
		},
		History: map[string]int{"kubectl": 16, "terraform": 3},
		Updates: Updates{Formulae: map[string]string{"terraform": "1.17.0"}, Casks: map[string]string{"docker-desktop": "4.97.0,244584"}},
	}
	return env, s
}

func byID(items []Item) map[string]Item {
	m := map[string]Item{}
	for _, it := range items {
		m[it.ID] = it
	}
	return m
}

// Section: Classification

func TestBuildClassifiesEverySource(t *testing.T) {
	env, s := fixture()
	items := byID(Build(env, s))

	docker := items["app:/Applications/Docker.app"]
	if docker.Kind != Desktop || docker.Source != SrcCask || docker.SourceRef != "docker-desktop" {
		t.Errorf("docker app = %+v", docker)
	}
	if docker.Latest != "4.97.0" || docker.Declared != DeclUntracked {
		t.Errorf("docker latest/declared = %q/%v", docker.Latest, docker.Declared)
	}
	if len(docker.Provides) != 2 {
		t.Errorf("docker provides %v, want docker and kubectl", docker.Provides)
	}
	if _, ok := items["app:/Applications/Safari.app"]; ok {
		t.Error("system Safari must be excluded")
	}
	if x := items["app:/Applications/Xcode.app"]; !x.Apple || x.Category != "Apple" || x.Source != SrcAppStore {
		t.Errorf("xcode = %+v", x)
	}

	cli := items["bundled:com.docker.docker:kubectl"]
	if cli.Source != SrcBundled || cli.ProvidedBy != docker.ID || cli.ShellRuns != 16 {
		t.Errorf("bundled kubectl = %+v", cli)
	}
	if len(cli.Notes) == 0 {
		t.Error("kubectl from Docker and kubernetes-cli should note the shared command")
	}

	tf := items["brew:terraform"]
	if tf.Declared != DeclTracked || tf.SourceRef != "hashicorp/tap/terraform" || tf.Latest != "1.17.0" || tf.Category != "Cloud & infra" {
		t.Errorf("terraform = %+v", tf)
	}
	if dep := items["brew:libuv"]; !dep.Dependency || dep.Category != "Dependencies" || dep.Declared != DeclUnknown {
		t.Errorf("libuv = %+v", dep)
	}
	if py := items["brew:python@3.14"]; py.Kind != Runtime || py.Name != "Python 3.14" {
		t.Errorf("python formula = %+v", py)
	}

	if c := items["bin:claude"]; c.Source != SrcDotfiles || c.Version != "2.1.311" || c.Declared != DeclTracked {
		t.Errorf("claude = %+v", c)
	}
	if th := items["bin:treehouse"]; th.Source != SrcManual || th.Declared != DeclUntracked {
		t.Errorf("treehouse = %+v", th)
	}
	if tbo := items["bin:tbo"]; len(tbo.Notes) == 0 {
		t.Errorf("broken tbo link should carry a note: %+v", tbo)
	}
	if _, ok := items["bin:terraform"]; ok {
		t.Error("formula links in brew's bin must not become loose binaries")
	}
	if npm := items["npm:gh-axi"]; npm.Source != SrcNpm || npm.Category != "Agent tooling" {
		t.Errorf("gh-axi = %+v", npm)
	}
}

func TestBuildReportsDeclaredButMissing(t *testing.T) {
	env, s := fixture()
	items := byID(Build(env, s))
	if jq := items["missing:brew:jq"]; jq.Declared != DeclMissing || jq.Kind != Terminal {
		t.Errorf("jq = %+v", jq)
	}
	if st := items["missing:app:eu.exelban.Stats"]; st.Declared != DeclMissing || st.Kind != Desktop {
		t.Errorf("stats = %+v", st)
	}
}

func TestBuildWithoutDotfilesLeavesDriftUnknown(t *testing.T) {
	env, s := fixture()
	s.Dotfiles = DotfilesData{}
	for _, it := range Build(env, s) {
		if it.Declared != DeclUnknown {
			t.Fatalf("%s declared=%v without a dotfiles repo", it.Name, it.Declared)
		}
	}
}

// Section: Versions and diffs

func TestVersionComparison(t *testing.T) {
	same := [][2]string{{"0.2026.10.21.08.25.02", "0.2026.10.21.08.25.stable_02"}, {"1.13.1", "1.13.1"}, {"4.91.0", "4.91"}}
	for _, c := range same {
		if !SameVersion(c[0], c[1]) {
			t.Errorf("SameVersion(%q, %q) = false", c[0], c[1])
		}
	}
	if SameVersion("4.91.0", "4.97.0") {
		t.Error("4.91.0 and 4.97.0 differ")
	}
	newer := [][2]string{{"1.17.0", "1.16.2"}, {"3.14.10", "3.14.7"}, {"2.56.0_1", "2.55.0"}}
	for _, c := range newer {
		if !NewerVersion(c[0], c[1]) {
			t.Errorf("NewerVersion(%q, %q) = false", c[0], c[1])
		}
	}
	if NewerVersion("3.5.9", "3.6.4") {
		t.Error("an older 'latest' must not count as an update")
	}
}

func TestDiffIgnoresDependenciesAndPlaceholders(t *testing.T) {
	before := []Item{{ID: "a", Version: "1"}, {ID: "dep", Dependency: true}, {ID: "gone"}}
	after := []Item{{ID: "a", Version: "2"}, {ID: "new"}, {ID: "dep2", Dependency: true}, {ID: "m", Declared: DeclMissing}}
	got := map[string]ChangeKind{}
	for _, c := range Diff(before, after) {
		got[c.Item.ID] = c.Kind
	}
	want := map[string]ChangeKind{"a": Changed, "new": Added, "gone": Removed}
	if len(got) != len(want) {
		t.Fatalf("Diff = %v, want %v", got, want)
	}
	for id, k := range want {
		if got[id] != k {
			t.Errorf("%s: got %v want %v", id, got[id], k)
		}
	}
}

func TestCommandWordSkipsWrappers(t *testing.T) {
	cases := map[string]string{"sudo terraform apply": "terraform", "FOO=1 kubectl get pods": "kubectl", " git status": "git", "": ""}
	for in, want := range cases {
		if got := commandWord(in); got != want {
			t.Errorf("commandWord(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInstalledAtFromCask(t *testing.T) {
	env, s := fixture()
	s.Brew.Casks[0].Time = 1_700_000_000
	docker := byID(Build(env, s))["app:/Applications/Docker.app"]
	if !docker.InstalledAt.Equal(time.Unix(1_700_000_000, 0)) {
		t.Errorf("installed at %v", docker.InstalledAt)
	}
}
