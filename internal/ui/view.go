package ui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/dboyza/inv/internal/inv"
)

// Section: Layout

type layout struct {
	tabsY, bodyY, bodyH int
	listW, detailW      int
}

// layout splits the screen: header, tabs and their rule, body panels, footer.
func (m Model) layout() layout {
	l := layout{tabsY: 2, bodyY: 4, bodyH: max(6, m.height-5)}
	if m.width >= narrowBreakpt {
		l.listW = min(max(m.width*46/100, 48), 74)
		l.detailW = m.width - l.listW
	} else {
		l.listW = m.width
	}
	return l
}

func (m Model) listHeight() int { return max(1, m.layout().bodyH-2) }

// Section: Root view

func (m Model) View() tea.View {
	lay := m.layout()
	var b strings.Builder
	b.WriteString(m.headerView() + "\n\n")
	b.WriteString(m.tabsView() + "\n")
	b.WriteString(m.tabRuleView() + "\n")
	switch {
	case lay.detailW > 0:
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, m.listView(lay.listW, lay.bodyH), m.detailView(lay.detailW, lay.bodyH)))
	case m.detailOpen:
		b.WriteString(m.detailView(lay.listW, lay.bodyH))
	default:
		b.WriteString(m.listView(lay.listW, lay.bodyH))
	}
	b.WriteString("\n" + m.footerView())

	content := b.String()
	if len(m.toasts) > 0 || m.showHelp || m.confirm != nil || m.exportOpen {
		layers := []*lipgloss.Layer{lipgloss.NewLayer(content)}
		if t := m.toastView(); t != "" {
			layers = append(layers, lipgloss.NewLayer(t).X(max(0, m.width-lipgloss.Width(t)-2)).Y(max(0, m.height-2-lipgloss.Height(t))).Z(1))
		}
		if m.exportOpen {
			e := m.exportView()
			layers = append(layers, lipgloss.NewLayer(e).X(max(0, (m.width-lipgloss.Width(e))/2)).Y(max(0, (m.height-lipgloss.Height(e))/2)).Z(2))
		}
		if m.confirm != nil {
			box, _, _ := m.confirmLayout()
			x, y := m.confirmOrigin(box)
			layers = append(layers, lipgloss.NewLayer(box).X(x).Y(y).Z(3))
		}
		if m.showHelp {
			h := m.helpView()
			layers = append(layers, lipgloss.NewLayer(h).X(max(0, (m.width-lipgloss.Width(h))/2)).Y(max(0, (m.height-lipgloss.Height(h))/2)).Z(2))
		}
		content = clamp(lipgloss.NewCompositor(layers...).Render(), m.width, m.height)
	}

	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = fmt.Sprintf("inv · %d installs", m.visibleTotal())
	return v
}

func (m Model) visibleTotal() int {
	n := 0
	for _, it := range m.items {
		if !it.Dependency && it.Declared != inv.DeclMissing {
			n++
		}
	}
	return n
}

// Section: Header and tabs

func (m Model) headerView() string {
	logo := lipgloss.NewStyle().Foreground(cBase).Background(cIris).Bold(true).Render(" ◆ inv ")
	left := logo + "  " +
		sSubtle.Render("every custom install on ") + sText.Bold(true).Render(m.host)

	var status string
	switch {
	case m.scans > 0 && !m.hasData:
		status = m.spin() + sSubtle.Render(" scanning your Mac")
	case m.scans > 0:
		status = m.spin() + sSubtle.Render(" rescanning")
	case m.checking:
		status = m.spin() + sSubtle.Render(" checking for updates")
	case m.watcher != nil:
		status = sGood.Render("●") + sSubtle.Render(" live")
	default:
		status = sMuted.Render("○ ") + sMuted.Render(firstNonEmpty(m.watchErr, "starting watcher"))
	}
	if !m.snap.ScannedAt.IsZero() && m.scans == 0 {
		status += sMuted.Render(" · scanned " + ago(m.snap.ScannedAt))
	}
	return spread(" "+left, status+" ", m.width)
}

