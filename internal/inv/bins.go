package inv

import (
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// BinEntry is one executable found in a user-managed bin directory.
type BinEntry struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Link        string    `json:"link,omitempty"`   // raw symlink text
	Target      string    `json:"target,omitempty"` // fully resolved path
	Broken      bool      `json:"broken,omitempty"`
	ShimApp     string    `json:"shim_app,omitempty"` // .app a wrapper script execs
	Size        int64     `json:"size"`
	InstalledAt time.Time `json:"installed_at"`
}

// BinDirs are scanned for hand-installed and installer-placed executables.
func (e *Env) BinDirs() []string {
	dirs := []string{
		filepath.Join(e.Home, ".local", "bin"),
		"/usr/local/bin",
		filepath.Join(e.Home, "bin"),
		filepath.Join(e.Home, "go", "bin"),
		filepath.Join(e.Home, ".cargo", "bin"),
		filepath.Join(e.Home, ".bun", "bin"),
		filepath.Join(e.Home, ".deno", "bin"),
	}
	// Brew's bin also holds cask-provided CLIs; formula links are filtered out when building items.
	if e.BrewPrefix != "" {
		dirs = append(dirs, filepath.Join(e.BrewPrefix, "bin"))
	}
	return dirs
}

var shimAppRe = regexp.MustCompile(`(/Applications/[^"'\s]+?\.app)/`)

func collectBins(env *Env) ([]BinEntry, error) {
	var out []BinEntry
	for _, dir := range env.BinDirs() {
		for _, name := range readDirNames(dir) {
			p := filepath.Join(dir, name)
			fi, err := os.Lstat(p)
			if err != nil || fi.IsDir() {
				continue
			}
			b := BinEntry{Name: name, Path: p, InstalledAt: birthTime(p)}
			if fi.Mode()&os.ModeSymlink != 0 {
				b.Link, _ = os.Readlink(p)
				if t, err := filepath.EvalSymlinks(p); err == nil {
					b.Target = t
					if st, err := os.Stat(t); err == nil {
						b.Size = st.Size()
					}
				} else {
					b.Broken = true
				}
			} else {
				if fi.Mode()&0o111 == 0 {
					continue
				}
				b.Target = p
				b.Size = fi.Size()
				if fi.Size() < 4096 {
					if raw, err := os.ReadFile(p); err == nil {
						if m := shimAppRe.FindStringSubmatch(string(raw)); m != nil {
							b.ShimApp = m[1]
						}
					}
				}
			}
			out = append(out, b)
		}
	}
	return out, nil
}
