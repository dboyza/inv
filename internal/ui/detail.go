package ui

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/dboyza/inv/internal/inv"
)

// detailView renders the selected item's full story in a bordered panel.
func (m Model) detailView(w, h int) string {
	it := m.selected()
	acc := kindAccent[m.kind()]
	inner := w - 4 // one space of padding inside each border
	var lines []string
	if it == nil {
		lines = []string{"", lipgloss.PlaceHorizontal(inner, lipgloss.Center, sMuted.Render("Select something to see its details"))}
	} else {
		lines = m.detailLines(it, inner)
	}
	maxScroll := max(0, len(lines)-(h-2))
	scroll := min(m.dScroll, maxScroll)
	lines = lines[scroll:]
	for i := range lines {
		lines[i] = " " + lines[i]
	}
	more := ""
	if scroll < maxScroll {
		more = sMuted.Render("J ↓ more")
	} else if scroll > 0 {
		more = sMuted.Render("K ↑")
	}
	hint := sMuted.Render("o open · f reveal · w web · c copy · i install")
	if w < 60 {
		hint = sMuted.Render("? for actions")
	}
	return box(w, h, fg(acc).Bold(true).Render("Details"), more, hint, "", lines, cOverlay)
}

func (m Model) detailLines(it *inv.Item, w int) []string {
	acc := kindAccent[it.Kind]
	var out []string
	add := func(s ...string) { out = append(out, s...) }

	// Title block: name, version pill, description.
	title := sBright.Render(it.Name)
	if it.Updatable() {
		title += "  " + lipgloss.NewStyle().Foreground(cBase).Background(cFoam).Bold(true).Render(" update ")
	}
	if it.Declared == inv.DeclMissing {
		title += "  " + lipgloss.NewStyle().Foreground(cBase).Background(cLove).Bold(true).Render(" missing ")
	}
	eyebrow := fg(acc).Bold(true).Render(kindIcon[it.Kind]+" "+kindSingular[it.Kind]) + sMuted.Render("  ·  "+it.Category)
	add("", eyebrow, title)
	if it.Desc != "" {
		add(wrap(sSubtle, it.Desc, w)...)
	}
	add("")

	kv := func(label, value string) {
		if value == "" {
			return
		}
		add(sLabel.Render(label) + ansi.Truncate(value, w-11, "…"))
	}

	// Section: Facts
	version := sText.Render(firstNonEmpty(it.Version, "unknown"))
	if it.Updatable() {
		version += sUpdate.Render("  → " + it.Latest + " available")
	}
	if it.Kind != inv.Service {
		kv("Version", version)
	}
	src := fg(sourceColor[it.Source]).Render("● " + firstNonEmpty(sourceLabel[it.Source], string(it.Source)))
	if it.SourceRef != "" && it.Source != inv.SrcBundled {
		src += sMuted.Render("  " + it.SourceRef)
	}
	kv("Source", src)
	if it.Path != "" {
		p := m.env.Tilde(it.Path)
		kv("Location", sText.Hyperlink("file://"+it.Path).Render(p))
	}
	if it.BundleID != "" {
		kv("Bundle ID", sMuted.Render(it.BundleID))
	}
	if !it.InstalledAt.IsZero() {
		kv("Installed", sText.Render(ago(it.InstalledAt))+sMuted.Render("  "+it.InstalledAt.Format("Jan 2, 2006")))
	}
	if !it.LastUsed.IsZero() {
		kv("Last used", sText.Render(ago(it.LastUsed))+sMuted.Render("  "+it.LastUsed.Format("Jan 2, 2006")))
	} else if it.Kind == inv.Desktop && it.Declared != inv.DeclMissing {
		kv("Last used", sMuted.Render("not recorded by Spotlight"))
	}
	if it.Kind == inv.Terminal && it.Declared != inv.DeclMissing {
		if it.ShellRuns > 0 {
			kv("Shell use", sText.Render(fmt.Sprintf("%d runs", it.ShellRuns))+sMuted.Render(" in zsh history"))
		} else {
			kv("Shell use", sMuted.Render("not in zsh history"))
		}
	}
	if it.Kind == inv.Service {
		if it.Running {
			kv("Status", sGood.Render("● running"))
		} else {
			kv("Status", sMuted.Render("○ not running"))
		}
	}
	kv("Dotfiles", dotfilesLine(it))
	if it.Homepage != "" {
		kv("Homepage", fg(cPine).Underline(true).Hyperlink(it.Homepage).Render(shortURL(it.Homepage)))
	}

	// Section: Relationships and contents
	section := func(title, hint string, body []string) {
		if len(body) == 0 {
			return
		}
		head := sSection.Render(strings.ToUpper(title))
		if hint != "" {
			head += "  " + sMuted.Render(hint)
		}
		add("", head)
		add(body...)
	}
	if len(it.Provides) > 0 {
		var names []string
		for _, id := range it.Provides {
			if p := m.byID[id]; p != nil {
				names = append(names, p.Name)
			}
		}
		section("Provides commands", "↵ jump", chips(names, w, cFoam))
	}
	if app := m.byID[it.ProvidedBy]; app != nil {
		label := "Ships with"
		if it.Kind == inv.Service {
			label = "Belongs to"
		}
		section(label, "↵ jump", []string{fg(kindAccent[app.Kind]).Render("◫ " + app.Name)})
	}
	if it.Kind == inv.Service {
		section("Runs", "", wrapPath(strings.Join(it.Commands, " "), w))
	} else if it.Kind == inv.Font {
		section("Families", fmt.Sprint(len(it.Commands)), chips(it.Commands, w, cText))
	} else if len(it.Commands) > 0 && !(len(it.Commands) == 1 && it.Commands[0] == it.Name) {
		section("Commands", fmt.Sprint(len(it.Commands)), chips(it.Commands, w, cText))
	}
	section("Styles", fmt.Sprint(len(it.Members)), chips(it.Members, w, cText))
	section("Depends on", fmt.Sprint(len(it.Deps)), chips(it.Deps, w, cSubtle))
	var notes []string
	for _, n := range it.Notes {
		for i, l := range wrap(sText, n, w-2) {
			prefix := sWarn.Render("! ")
			if i > 0 {
				prefix = "  "
			}
			notes = append(notes, prefix+l)
		}
	}
	section("Notes", "", notes)
	if cmd := installCommand(it); cmd != "" {
		section("Reinstall", "i copies", []string{sMuted.Render("$ ") + sText.Render(cmd)})
	}
	return out
}