func (m Model) spin() string {
	frames := []rune(spinnerFrames)
	return fg(kindAccent[m.kind()]).Render(string(frames[m.frame%len(frames)]))
}

// tabLabels renders each tab, dropping icons and then names until the row fits.
// tabSpans uses the same labels for mouse hit-testing.
func (m Model) tabLabels() []string {
	counts := make([]string, len(inv.Kinds))
	for i, k := range inv.Kinds {
		counts[i] = fmt.Sprint(m.tabCount(k))
	}
	for style := 0; ; style++ {
		labels := make([]string, len(inv.Kinds))
		total := 2
		for i, k := range inv.Kinds {
			text, count := kindIcon[k]+" "+k.String(), " "+counts[i]+" "
			switch style {
			case 1:
				text, count = strings.Replace(k.String(), "Extensions", "Exts", 1), counts[i]
			case 2:
				text, count = kindIcon[k], counts[i]
			}
			if i == m.tab {
				labels[i] = fg(kindAccent[k]).Bold(true).Render(text) + " " +
					lipgloss.NewStyle().Foreground(cBase).Background(kindAccent[k]).Bold(true).Render(" "+counts[i]+" ")
			} else {
				labels[i] = sSubtle.Render(text) + " " + sMuted.Render(count)
			}
			total += lipgloss.Width(labels[i]) + tabGap
		}
		if total-tabGap <= m.width || style == 2 {
			return labels
		}
	}
}

const tabGap = 3

func (m Model) tabSpans() [][2]int {
	spans := [][2]int{}
	x := 2
	for _, l := range m.tabLabels() {
		w := lipgloss.Width(l)
		spans = append(spans, [2]int{x - 1, x + w + 1})
		x += w + tabGap
	}
	return spans
}

func (m Model) tabsView() string {
	return "  " + strings.Join(m.tabLabels(), strings.Repeat(" ", tabGap))
}

// tabRuleView draws a heavy accent rule under the active tab and a hairline elsewhere.
func (m Model) tabRuleView() string {
	spans := m.tabSpans()
	active := spans[m.tab]
	start, end := active[0]+1, active[1]-1
	acc := fg(kindAccent[m.kind()])
	return sBorder.Render(strings.Repeat("─", max(0, start))) + acc.Render(strings.Repeat("━", max(0, end-start))) +
		sBorder.Render(strings.Repeat("─", max(0, m.width-end)))
}

// Section: List panel

func (m Model) listView(w, h int) string {
	k := m.kind()
	acc := kindAccent[k]
	rows := m.rows()
	lh := h - 2
	inner := w - 2
	sel := m.selectedIndex(rows)
	off := m.offset[m.tab]

	lines := make([]string, 0, lh)
	for i := off; i < len(rows) && len(lines) < lh; i++ {
		if rows[i].item == nil {
			lines = append(lines, m.groupHeader(rows[i], inner, acc))
		} else {
			lines = append(lines, m.itemRow(rows[i], inner, i == sel, acc))
		}
	}
	if len(rows) == 0 {
		lines = append(lines, "", m.emptyMessage(inner))
	}

	title := fg(acc).Bold(true).Render(kindTitle[k])
	if m.searching || m.search.Value() != "" {
		// Size the field to its text so the border rule continues right after it.
		m.search.SetWidth(min(max(lipgloss.Width(m.search.Value())+1, 22), inner-24))
		title = fg(acc).Bold(true).Render("/ ") + m.searchView()
	}
	var chips []string
	if m.untracked {
		chips = append(chips, sWarn.Render("◇ untracked"))
	}
	if m.updatesOnly {
		chips = append(chips, sUpdate.Render("↑ updates"))
	}
	if m.showDeps {
		chips = append(chips, sSubtle.Render("+ deps"))
	}
	var modes []string
	if m.group != groupPurpose {
		modes = append(modes, "by "+m.group.String())
	}
	if m.sort != sortName {
		modes = append(modes, m.sort.String()+" first")
	}
	pos := ""
	if n := countItems(rows); n > 0 {
		pos = fmt.Sprintf("%d of %d", itemOrdinal(rows, sel), n)
	}
	return box(w, h, title, strings.Join(chips, sMuted.Render(" · ")), sMuted.Render(strings.Join(modes, " · ")), sMuted.Render(pos), lines, cOverlay)
}

