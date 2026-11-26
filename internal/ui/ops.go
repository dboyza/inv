package ui

import (
	"fmt"
	"image/color"
	"os/exec"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/dboyza/inv/internal/inv"
)

// Section: Plans for the selected item

type opDoneMsg struct {
	plan       inv.Plan
	err        error
	output     string // captured output of a background run
	background bool
}

func (m Model) plans(it *inv.Item) (up, rm inv.Plan) {
	return inv.PlanUpgrade(m.env, *it, m.items), inv.PlanUninstall(m.env, *it, m.items)
}

// askOp opens the confirmation dialog, or explains why the operation is unavailable.
func (m *Model) askOp(kind inv.OpKind) {
	it := m.selected()
	if it == nil {
		return
	}
	up, rm := m.plans(it)
	p := up
	if kind == inv.OpUninstall {
		p = rm
	}
	if !p.Allowed() {
		m.notify("!", p.Blocked, cGold)
		return
	}
	if m.queued(it.ID) >= 0 {
		m.notify("!", it.Name+" is already upgrading", cGold)
		return
	}
	if m.failed[it.ID] != "" && kind == inv.OpUpgrade {
		p.Label = "Retry in terminal"
		p.Warnings = append(p.Warnings, "The background upgrade failed, so this retry runs in the terminal where prompts can be answered")
	}
	m.confirm = &p
	// Destructive dialogs default to Cancel; upgrades default to confirm.
	m.confirmFocus = 0
	if kind == inv.OpUninstall {
		m.confirmFocus = 1
	}
}

// runPlan hands the terminal to the real command so output and Touch ID or sudo prompts stay visible.
// On failure it pauses so the error can be read before inv returns.
func runPlan(p inv.Plan) tea.Cmd {
	const script = `printf '\n\033[1m%s\033[0m\n\033[2m$ %s\033[0m\n\n' "$1" "$2"; shift 2
"$@"; s=$?
if [ "$s" -ne 0 ]; then printf '\n\033[31mExited with code %d.\033[0m Press return to go back to inv. ' "$s"; read _; fi
exit "$s"`
	title := fmt.Sprintf("inv · %s %s", strings.ToLower(p.Kind.String()), p.Item.Name)
	args := append([]string{"-c", script, "inv", title, p.Command()}, p.Argv...)
	return tea.ExecProcess(exec.Command("/bin/sh", args...), func(err error) tea.Msg { return opDoneMsg{plan: p, err: err} })
}

// runBackground captures the command's output without a terminal, so password prompts fail fast instead of drawing over inv.
func runBackground(p inv.Plan) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command(p.Argv[0], p.Argv[1:]...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		out, err := cmd.CombinedOutput()
		return opDoneMsg{plan: p, err: err, output: string(out), background: true}
	}
}

// queued reports an item's place in the upgrade queue, where 0 is running, or -1.
func (m Model) queued(id string) int {
	for i, p := range m.jobs {
		if p.Item.ID == id {
			return i
		}
	}
	return -1
}

// enqueue adds a background upgrade; jobs run one at a time because package managers hold locks.
func (m *Model) enqueue(p inv.Plan) tea.Cmd {
	m.jobs = append(m.jobs, p)
	if len(m.jobs) > 1 {
		m.notify("…", p.Item.Name+" queued", cSubtle)
		return nil
	}
	return runBackground(p)
}

func (m Model) finishOp(msg opDoneMsg) (tea.Model, tea.Cmd) {
	id, name := msg.plan.Item.ID, msg.plan.Item.Name
	verb := map[inv.OpKind]string{inv.OpUpgrade: "Upgraded", inv.OpUninstall: "Removed"}[msg.plan.Kind]
	if msg.err != nil {
		m.notify("!", fmt.Sprintf("%s %s failed", strings.ToLower(msg.plan.Kind.String()), name), cLove)
		if msg.background {
			m.failed[id] = tail(msg.output, 3)
		}
	} else {
		m.notify("✓", verb+" "+name, cPine)
		delete(m.failed, id)
	}
	cmds := []tea.Cmd{m.scanCmd(append(msg.plan.Groups, inv.GHistory), false)}
	m.scans++
	if msg.background && len(m.jobs) > 0 {
		m.jobs = m.jobs[1:]
		if len(m.jobs) > 0 {
			cmds = append(cmds, runBackground(m.jobs[0]))
		}
	}
	return m, tea.Batch(cmds...)
}

