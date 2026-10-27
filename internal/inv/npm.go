package inv

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// NpmPkg is one globally installed npm package.
type NpmPkg struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Desc        string    `json:"desc"`
	Homepage    string    `json:"homepage"`
	Prefix      string    `json:"prefix"`
	Path        string    `json:"path"`
	Bins        []string  `json:"bins"`
	InstalledAt time.Time `json:"installed_at"`
}

// npmBundled ships with node itself, so it is not a custom install.
var npmBundled = map[string]bool{"npm": true, "corepack": true}

// NpmPrefixes are the global install roots inspected for packages.
func (e *Env) NpmPrefixes() []string {
	var out []string
	if e.BrewPrefix != "" {
		out = append(out, e.BrewPrefix)
	}
	return append(out, filepath.Join(e.Home, ".local"), filepath.Join(e.Home, ".npm-global"))
}

func collectNpm(env *Env) ([]NpmPkg, error) {
	var pkgs []NpmPkg
	for _, prefix := range env.NpmPrefixes() {
		root := filepath.Join(prefix, "lib", "node_modules")
		for _, name := range readDirNames(root) {
			if strings.HasPrefix(name, ".") {
				continue
			}
			if strings.HasPrefix(name, "@") {
				for _, sub := range readDirNames(filepath.Join(root, name)) {
					if p, ok := readNpmPkg(filepath.Join(root, name, sub), prefix); ok {
						pkgs = append(pkgs, p)
					}
				}
				continue
			}
			if p, ok := readNpmPkg(filepath.Join(root, name), prefix); ok && !npmBundled[p.Name] {
				pkgs = append(pkgs, p)
			}
		}
	}
	return pkgs, nil
}

func readNpmPkg(dir, prefix string) (NpmPkg, bool) {
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return NpmPkg{}, false
	}
	var pj struct {
		Name        string          `json:"name"`
		Version     string          `json:"version"`
		Description string          `json:"description"`
		Homepage    string          `json:"homepage"`
		Bin         json.RawMessage `json:"bin"`
	}
	if json.Unmarshal(raw, &pj) != nil || pj.Name == "" {
		return NpmPkg{}, false
	}
	p := NpmPkg{Name: pj.Name, Version: pj.Version, Desc: pj.Description, Homepage: pj.Homepage,
		Prefix: prefix, Path: dir, InstalledAt: birthTime(dir)}
	var binMap map[string]string
	var binStr string
	switch {
	case json.Unmarshal(pj.Bin, &binMap) == nil:
		for k := range binMap {
			p.Bins = append(p.Bins, k)
		}
		sort.Strings(p.Bins)
	case json.Unmarshal(pj.Bin, &binStr) == nil:
		p.Bins = []string{pj.Name[strings.LastIndex(pj.Name, "/")+1:]}
	}
	return p, true
}
