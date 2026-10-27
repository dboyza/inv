package inv

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DotfilesData is what the dotfiles repo declares should be installed.
type DotfilesData struct {
	Repo         string            `json:"repo"`
	Brews        []string          `json:"brews"`         // formula names, tap prefix removed
	Casks        []string          `json:"casks"`         // cask tokens, tap prefix removed
	Apps         []DeclaredApp     `json:"apps"`          // nix/macos-apps.json
	Tools        []string          `json:"tools"`         // scripts/managed-tools.json keys
	ToolVersions map[string]string `json:"tool_versions"` // from ~/.local/share/dotfiles/tools/*/current.json
	Corpus       map[string]string `json:"corpus"`        // nix/ and scripts/ text, for mention lookups
}

// DeclaredApp is one entry from nix/macos-apps.json.
type DeclaredApp struct {
	Bundle     string `json:"bundle"`
	ID         string `json:"id"`
	Cask       string `json:"cask,omitempty"`
	AppStoreID int64  `json:"app_store_id,omitempty"`
	URL        string `json:"url,omitempty"`
	Build      string `json:"build,omitempty"`
}

var nixListRe = regexp.MustCompile(`(?s)\b(brews|casks)\s*=\s*\[(.*?)\];`)
var nixStringRe = regexp.MustCompile(`"([^"]+)"`)

func collectDotfiles(env *Env) (DotfilesData, error) {
	d := DotfilesData{Repo: env.Dotfiles, ToolVersions: map[string]string{}, Corpus: map[string]string{}}
	if env.Dotfiles == "" {
		return d, nil
	}
	if raw, err := os.ReadFile(filepath.Join(env.Dotfiles, "nix", "homebrew.nix")); err == nil {
		for _, m := range nixListRe.FindAllStringSubmatch(stripNixComments(string(raw)), -1) {
			for _, s := range nixStringRe.FindAllStringSubmatch(m[2], -1) {
				name := s[1][strings.LastIndex(s[1], "/")+1:]
				if m[1] == "brews" {
					d.Brews = append(d.Brews, name)
				} else {
					d.Casks = append(d.Casks, name)
				}
			}
		}
	}
	if raw, err := os.ReadFile(filepath.Join(env.Dotfiles, "nix", "macos-apps.json")); err == nil {
		_ = json.Unmarshal(raw, &d.Apps)
	}
	if raw, err := os.ReadFile(filepath.Join(env.Dotfiles, "scripts", "managed-tools.json")); err == nil {
		var tools map[string]json.RawMessage
		if json.Unmarshal(raw, &tools) == nil {
			for name := range tools {
				d.Tools = append(d.Tools, name)
			}
		}
	}
	toolsDir := filepath.Join(env.Home, ".local", "share", "dotfiles", "tools")
	for _, name := range readDirNames(toolsDir) {
		var cur struct {
			Current struct{ Version string } `json:"current"`
		}
		if raw, err := os.ReadFile(filepath.Join(toolsDir, name, "current.json")); err == nil && json.Unmarshal(raw, &cur) == nil {
			d.ToolVersions[name] = cur.Current.Version
		}
	}
	// Small text corpus lets services and hand-built apps count as declared when a script installs them.
	for _, sub := range []string{"nix", "scripts"} {
		_ = filepath.WalkDir(filepath.Join(env.Dotfiles, sub), func(p string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() {
				return nil
			}
			if info, err := e.Info(); err == nil && info.Size() < 512<<10 {
				if raw, err := os.ReadFile(p); err == nil {
					rel, _ := filepath.Rel(env.Dotfiles, p)
					d.Corpus[rel] = string(raw)
				}
			}
			return nil
		})
	}
	return d, nil
}

// stripNixComments removes # line comments so commented-out packages are not counted.
func stripNixComments(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if j := strings.Index(l, "#"); j >= 0 && !strings.Contains(l[:j], `"`) {
			lines[i] = l[:j]
		}
	}
	return strings.Join(lines, "\n")
}

// Mentions returns the first dotfiles file containing needle, preferring nix/ declarations.
func (d DotfilesData) Mentions(needle string) string {
	if needle == "" {
		return ""
	}
	best := ""
	for file, text := range d.Corpus {
		if strings.Contains(text, needle) && (best == "" || file < best) {
			best = file
		}
	}
	return best
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