// tail keeps the last n non-empty lines of command output, without colors.
func tail(out string, n int) string {
	var lines []string
	for _, l := range strings.Split(ansi.Strip(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return "exited without output"
	}
	return strings.Join(lines[max(0, len(lines)-n):], "\n")
}

// Section: Confirmation dialog

func (m Model) handleConfirmKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "n", "q":
		m.confirm = nil
	case "tab", "shift+tab", "left", "right", "h", "l":
		m.confirmFocus = 1 - m.confirmFocus
	case "y":
		return m.confirmChoice(0)
	case "enter", "space":
		return m.confirmChoice(m.confirmFocus)
	}
	return m, nil
}

func (m Model) confirmChoice(choice int) (tea.Model, tea.Cmd) {
	p := m.confirm
	m.confirm = nil
	if choice != 0 || p == nil {
		return m, nil
	}
	// Upgrades run in the background; uninstalls and retries keep the terminal for prompts.
	if p.Kind == inv.OpUpgrade && m.failed[p.Item.ID] == "" {
		return m, m.enqueue(*p)
	}
	return m, runPlan(*p)
}

// confirmLayout renders the dialog and reports where its two buttons sit, relative to the dialog.
func (m Model) confirmLayout() (string, [2][2]int, int) {
	p := m.confirm
	w := min(72, m.width-4)
	inner := w - 6
	accent, icon := cFoam, "↑"
	if p.Kind == inv.OpUninstall {
		accent, icon = cLove, "✕"
	}
	lines := []string{
		fg(accent).Bold(true).Render(icon+" "+p.Kind.String()+" ") + sBright.Render(p.Item.Name) + fg(accent).Bold(true).Render("?"),
		"",
	}
	for _, l := range strings.Split(ansi.Wrap(p.Command(), inner-2, " /"), "\n") {
		lines = append(lines, sMuted.Render("$ ")+sText.Render(l))
	}
	if len(p.Warnings) > 0 {
		lines = append(lines, "")
		for _, warn := range p.Warnings {
			for i, l := range strings.Split(ansi.Wordwrap(warn, inner-2, " "), "\n") {
				prefix := sWarn.Render("! ")
				if i > 0 {
					prefix = "  "
				}
				lines = append(lines, prefix+sSubtle.Render(l))
			}
		}
	}
	confirmLabel := p.Label
	buttons := [2]string{button(confirmLabel, accent, m.confirmFocus == 0), button("Cancel", cSubtle, m.confirmFocus == 1)}
	lines = append(lines, "", buttons[0]+"   "+buttons[1], "", sMuted.Render("enter choose · y confirm · tab switch · esc cancel"))
	btnLine := len(lines) - 3
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(1, 2).Width(w).Render(strings.Join(lines, "\n"))
	// Border (1) + top padding (1) put content on row 2; left border + padding put it at column 3.
	b0 := lipgloss.Width(buttons[0])
	spans := [2][2]int{{3, 3 + b0}, {3 + b0 + 3, 3 + b0 + 3 + lipgloss.Width(buttons[1])}}
	return box, spans, btnLine + 2
}

func (m Model) confirmOrigin(box string) (int, int) {
	return max(0, (m.width-lipgloss.Width(box))/2), max(0, (m.height-lipgloss.Height(box))/2)
}

func (m Model) handleConfirmClick(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	box, spans, row := m.confirmLayout()
	x0, y0 := m.confirmOrigin(box)
	if mouse.Y == y0+row {
		for i, s := range spans {
			if mouse.X >= x0+s[0] && mouse.X < x0+s[1] {
				return m.confirmChoice(i)
			}
		}
	}
	return m, nil
}

