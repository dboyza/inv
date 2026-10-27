package inv

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"howett.net/plist"
)

// RuntimeRaw is one language toolchain found outside Homebrew.
type RuntimeRaw struct {
	Name        string    `json:"name"` // language, e.g. "Python"
	Version     string    `json:"version"`
	Source      Source    `json:"source"`
	Manager     string    `json:"manager"` // human label for the installer
	Path        string    `json:"path"`
	InstalledAt time.Time `json:"installed_at"`
}

// versionManager describes a directory where a tool keeps one subfolder per installed version.
type versionManager struct {
	dir, lang, manager string
	src                Source
	pattern            *regexp.Regexp // first group is the version
}

var (
	uvPythonRe = regexp.MustCompile(`^(?:cpython|pypy|graalpy)-(\d+\.\d+\.\d+[a-z0-9+]*)-`)
	plainVerRe = regexp.MustCompile(`^v?(\d+(?:\.\d+)+[a-z0-9.-]*)`)
	rustupRe   = regexp.MustCompile(`^((?:stable|beta|nightly|\d+\.\d+(?:\.\d+)?)[^-]*)-`)
)

func versionManagers(home string) []versionManager {
	return []versionManager{
		{filepath.Join(home, ".local/share/uv/python"), "Python", "uv", SrcUv, uvPythonRe},
		{filepath.Join(home, ".pyenv/versions"), "Python", "pyenv", SrcManual, plainVerRe},
		{filepath.Join(home, ".nvm/versions/node"), "Node.js", "nvm", SrcManual, plainVerRe},
		{filepath.Join(home, ".local/share/fnm/node-versions"), "Node.js", "fnm", SrcManual, plainVerRe},
		{filepath.Join(home, ".rbenv/versions"), "Ruby", "rbenv", SrcManual, plainVerRe},
		{filepath.Join(home, ".rustup/toolchains"), "Rust", "rustup", SrcInstaller, rustupRe},
		{filepath.Join(home, "sdk"), "Go", "go sdk", SrcManual, regexp.MustCompile(`^go(\d+\.\d+(?:\.\d+)?)`)},
	}
}

func collectRuntimes(env *Env) ([]RuntimeRaw, error) {
	var out []RuntimeRaw
	for _, vm := range versionManagers(env.Home) {
		for _, name := range readDirNames(vm.dir) {
			p := filepath.Join(vm.dir, name)
			if fi, err := os.Lstat(p); err != nil || !fi.IsDir() {
				continue // uv keeps minor-version symlinks beside the real installs
			}
			if m := vm.pattern.FindStringSubmatch(name); m != nil {
				lang := vm.lang
				if strings.HasPrefix(name, "pypy") {
					lang = "PyPy"
				}
				out = append(out, RuntimeRaw{Name: lang, Version: m[1], Source: vm.src, Manager: vm.manager, Path: p, InstalledAt: birthTime(p)})
			}
		}
	}
	// python.org installers place framework builds under /Library/Frameworks.
	for _, fw := range []string{"Python.framework", "PythonT.framework"} {
		base := filepath.Join("/Library/Frameworks", fw, "Versions")
		for _, v := range readDirNames(base) {
			if v == "Current" {
				continue
			}
			p := filepath.Join(base, v)
			version := v
			if raw, err := os.ReadFile(filepath.Join(p, "Resources", "Info.plist")); err == nil {
				var info map[string]any
				if _, err := plist.Unmarshal(raw, &info); err == nil {
					if s, ok := info["CFBundleShortVersionString"].(string); ok {
						version = s
					}
				}
			}
			name := "Python"
			if fw == "PythonT.framework" {
				name = "Python (free-threaded)"
			}
			out = append(out, RuntimeRaw{Name: name, Version: version, Source: SrcPythonOrg, Manager: "python.org installer", Path: p, InstalledAt: birthTime(p)})
		}
	}
	if clt := "/Library/Developer/CommandLineTools"; isDir(clt) {
		version := ""
		if raw, err := run(5*time.Second, "/usr/sbin/pkgutil", "--pkg-info=com.apple.pkg.CLTools_Executables"); err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				if v, ok := strings.CutPrefix(line, "version: "); ok {
					parts := strings.Split(v, ".")
					version = strings.Join(parts[:min(2, len(parts))], ".")
				}
			}
		}
		out = append(out, RuntimeRaw{Name: "Command Line Tools", Version: version, Source: SrcSystem, Manager: "xcode-select --install", Path: clt, InstalledAt: birthTime(clt)})
	}
	return out, nil
}
