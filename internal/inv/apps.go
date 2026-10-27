package inv

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"howett.net/plist"
)

// Bundle is one .app found on disk.
type Bundle struct {
	Path        string    `json:"path"`
	Name        string    `json:"name"`
	ID          string    `json:"id"`
	Version     string    `json:"version"`
	Category    string    `json:"category"` // LSApplicationCategoryType
	Copyright   string    `json:"copyright"`
	MenuBar     bool      `json:"menu_bar"`
	AppStore    bool      `json:"app_store"`
	AppStoreID  string    `json:"app_store_id,omitempty"`
	WebApp      bool      `json:"web_app"`
	InstalledAt time.Time `json:"installed_at"`
	LastUsed    time.Time `json:"last_used"`
}

// appSkipDirs are /Applications subfolders owned by another inventory kind.
var appSkipDirs = map[string]bool{"Python 3.14": true}

func collectApps(env *Env) ([]Bundle, error) {
	var bundles []Bundle
	for _, root := range []string{"/Applications", filepath.Join(env.Home, "Applications")} {
		for _, name := range readDirNames(root) {
			if strings.HasPrefix(name, ".") {
				continue // hidden updater backups and staging copies
			}
			p := filepath.Join(root, name)
			switch {
			case strings.HasSuffix(name, ".app"):
				if b, ok := readBundle(p); ok {
					bundles = append(bundles, b)
				}
			case isDir(p) && !appSkipDirs[name] && !strings.HasPrefix(name, "Python 3."):
				// One level of grouping folders (Utilities, Nix Apps, Chrome Apps.localized).
				for _, sub := range readDirNames(p) {
					if strings.HasSuffix(sub, ".app") {
						if b, ok := readBundle(filepath.Join(p, sub)); ok {
							b.WebApp = strings.HasPrefix(name, "Chrome Apps")
							bundles = append(bundles, b)
						}
					}
				}
			}
		}
	}
	fillLastUsed(bundles)
	fillAppStoreIDs(bundles)
	return bundles, nil
}

func readBundle(p string) (Bundle, bool) {
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return Bundle{}, false
	}
	raw, err := os.ReadFile(filepath.Join(real, "Contents", "Info.plist"))
	if err != nil {
		return Bundle{}, false
	}
	var info map[string]any
	if _, err := plist.Unmarshal(raw, &info); err != nil {
		return Bundle{}, false
	}
	str := func(k string) string { s, _ := info[k].(string); return strings.TrimSpace(s) }
	b := Bundle{
		Path:        p,
		Name:        firstNonEmpty(str("CFBundleDisplayName"), str("CFBundleName"), strings.TrimSuffix(filepath.Base(p), ".app")),
		ID:          str("CFBundleIdentifier"),
		Version:     firstNonEmpty(str("CFBundleShortVersionString"), str("CFBundleVersion")),
		Category:    str("LSApplicationCategoryType"),
		Copyright:   str("NSHumanReadableCopyright"),
		AppStore:    isDir(filepath.Join(real, "Contents", "_MASReceipt")),
		InstalledAt: birthTime(p),
	}
	switch v := info["LSUIElement"].(type) {
	case bool:
		b.MenuBar = v
	case string:
		b.MenuBar = v == "1" || v == "YES"
	}
	// Finder and Spotlight show the folder name ("Visual Studio Code", not "Code").
	b.Name = strings.TrimSuffix(filepath.Base(p), ".app")
	return b, true
}

// fillLastUsed asks Spotlight for every bundle's last-used date in one mdls call.
func fillLastUsed(bundles []Bundle) {
	if len(bundles) == 0 {
		return
	}
	args := []string{"-raw", "-nullMarker", "-", "-name", "kMDItemLastUsedDate"}
	for _, b := range bundles {
		args = append(args, b.Path)
	}
	out, err := run(10*time.Second, "/usr/bin/mdls", args...)
	if err != nil && len(out) == 0 {
		return
	}
	vals := bytes.Split(out, []byte{0})
	for i := range bundles {
		if i < len(vals) {
			if t, err := time.Parse("2006-01-02 15:04:05 -0700", strings.TrimSpace(string(vals[i]))); err == nil {
				bundles[i].LastUsed = t
			}
		}
	}
}

// birthTime returns a file's creation time, which approximates when it was installed.
func birthTime(p string) time.Time {
	var st syscall.Stat_t
	if syscall.Lstat(p, &st) != nil {
		return time.Time{}
	}
	return time.Unix(st.Birthtimespec.Sec, st.Birthtimespec.Nsec)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

var masLineRe = regexp.MustCompile(`^\s*(\d+)\s+(.+?)\s+\(`)

// fillAppStoreIDs matches `mas list` names to receipted bundles so store links and updates work.
func fillAppStoreIDs(bundles []Bundle) {
	out, err := run(5*time.Second, "mas", "list")
	if err != nil {
		return
	}
	ids := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		if m := masLineRe.FindStringSubmatch(line); m != nil {
			ids[m[2]] = m[1]
		}
	}
	for i, b := range bundles {
		if !b.AppStore {
			continue
		}
		base := strings.TrimSuffix(filepath.Base(b.Path), ".app")
		bundles[i].AppStoreID = firstNonEmpty(ids[b.Name], ids[base], ids[strings.Fields(base)[0]])
	}
}
