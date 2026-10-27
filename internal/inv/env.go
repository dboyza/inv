package inv

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Env holds machine paths shared by every collector.
type Env struct {
	Home       string
	BrewPrefix string // empty when Homebrew is absent
	Dotfiles   string // empty when no dotfiles repo was found
}

// NewEnv resolves the home directory, Homebrew prefix, and dotfiles checkout.
func NewEnv() *Env {
	home, _ := os.UserHomeDir()
	env := &Env{Home: home}
	for _, p := range []string{"/opt/homebrew", "/usr/local"} {
		if isFile(filepath.Join(p, "bin", "brew")) {
			env.BrewPrefix = p
			break
		}
	}
	env.Dotfiles = findDotfiles(home)
	return env
}

// findDotfiles prefers $INV_DOTFILES, then follows Home Manager's live ~/.zshrc link back to its repo.
func findDotfiles(home string) string {
	if d := os.Getenv("INV_DOTFILES"); d != "" {
		return d
	}
	candidates := []string{}
	for _, link := range []string{".zshrc", ".local/bin/dotfiles-tool.mjs", ".tmux.conf"} {
		if real, err := filepath.EvalSymlinks(filepath.Join(home, link)); err == nil {
			candidates = append(candidates, filepath.Dir(real))
		}
	}
	candidates = append(candidates, filepath.Join(home, "code", "dotfiles"), filepath.Join(home, "dotfiles"), filepath.Join(home, ".dotfiles"))
	for _, dir := range candidates {
		for d := dir; d != "/" && d != "."; d = filepath.Dir(d) {
			if isFile(filepath.Join(d, "flake.nix")) && isDir(filepath.Join(d, "nix")) {
				return d
			}
		}
	}
	return ""
}

// Tilde shortens a path under the home directory for display.
func (e *Env) Tilde(p string) string {
	if e.Home != "" && strings.HasPrefix(p, e.Home+"/") {
		return "~" + p[len(e.Home):]
	}
	return p
}

// Section: Small filesystem and process helpers

func isFile(p string) bool { st, err := os.Stat(p); return err == nil && !st.IsDir() }
func isDir(p string) bool  { st, err := os.Stat(p); return err == nil && st.IsDir() }

// run executes a command with a timeout and returns stdout; stderr is discarded.
func run(timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ENV_HINTS=1", "NO_COLOR=1")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.Bytes(), err
}

// readDirNames lists entry names in a directory, ignoring errors.
func readDirNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}