var kindTitle = map[inv.Kind]string{
	inv.Desktop: "Desktop apps", inv.Terminal: "Terminal tools", inv.Runtime: "Language runtimes",
	inv.Font: "Fonts", inv.Extension: "Editor extensions", inv.Service: "Background services",
}

func (m Model) searchView() string {
	if m.searching {
		return m.search.View()
	}
	return sText.Render(m.search.Value())
}

func (m Model) emptyMessage(w int) string {
	var msg string
	switch {
	case !m.hasData:
		msg = m.spin() + sSubtle.Render(" Looking through your Mac…")
	case m.search.Value() != "":
		msg = sSubtle.Render("Nothing here matches “" + m.search.Value() + "”")
	case m.untracked || m.updatesOnly:
		msg = sSubtle.Render("Nothing matches these filters · esc clears")
	default:
		msg = sSubtle.Render("Nothing installed here")
	}
	return lipgloss.PlaceHorizontal(w, lipgloss.Center, msg)
}

func (m Model) groupHeader(r row, w int, acc color.Color) string {
	label := " " + fg(acc).Render(r.title) + " " + sMuted.Render(fmt.Sprint(r.count)) + " "
	return label + sBorder.Render(strings.Repeat("─", max(0, w-lipgloss.Width(label)-1))) + " "
}

// itemRow lays out: selection bar, drift marker, name, version, source badge.
func (m Model) itemRow(r row, w int, selected bool, acc color.Color) string {
	it := r.item
	bg := func(s lipgloss.Style) lipgloss.Style {
		if selected {
			return s.Background(cSurface)
		}
		return s
	}
	bar := bg(lipgloss.NewStyle()).Render(" ")
	if selected {
		bar = bg(fg(acc)).Render("▌")
	}
	marker := bg(lipgloss.NewStyle()).Render(" ")
	switch it.Declared {
	case inv.DeclTracked:
		marker = bg(fg(cHighlight)).Render("◆")
	case inv.DeclUntracked:
		marker = bg(sWarn).Render("◇")
	case inv.DeclMissing:
		marker = bg(sErr).Render("✗")
	}

	badgeText, badgeColor := badge(it, m.byID)
	badgeW := 12
	if w >= 60 {
		badgeW = 15
	}
	badgeCell := bg(fg(badgeColor)).Render(padLeft(ansi.Truncate(badgeText, badgeW, "…"), badgeW))

	version := it.Version
	verStyle := bg(sMuted)
	if it.Updatable() {
		version += " ↑"
		verStyle = bg(sUpdate)
	}
	version = ansi.Truncate(version, 16, "…")

	nameStyle := bg(sText)
	switch {
	case selected:
		nameStyle = bg(sBright)
	case it.Declared == inv.DeclMissing:
		nameStyle = bg(sErr)
	case it.Apple || it.Dependency:
		nameStyle = bg(sSubtle)
	}
	_, isNew := m.flash[it.ID]
	newPill := ""
	if isNew {
		newPill = " " + lipgloss.NewStyle().Foreground(cBase).Background(cFoam).Bold(true).Render(" new ")
	}
	nameW := w - 1 - 2 - 1 - lipgloss.Width(version) - 1 - badgeW - 1 - lipgloss.Width(newPill) - 1
	name := highlight(ansi.Truncate(it.Name, max(4, nameW), "…"), r.matches, nameStyle, bg(sWarn.Underline(true)))
	gap := max(1, nameW-lipgloss.Width(name)+1)

	return bar + marker + bg(lipgloss.NewStyle()).Render(" ") + name + newPill + bg(lipgloss.NewStyle()).Render(strings.Repeat(" ", gap)) +
		verStyle.Render(version) + bg(lipgloss.NewStyle()).Render(" ") + badgeCell + bg(lipgloss.NewStyle()).Render(" ")
}

