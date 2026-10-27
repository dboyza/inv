package inv

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"howett.net/plist"
)

// Section: Fonts

// FontFile is one user-installed font file.
type FontFile struct {
	File        string    `json:"file"`
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	InstalledAt time.Time `json:"installed_at"`
}

// appleFonts ship with macOS even though they live in /Library/Fonts.
var appleFonts = map[string]bool{"Arial Unicode.ttf": true}

func (e *Env) FontDirs() []string {
	return []string{filepath.Join(e.Home, "Library", "Fonts"), "/Library/Fonts"}
}

func collectFonts(env *Env) ([]FontFile, error) {
	var out []FontFile
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			p := filepath.Join(dir, e.Name())
			if e.IsDir() && depth == 0 {
				walk(p, 1)
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if (ext == ".ttf" || ext == ".otf" || ext == ".ttc" || ext == ".otc") && !appleFonts[e.Name()] {
				info, _ := e.Info()
				out = append(out, FontFile{File: e.Name(), Path: p, Size: sizeOf(info), InstalledAt: birthTime(p)})
			}
		}
	}
	for _, dir := range env.FontDirs() {
		walk(dir, 0)
	}
	return out, nil
}

func sizeOf(fi os.FileInfo) int64 {
	if fi == nil {
		return 0
	}
	return fi.Size()
}

// Section: Editor extensions

