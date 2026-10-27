package inv

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Section: Update checks (network)

// CheckUpdates asks brew, mas, and npm for newer versions; each source fails independently.
func CheckUpdates(env *Env, s Snapshot) Snapshot {
	u := Updates{CheckedAt: time.Now(), Formulae: map[string]string{}, Casks: map[string]string{},
		AppStore: map[string]string{}, Npm: map[string]string{}}
	var wg sync.WaitGroup
	var mu sync.Mutex
	if env.BrewPrefix != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// --greedy includes self-updating casks; Build compares against the bundle's real version.
			out, _ := run(90*time.Second, filepath.Join(env.BrewPrefix, "bin", "brew"), "outdated", "--json=v2", "--greedy")
			type outdated struct {
				Name           string `json:"name"`
				CurrentVersion string `json:"current_version"`
			}
			var res struct {
				Formulae []outdated `json:"formulae"`
				Casks    []outdated `json:"casks"`
			}
			if json.Unmarshal(out, &res) == nil {
				mu.Lock()
				for _, f := range res.Formulae {
					u.Formulae[f.Name] = f.CurrentVersion
				}
				for _, c := range res.Casks {
					u.Casks[c.Name] = c.CurrentVersion
				}
				mu.Unlock()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		out, _ := run(60*time.Second, "mas", "outdated")
		re := regexp.MustCompile(`^\s*(\d+)\s+.+\(.*?->\s*([^)]+)\)`)
		mu.Lock()
		defer mu.Unlock()
		for _, line := range strings.Split(string(out), "\n") {
			if m := re.FindStringSubmatch(line); m != nil {
				u.AppStore[m[1]] = strings.TrimSpace(m[2])
			}
		}
	}()
	prefixes := map[string]bool{}
	for _, p := range s.Npm {
		prefixes[p.Prefix] = true
	}
	for prefix := range prefixes {
		wg.Add(1)
		go func(prefix string) {
			defer wg.Done()
			out, _ := run(60*time.Second, "npm", "outdated", "-g", "--json", "--prefix", prefix)
			var res map[string]struct{ Current, Latest string }
			if json.Unmarshal(out, &res) == nil {
				mu.Lock()
				for name, r := range res {
					if r.Latest != "" && r.Latest != r.Current {
						u.Npm[name] = r.Latest
					}
				}
				mu.Unlock()
			}
		}(prefix)
	}
	wg.Wait()
	s.Updates = u
	return s
}

// Section: Version probes for loose binaries

var versionRe = regexp.MustCompile(`v?(\d+\.\d+(?:\.\d+)?(?:[-+][0-9A-Za-z.]+)?)`)

// probeKey changes whenever the binary is replaced, so stale versions are re-probed.
func probeKey(e BinEntry) string { return fmt.Sprintf("%s@%d", e.Target, e.Size) }

// ProbeVersions runs `--version` on standalone executables that have no other version source.
func ProbeVersions(env *Env, s Snapshot) Snapshot {
	versions := map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, e := range s.Bins {
		if !probeable(env, e) {
			continue
		}
		key := probeKey(e)
		if v, ok := s.BinVersions[key]; ok {
			versions[key] = v
			continue
		}
		wg.Add(1)
		go func(e BinEntry, key string) {
			defer wg.Done()
			out, _ := run(3*time.Second, e.Target, "--version")
			v := ""
			if m := versionRe.FindSubmatch(firstLine(out)); m != nil {
				v = string(m[1])
			}
			mu.Lock()
			versions[key] = v
			mu.Unlock()
		}(e, key)
	}
	wg.Wait()
	s.BinVersions = versions
	return s
}

// probeable limits probes to plain executables outside package managers and app bundles.
func probeable(env *Env, e BinEntry) bool {
	if e.Broken || e.ShimApp != "" || e.Size == 0 && e.Target == e.Path {
		return false
	}
	t := e.Target
	switch {
	case strings.HasPrefix(t, "/Applications/"), strings.Contains(t, "/Caskroom/"), strings.Contains(t, "/Cellar/"),
		strings.Contains(t, "/lib/node_modules/"), strings.Contains(t, "Python.framework"),
		strings.Contains(t, "/uv/python/"), strings.Contains(e.Link, "-home-manager-files/"), strings.HasPrefix(t, "/nix/store/"):
		return false
	case env.Dotfiles != "" && strings.HasPrefix(t, env.Dotfiles+"/"):
		return false
	case env.BrewPrefix != "" && filepath.Dir(e.Path) == filepath.Join(env.BrewPrefix, "bin"):
		return false
	}
	return !pathVersionRe.MatchString(strings.TrimPrefix(t, env.Home))
}

func firstLine(b []byte) []byte {
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) != "" {
			return []byte(line)
		}
	}
	return nil
}