// button draws a pill; the focused one is filled with its accent.
func button(label string, c color.Color, focused bool) string {
	if focused {
		return lipgloss.NewStyle().Foreground(cBase).Background(c).Bold(true).Render(" " + label + " ")
	}
	return lipgloss.NewStyle().Foreground(c).Background(cSurface).Render(" " + label + " ")
}

// Section: Detail pane action buttons

type actionHit struct {
	line, x0, x1 int
	kind         inv.OpKind
}

// actionBar renders Upgrade and Uninstall buttons, or the reason they are unavailable.
func (m Model) actionBar(it *inv.Item, w int) (lines []string, hits []actionHit) {
	up, rm := m.plans(it)
	if q := m.queued(it.ID); q >= 0 {
		status := m.spin() + sSubtle.Render(" Upgrading in the background…")
		if q > 0 {
			status = sMuted.Render(fmt.Sprintf("… Queued to upgrade (%d ahead)", q))
		}
		return []string{status}, nil
	}
	var row string
	x := 0
	add := func(p inv.Plan, accent color.Color, icon string, filled bool) {
		b := button(icon+" "+p.Label, accent, filled)
		if row != "" {
			row += "  "
			x += 2
		}
		hits = append(hits, actionHit{x0: x, x1: x + lipgloss.Width(b), kind: p.Kind})
		row += b
		x += lipgloss.Width(b)
	}
	if up.Allowed() {
		add(up, cFoam, "↑", true)
	}
	if rm.Allowed() {
		add(rm, cLove, "✕", false)
	}
	if row != "" {
		keys := []string{}
		if up.Allowed() {
			keys = append(keys, "U")
		}
		if rm.Allowed() {
			keys = append(keys, "X")
		}
		row += "  " + sMuted.Render(strings.Join(keys, " · "))
		lines = append(lines, row)
	}
	if !rm.Allowed() && rm.Blocked != "Not installed" && it.Declared != inv.DeclMissing {
		lines = append(lines, sMuted.Render(ansi.Truncate("✕ "+rm.Blocked, w, "…")))
	}
	if out := m.failed[it.ID]; out != "" {
		lines = append(lines, sErr.Render("! Upgrade failed"))
		for _, l := range strings.Split(out, "\n") {
			lines = append(lines, sMuted.Render(ansi.Truncate("  "+l, w, "…")))
		}
	}
	return lines, hits
}

// Section: Export modal

func (m *Model) openExport() tea.Cmd {
	m.exportText = inv.ExportUntracked(m.items, m.host, time.Now())
	m.exportScroll = 0
	m.exportOpen = true
	return m.copy(m.exportText, "Copied untracked installs as dotfiles snippets")
}

func (m Model) exportView() string {
	w := min(96, m.width-4)
	h := max(6, m.height-6)
	all := strings.Split(strings.TrimRight(m.exportText, "\n"), "\n")
	scroll := min(m.exportScroll, max(0, len(all)-(h-4)))
	body := all[scroll:]
	if len(body) > h-4 {
		body = body[:h-4]
	}
	for i, l := range body {
		st := sText
		if strings.HasPrefix(strings.TrimSpace(l), "#") {
			st = sMuted
		}
		body[i] = st.Render(ansi.Truncate(l, w-6, "…"))
	}
	head := fg(cIris).Bold(true).Render("Export untracked installs") + sMuted.Render(fmt.Sprintf("  %d lines · copied to clipboard", len(all)))
	foot := sMuted.Render("j k scroll · c copy again · esc close")
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cIris).Padding(0, 2).Width(w).
		Render(head + "\n\n" + strings.Join(body, "\n") + "\n\n" + foot)
}

func (m Model) handleExportKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q", "E":
		m.exportOpen = false
	case "j", "down":
		m.exportScroll++
	case "k", "up":
		m.exportScroll = max(0, m.exportScroll-1)
	case "c":
		return m, m.copy(m.exportText, "Copied again")
	}
	return m, nil
}