// badge is the short source tag; bundled CLIs name their app instead.
func badge(it *inv.Item, byID map[string]*inv.Item) (string, color.Color) {
	c := sourceColor[it.Source]
	if c == nil {
		c = cSubtle
	}
	switch it.Source {
	case inv.SrcBundled:
		if app := byID[it.ProvidedBy]; app != nil {
			return "via " + app.Name, c
		}
	case inv.SrcSystem:
		if it.Kind == inv.Service {
			return it.SourceRef, c
		}
	}
	return string(it.Source), c
}

// highlight styles fuzzy-matched runes in s.
func highlight(s string, idx []int, base, hit lipgloss.Style) string {
	if len(idx) == 0 {
		return base.Render(s)
	}
	set := map[int]bool{}
	for _, i := range idx {
		set[i] = true
	}
	var b strings.Builder
	for i, r := range []rune(s) {
		if set[i] {
			b.WriteString(hit.Render(string(r)))
		} else {
			b.WriteString(base.Render(string(r)))
		}
	}
	return b.String()
}

func countItems(rows []row) int {
	n := 0
	for _, r := range rows {
		if r.item != nil {
			n++
		}
	}
	return n
}

func itemOrdinal(rows []row, idx int) int {
	n := 0
	for i := 0; i <= idx && i < len(rows); i++ {
		if rows[i].item != nil {
			n++
		}
	}
	return n
}

// Section: Footer, toasts, help

func (m Model) footerView() string {
	enter := "jump"
	if m.width < narrowBreakpt && !m.detailOpen {
		enter = "details"
	}
	hints := []string{"/", "search", "←→", "tabs", "↵", enter, "b", "group", "s", "sort", "t", "untracked", "u", "updates",
		"U", "upgrade", "X", "uninstall", "d", "deps", "E", "export", "o", "open", "?", "help", "q", "quit"}
	var untracked, updates int
	for _, it := range m.items {
		if it.Dependency {
			continue
		}
		if it.Declared == inv.DeclUntracked {
			untracked++
		}
		if it.Updatable() {
			updates++
		}
	}
	right := ""
	if updates > 0 {
		right += sUpdate.Render(fmt.Sprintf("↑ %d", updates)) + sMuted.Render(" updates  ")
	}
	if untracked > 0 {
		right += sWarn.Render(fmt.Sprintf("◇ %d", untracked)) + sMuted.Render(" untracked ")
	}
	budget := m.width - lipgloss.Width(right) - 2
	left := ""
	for i := 0; i+1 < len(hints); i += 2 {
		part := sKey.Render(hints[i]) + " " + sMuted.Render(hints[i+1]) + "   "
		if lipgloss.Width(left+part) > budget {
			break
		}
		left += part
	}
	return spread(" "+left, right, m.width)
}

func (m Model) toastView() string {
	var boxes []string
	for _, t := range m.toasts {
		st := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.color).Padding(0, 1)
		text := ansi.Truncate(t.text, max(10, m.width-12), "…")
		boxes = append(boxes, st.Render(fg(t.color).Bold(true).Render(t.icon)+"  "+sText.Render(text)))
	}
	if len(boxes) == 0 {
		return ""
	}
	return lipgloss.JoinVertical(lipgloss.Right, boxes...)
}