var kindSingular = map[inv.Kind]string{
	inv.Desktop: "Desktop app", inv.Terminal: "Terminal tool", inv.Runtime: "Language runtime",
	inv.Font: "Font", inv.Extension: "Editor extension", inv.Service: "Background service",
}

func dotfilesLine(it *inv.Item) string {
	switch it.Declared {
	case inv.DeclTracked:
		return fg(cPine).Render("◆ declared") + sMuted.Render(" in "+it.DeclaredIn)
	case inv.DeclUntracked:
		return sWarn.Render("◇ not declared") + sMuted.Render(" · installed by hand")
	case inv.DeclMissing:
		return sErr.Render("✗ not installed") + sMuted.Render(" · declared in "+it.DeclaredIn)
	}
	return ""
}

// chips flows names into lines separated by a dim dot.
func chips(names []string, w int, c color.Color) []string {
	if len(names) == 0 {
		return nil
	}
	st := fg(c)
	sep := sMuted.Render(" · ")
	var lines []string
	cur, curW := "", 0
	for _, n := range names {
		n = filepath.Base(n)
		nw := lipgloss.Width(n)
		if curW > 0 && curW+3+nw > w {
			lines = append(lines, cur)
			cur, curW = "", 0
		}
		if curW > 0 {
			cur += sep
			curW += 3
		}
		cur += st.Render(n)
		curW += nw
	}
	return append(lines, cur)
}

// wrapPath breaks long paths at slashes so they stay readable.
func wrapPath(p string, w int) []string {
	var out []string
	for _, l := range strings.Split(ansi.Wrap(p, max(10, w), "/"), "\n") {
		out = append(out, sText.Render(l))
	}
	return out
}

// wrap word-wraps plain text to width and styles each line.
func wrap(st lipgloss.Style, s string, w int) []string {
	var out []string
	for _, l := range strings.Split(ansi.Wordwrap(s, max(10, w), " "), "\n") {
		out = append(out, st.Render(l))
	}
	return out
}
