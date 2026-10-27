package inv

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Section: Snapshot of raw observations

// Snapshot holds everything observed on disk; Build turns it into items without further I/O.
type Snapshot struct {
	Brew        BrewData          `json:"brew"`
	Apps        []Bundle          `json:"apps"`
	Bins        []BinEntry        `json:"bins"`
	Npm         []NpmPkg          `json:"npm"`
	Runtimes    []RuntimeRaw      `json:"runtimes"`
	Fonts       []FontFile        `json:"fonts"`
	Exts        []ExtRaw          `json:"exts"`
	Services    []ServiceRaw      `json:"services"`
	Dotfiles    DotfilesData      `json:"dotfiles"`
	History     map[string]int    `json:"history"`
	Updates     Updates           `json:"updates"`
	BinVersions map[string]string `json:"bin_versions"` // resolved path -> version, from --version probes
	ScannedAt   time.Time         `json:"scanned_at"`
	Errors      map[Group]string  `json:"errors,omitempty"`
}

// Updates holds the latest known versions for outdated items, keyed by package name.
type Updates struct {
	CheckedAt time.Time         `json:"checked_at"`
	Formulae  map[string]string `json:"formulae"`
	Casks     map[string]string `json:"casks"`
	AppStore  map[string]string `json:"app_store"` // keyed by App Store ID
	Npm       map[string]string `json:"npm"`
}

// Group identifies one independently refreshable collector.
type Group string

const (
	GBrew     Group = "brew"
	GApps     Group = "apps"
	GBins     Group = "bins"
	GNpm      Group = "npm"
	GRuntimes Group = "runtimes"
	GFonts    Group = "fonts"
	GExts     Group = "extensions"
	GServices Group = "services"
	GDotfiles Group = "dotfiles"
	GHistory  Group = "history"
)

// AllGroups is every collector, used for a full scan.
var AllGroups = []Group{GBrew, GApps, GBins, GNpm, GRuntimes, GFonts, GExts, GServices, GDotfiles, GHistory}

// Section: Collection

// Collect re-runs the given collectors concurrently on top of prev and returns the merged snapshot.
func Collect(env *Env, prev Snapshot, groups []Group) Snapshot {
	next := prev
	next.Errors = map[Group]string{}
	for g, e := range prev.Errors {
		next.Errors[g] = e
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	record := func(g Group, err error) {
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			next.Errors[g] = err.Error()
		} else {
			delete(next.Errors, g)
		}
	}
	// Each collector writes only its own field of next, so they can run in parallel.
	for _, g := range groups {
		wg.Add(1)
		go func(g Group) {
			defer wg.Done()
			var err error
			switch g {
			case GBrew:
				next.Brew, err = collectBrew(env)
			case GApps:
				next.Apps, err = collectApps(env)
			case GBins:
				next.Bins, err = collectBins(env)
			case GNpm:
				next.Npm, err = collectNpm(env)
			case GRuntimes:
				next.Runtimes, err = collectRuntimes(env)
			case GFonts:
				next.Fonts, err = collectFonts(env)
			case GExts:
				next.Exts, err = collectExtensions(env)
			case GServices:
				next.Services, err = collectServices(env)
			case GDotfiles:
				next.Dotfiles, err = collectDotfiles(env)
			case GHistory:
				next.History, err = collectHistory(env)
			}
			record(g, err)
		}(g)
	}
	wg.Wait()
	next.ScannedAt = time.Now()
	return next
}

// Section: Cache

func cachePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "inv", "snapshot.json")
}

// LoadCache returns the last saved snapshot so the UI can paint before the first scan finishes.
func LoadCache() (Snapshot, bool) {
	raw, err := os.ReadFile(cachePath())
	if err != nil {
		return Snapshot{}, false
	}
	var s Snapshot
	if json.Unmarshal(raw, &s) != nil {
		return Snapshot{}, false
	}
	return s, true
}

// SaveCache writes the snapshot atomically so a half-written cache is never read.
func SaveCache(s Snapshot) error {
	p := cachePath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
