package inv

import (
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// debounce lets bursts of writes (brew installs, app copies) settle into one rescan.
const debounce = 700 * time.Millisecond

// settleDelay re-runs a rescan once more, since apps and packages can appear before they finish copying.
const settleDelay = 4 * time.Second

// WatchTargets maps each watched directory to the collectors it affects.
func (e *Env) WatchTargets() map[string][]Group {
	t := map[string][]Group{}
	add := func(dir string, groups ...Group) {
		if isDir(dir) {
			t[dir] = append(t[dir], groups...)
		}
	}
	for _, d := range []string{"/Applications", "/Applications/Utilities", filepath.Join(e.Home, "Applications")} {
		add(d, GApps)
	}
	if e.BrewPrefix != "" {
		add(filepath.Join(e.BrewPrefix, "Cellar"), GBrew)
		add(filepath.Join(e.BrewPrefix, "Caskroom"), GBrew)
		add(filepath.Join(e.BrewPrefix, "var", "homebrew", "linked"), GBrew) // changes on upgrade
	}
	for _, d := range e.BinDirs() {
		add(d, GBins)
	}
	for _, p := range e.NpmPrefixes() {
		add(filepath.Join(p, "lib", "node_modules"), GNpm)
	}
	for _, vm := range versionManagers(e.Home) {
		add(vm.dir, GRuntimes)
	}
	add("/Library/Frameworks/Python.framework/Versions", GRuntimes)
	for _, d := range e.FontDirs() {
		add(d, GFonts)
	}
	for _, d := range e.ExtensionDirs() {
		add(d, GExts)
	}
	for d := range e.ServiceDirs() {
		add(d, GServices)
	}
	if e.Dotfiles != "" {
		add(filepath.Join(e.Dotfiles, "nix"), GDotfiles)
		add(filepath.Join(e.Dotfiles, "scripts"), GDotfiles)
	}
	add(filepath.Join(e.Home, ".local", "share", "dotfiles", "tools"), GDotfiles)
	return t
}

// Watcher turns filesystem changes into batches of collector groups to rescan.
type Watcher struct {
	fs      *fsnotify.Watcher
	targets map[string][]Group
	Changes chan []Group
	Count   int // number of directories being watched
}

// NewWatcher starts watching every existing target directory.
func NewWatcher(env *Env) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{fs: fw, targets: env.WatchTargets(), Changes: make(chan []Group, 4)}
	for dir := range w.targets {
		if fw.Add(dir) == nil {
			w.Count++
		}
	}
	go w.loop()
	return w, nil
}

func (w *Watcher) loop() {
	pending := map[Group]bool{}
	var settle map[Group]bool
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	settleTimer := time.NewTimer(time.Hour)
	settleTimer.Stop()
	flush := func(set map[Group]bool) {
		groups := make([]Group, 0, len(set)+1)
		for g := range set {
			groups = append(groups, g)
		}
		w.Changes <- append(groups, GHistory)
	}
	for {
		select {
		case ev, ok := <-w.fs.Events:
			if !ok {
				return
			}
			// Events name the changed entry; its parent is the watched directory.
			groups := w.targets[filepath.Dir(ev.Name)]
			if len(groups) == 0 {
				groups = w.targets[ev.Name]
			}
			for _, g := range groups {
				pending[g] = true
			}
			if len(groups) > 0 {
				timer.Reset(debounce)
			}
		case <-timer.C:
			if len(pending) > 0 {
				flush(pending)
				settle, pending = pending, map[Group]bool{}
				settleTimer.Reset(settleDelay)
			}
		case <-settleTimer.C:
			if len(settle) > 0 {
				flush(settle)
				settle = nil
			}
		case _, ok := <-w.fs.Errors:
			if !ok {
				return
			}
		}
	}
}

// Close stops the watcher.
func (w *Watcher) Close() error { return w.fs.Close() }