// ExtRaw is one editor extension or plugin.
type ExtRaw struct {
	Editor      string    `json:"editor"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Desc        string    `json:"desc"`
	Publisher   string    `json:"publisher"`
	Homepage    string    `json:"homepage"`
	Path        string    `json:"path"`
	InstalledAt time.Time `json:"installed_at"`
}

// vscodeFamilies maps extension roots to the editor that owns them.
var vscodeFamilies = []struct{ dir, editor string }{
	{".vscode/extensions", "VS Code"},
	{".cursor/extensions", "Cursor"},
	{".windsurf/extensions", "Windsurf"},
	{".vscode-oss/extensions", "VSCodium"},
}

func (e *Env) ExtensionDirs() []string {
	dirs := []string{filepath.Join(e.Home, ".config", "nvim"), filepath.Join(e.Home, ".local", "share", "nvim", "lazy")}
	for _, f := range vscodeFamilies {
		dirs = append(dirs, filepath.Join(e.Home, f.dir))
	}
	return dirs
}

func collectExtensions(env *Env) ([]ExtRaw, error) {
	var out []ExtRaw
	for _, fam := range vscodeFamilies {
		root := filepath.Join(env.Home, fam.dir)
		raw, err := os.ReadFile(filepath.Join(root, "extensions.json"))
		if err != nil {
			continue
		}
		var list []struct {
			Identifier       struct{ ID string } `json:"identifier"`
			Version          string              `json:"version"`
			RelativeLocation string              `json:"relativeLocation"`
			Metadata         struct {
				InstalledTimestamp int64 `json:"installedTimestamp"`
			} `json:"metadata"`
		}
		if json.Unmarshal(raw, &list) != nil {
			continue
		}
		for _, x := range list {
			ext := ExtRaw{Editor: fam.editor, ID: x.Identifier.ID, Name: x.Identifier.ID, Version: x.Version,
				Path: filepath.Join(root, x.RelativeLocation)}
			if x.Metadata.InstalledTimestamp > 0 {
				ext.InstalledAt = time.UnixMilli(x.Metadata.InstalledTimestamp)
			}
			if pj, err := os.ReadFile(filepath.Join(ext.Path, "package.json")); err == nil {
				var meta struct {
					DisplayName string `json:"displayName"`
					Description string `json:"description"`
					Publisher   string `json:"publisher"`
					Homepage    string `json:"homepage"`
				}
				if json.Unmarshal(pj, &meta) == nil {
					meta.DisplayName = localizeManifest(ext.Path, meta.DisplayName)
					meta.Description = localizeManifest(ext.Path, meta.Description)
					// Localized manifests use %placeholders%; keep the identifier then.
					if meta.DisplayName != "" && !strings.HasPrefix(meta.DisplayName, "%") {
						ext.Name = meta.DisplayName
					}
					if !strings.HasPrefix(meta.Description, "%") {
						ext.Desc = meta.Description
					}
					ext.Publisher, ext.Homepage = meta.Publisher, meta.Homepage
				}
			}
			if ext.Homepage == "" {
				ext.Homepage = "https://marketplace.visualstudio.com/items?itemName=" + x.Identifier.ID
			}
			out = append(out, ext)
		}
	}
	out = append(out, collectLazy(env)...)
	return out, nil
}

// localizeManifest resolves VS Code's "%key%" manifest strings from package.nls.json.
func localizeManifest(dir, s string) string {
	if !strings.HasPrefix(s, "%") || !strings.HasSuffix(s, "%") {
		return s
	}
	raw, err := os.ReadFile(filepath.Join(dir, "package.nls.json"))
	if err != nil {
		return s
	}
	var nls map[string]any
	if json.Unmarshal(raw, &nls) != nil {
		return s
	}
	switch v := nls[strings.Trim(s, "%")].(type) {
	case string:
		return v
	case map[string]any: // {"message": "...", "comment": [...]}
		if m, ok := v["message"].(string); ok {
			return m
		}
	}
	return s
}

var gitURLRe = regexp.MustCompile(`(?m)^\s*url\s*=\s*(\S+)`)

// collectLazy reads Neovim plugins pinned in lazy-lock.json.
func collectLazy(env *Env) []ExtRaw {
	raw, err := os.ReadFile(filepath.Join(env.Home, ".config", "nvim", "lazy-lock.json"))
	if err != nil {
		return nil
	}
	var lock map[string]struct{ Branch, Commit string }
	if json.Unmarshal(raw, &lock) != nil {
		return nil
	}
	var out []ExtRaw
	for name, pin := range lock {
		dir := filepath.Join(env.Home, ".local", "share", "nvim", "lazy", name)
		ext := ExtRaw{Editor: "Neovim", ID: name, Name: name, Version: shortCommit(pin.Commit), Path: dir, InstalledAt: birthTime(dir)}
		if cfg, err := os.ReadFile(filepath.Join(dir, ".git", "config")); err == nil {
			if m := gitURLRe.FindSubmatch(cfg); m != nil {
				ext.Homepage = strings.TrimSuffix(string(m[1]), ".git")
				if parts := strings.Split(ext.Homepage, "/"); len(parts) >= 2 {
					ext.Publisher = parts[len(parts)-2]
				}
			}
		}
		out = append(out, ext)
	}
	return out
}

func shortCommit(c string) string {
	if len(c) > 7 {
		return c[:7]
	}
	return c
}

// Section: Background services

// ServiceRaw is one third-party launchd job.
type ServiceRaw struct {
	Label       string    `json:"label"`
	Path        string    `json:"path"`
	Program     string    `json:"program"`
	Scope       string    `json:"scope"` // "user agent", "global agent", "daemon"
	RunAtLoad   bool      `json:"run_at_load"`
	KeepAlive   bool      `json:"keep_alive"`
	Disabled    bool      `json:"disabled"`
	Running     bool      `json:"running"`
	InstalledAt time.Time `json:"installed_at"`
}

func (e *Env) ServiceDirs() map[string]string {
	return map[string]string{
		filepath.Join(e.Home, "Library", "LaunchAgents"): "user agent",
		"/Library/LaunchAgents":                          "global agent",
		"/Library/LaunchDaemons":                         "daemon",
	}
}

func collectServices(env *Env) ([]ServiceRaw, error) {
	running := map[string]bool{}
	// `launchctl print` lists "pid status label" rows per domain and needs no root for reading.
	for _, domain := range []string{"system", fmt.Sprintf("gui/%d", os.Getuid())} {
		out, err := run(5*time.Second, "/bin/launchctl", "print", domain)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			if f := strings.Fields(line); len(f) == 3 && f[0] != "0" && isDigits(f[0]) {
				running[f[2]] = true
			}
		}
	}
	var out []ServiceRaw
	for dir, scope := range env.ServiceDirs() {
		for _, name := range readDirNames(dir) {
			if !strings.HasSuffix(name, ".plist") || strings.HasPrefix(name, "com.apple.") {
				continue
			}
			p := filepath.Join(dir, name)
			raw, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			var job struct {
				Label            string   `plist:"Label"`
				Program          string   `plist:"Program"`
				ProgramArguments []string `plist:"ProgramArguments"`
				RunAtLoad        bool     `plist:"RunAtLoad"`
				KeepAlive        any      `plist:"KeepAlive"`
				Disabled         bool     `plist:"Disabled"`
			}
			if _, err := plist.Unmarshal(raw, &job); err != nil || job.Label == "" {
				continue
			}
			s := ServiceRaw{Label: job.Label, Path: p, Program: job.Program, Scope: scope, RunAtLoad: job.RunAtLoad,
				Disabled: job.Disabled, Running: running[job.Label], InstalledAt: birthTime(p)}
			if s.Program == "" && len(job.ProgramArguments) > 0 {
				s.Program = job.ProgramArguments[0]
			}
			if b, ok := job.KeepAlive.(bool); ok {
				s.KeepAlive = b
			} else {
				s.KeepAlive = job.KeepAlive != nil
			}
			out = append(out, s)
		}
	}
	return out, nil
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// Section: Shell history

// collectHistory counts how often each command starts a pipeline segment in zsh history.
func collectHistory(env *Env) (map[string]int, error) {
	path := os.Getenv("HISTFILE")
	if path == "" {
		path = filepath.Join(env.Home, ".zsh_history")
	}
	f, err := os.Open(path)
	if err != nil {
		return map[string]int{}, nil
	}
	defer f.Close()
	counts := map[string]int{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, ": ") { // EXTENDED_HISTORY ": <ts>:<dur>;cmd"
			if i := strings.IndexByte(line, ';'); i > 0 {
				line = line[i+1:]
			}
		}
		for _, seg := range splitCommands(line) {
			if cmd := commandWord(seg); cmd != "" {
				counts[cmd]++
			}
		}
	}
	return counts, nil
}

var segmentSplitRe = regexp.MustCompile(`\|\||&&|[|;]|\$\(`)

func splitCommands(line string) []string { return segmentSplitRe.Split(line, -1) }

// commandWord skips env assignments and wrappers like sudo to find the program name.
func commandWord(seg string) string {
	for _, w := range strings.Fields(seg) {
		switch {
		case strings.Contains(w, "=") && !strings.HasPrefix(w, "="):
			continue
		case w == "sudo" || w == "time" || w == "exec" || w == "command" || w == "nohup" || w == "env" || w == "builtin":
			continue
		}
		return filepath.Base(strings.Trim(w, "()`'\""))
	}
	return ""
}