func (m Model) helpView() string {
	section := func(title string, pairs ...string) string {
		lines := []string{sSection.Render(title)}
		for i := 0; i+1 < len(pairs); i += 2 {
			lines = append(lines, sKey.Width(10).Render(pairs[i])+sSubtle.Render(pairs[i+1]))
		}
		return strings.Join(lines, "\n")
	}
	nav := section("Navigate",
		"←→ tab", "switch tabs", "1-6", "jump to tab", "↑↓ j k", "move", "g G", "top / bottom",
		"pgup pgdn", "page", "J K", "scroll details", "↵ / click", "app ⇄ bundled CLI", "/", "fuzzy search")
	view := section("View",
		"b", "group: purpose, source, none", "s", "sort: name, newest, last used", "t", "only untracked (◇)",
		"u", "only updates (↑)", "d", "show brew dependencies", "esc", "clear filters", "r", "rescan now", "R", "recheck updates")
	act := section("Actions",
		"U", "upgrade (asks first)", "X", "uninstall (asks first)", "o", "open app", "f", "reveal in Finder",
		"w", "open homepage", "c", "copy path", "i", "copy install command", "E", "export untracked")
	legend := section("Legend",
		"◆", "declared in dotfiles", "◇", "installed, not in dotfiles", "✗", "declared, not installed", "↑", "update available")
	body := lipgloss.JoinHorizontal(lipgloss.Top, nav, "    ", view) + "\n\n" + lipgloss.JoinHorizontal(lipgloss.Top, act, "    ", legend)
	head := gradient("inv", cIris, cFoam) + sMuted.Render("  "+m.version+" · updates live as you install and remove things")
	vpad := 1
	if m.height < 30 {
		vpad = 0
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cIris).Padding(vpad, 2).
		Render(head + "\n\n" + body + "\n\n" + sMuted.Render("press any key to close"))
}

// Section: Drawing helpers

// box draws a rounded panel with titles embedded in its top and bottom borders.
func box(w, h int, titleL, titleR, footL, footR string, lines []string, border color.Color) string {
	bs := fg(border)
	inner := w - 2
	edge := func(left, right, l, r string) string {
		l, r = pad(l), pad(r)
		fill := w - 4 - lipgloss.Width(l) - lipgloss.Width(r)
		if fill < 1 {
			r, fill = "", max(0, w-4-lipgloss.Width(l))
		}
		return bs.Render(left+"─") + l + bs.Render(strings.Repeat("─", fill)) + r + bs.Render("─"+right)
	}
	out := []string{edge("╭", "╮", titleL, titleR)}
	for i := 0; i < h-2; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		out = append(out, bs.Render("│")+fitWidth(line, inner)+bs.Render("│"))
	}
	return strings.Join(append(out, edge("╰", "╯", footL, footR)), "\n")
}

func pad(s string) string {
	if lipgloss.Width(s) == 0 {
		return ""
	}
	return " " + s + " "
}

// clamp crops composited overlays to the terminal so tiny windows never wrap.
func clamp(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		if lipgloss.Width(l) > w {
			lines[i] = ansi.Truncate(l, w, "")
		}
	}
	return strings.Join(lines, "\n")
}

func fitWidth(s string, w int) string {
	if lw := lipgloss.Width(s); lw > w {
		return ansi.Truncate(s, w, "…")
	} else if lw < w {
		return s + strings.Repeat(" ", w-lw)
	}
	return s
}

func padLeft(s string, w int) string {
	if lw := lipgloss.Width(s); lw < w {
		return strings.Repeat(" ", w-lw) + s
	}
	return s
}

// spread places left and right on one line of width w.
func spread(left, right string, w int) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return ansi.Truncate(left, max(0, w-lipgloss.Width(right)-1), "…") + " " + right
	}
	return left + strings.Repeat(" ", gap) + right
}

// gradient colors each rune along a blend between two hues.
func gradient(s string, from, to color.Color) string {
	runes := []rune(s)
	colors := lipgloss.Blend1D(max(2, len(runes)), from, to)
	var b strings.Builder
	for i, r := range runes {
		b.WriteString(lipgloss.NewStyle().Foreground(colors[i]).Bold(true).Render(string(r)))
	}
	return b.String()
}

// ago formats a time as a short relative phrase.
func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 10*time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "yesterday"
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%d weeks ago", int(d.Hours()/24/7))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%d months ago", int(d.Hours()/24/30))
	}
	return fmt.Sprintf("%d years ago", int(d.Hours()/24/365))
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
