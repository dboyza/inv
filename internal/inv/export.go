package inv

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ExportUntracked renders untracked installs as snippets for the dotfiles inventory files.
// Items no package manager can reproduce are listed as comments so nothing is silently dropped.
func ExportUntracked(items []Item, host string, now time.Time) string {
	var brews, fontCasks, apps, manual, npm, uv, vscode []string
	for _, it := range items {
		if it.Declared != DeclUntracked || it.Dependency || it.Source == SrcBundled || it.Kind == Service {
			continue
		}
		switch {
		case it.Source == SrcBrew:
			brews = append(brews, fmt.Sprintf("    %q", it.SourceRef))
		case it.Source == SrcCask && it.Kind == Font:
			fontCasks = append(fontCasks, fmt.Sprintf("    %q", it.SourceRef))
		case it.Source == SrcCask && it.Kind == Desktop:
			apps = append(apps, fmt.Sprintf(`  { "bundle": %q, "id": %q, "cask": %q },`, bundleName(it), it.BundleID, it.SourceRef))
		case it.Source == SrcAppStore && it.SourceRef != "":
			apps = append(apps, fmt.Sprintf(`  { "bundle": %q, "id": %q, "app_store_id": %s },`, bundleName(it), it.BundleID, it.SourceRef))
		case it.Source == SrcNpm:
			npm = append(npm, it.Name)
		case it.Source == SrcUv:
			uv = append(uv, minorVersion(it.Version))
		case it.Source == SrcVSCode:
			vscode = append(vscode, it.SourceRef)
		default:
			where := it.Path
			if it.Kind == Desktop {
				where = it.BundleID
			}
			manual = append(manual, fmt.Sprintf("#   %s  (%s, %s)", it.Name, sourceText(it), where))
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Untracked installs on %s, exported by inv on %s.\n", host, now.Format("2006-01-02"))
	section := func(title string, lines []string) {
		if len(lines) == 0 {
			return
		}
		sort.Strings(lines)
		fmt.Fprintf(&b, "\n# %s\n%s\n", title, strings.Join(lines, "\n"))
	}
	section("nix/homebrew.nix: add to brews", brews)
	section("nix/homebrew.nix: add to casks", fontCasks)
	section("nix/macos-apps.json: add to the list", apps)
	if len(npm) > 0 {
		sort.Strings(npm)
		section("npm globals", []string{"npm install -g " + strings.Join(npm, " ")})
	}
	if len(uv) > 0 {
		section("uv runtimes", []string{"uv python install " + strings.Join(uv, " ")})
	}
	if len(vscode) > 0 {
		sort.Strings(vscode)
		lines := make([]string, len(vscode))
		for i, id := range vscode {
			lines[i] = "code --install-extension " + id
		}
		section("VS Code extensions", lines)
	}
	section("No package manager can reinstall these; keep them by hand or find a cask", manual)
	return b.String()
}

func bundleName(it Item) string {
	if i := strings.LastIndex(it.Path, "/"); i >= 0 {
		return it.Path[i+1:]
	}
	return it.Name + ".app"
}

func sourceText(it Item) string {
	if it.SourceRef != "" && it.Source != SrcDirect {
		return string(it.Source) + ": " + it.SourceRef
	}
	return string(it.Source)
}
