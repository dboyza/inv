package ui

import (
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/dboyza/inv/internal/inv"
)

// Section: Keyboard

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}
	if m.searching {
		switch key {
		case "esc":
			m.searching = false
			m.search.SetValue("")
			m.search.Blur()
		case "enter":
			m.searching = false
			m.search.Blur()
		case "up", "ctrl+p":
			m.move(-1)
		case "down", "ctrl+n":
			m.move(1)
		case "tab":
			m.setTab(m.tab + 1)
		case "shift+tab":
			m.setTab(m.tab - 1)
		default:
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			m.offset[m.tab] = 0
			m.clampScroll()
			return m, cmd
		}
		m.clampScroll()
		return m, nil
	}

	switch key {
	case "q":
		return m, tea.Quit
	case "esc":
		switch {
		case m.detailOpen:
			m.detailOpen = false
		case m.search.Value() != "":
			m.search.SetValue("")
		default:
			m.untracked, m.updatesOnly = false, false
		}
		m.clampScroll()
	case "?":
		m.showHelp = true
	case "/":
		m.searching = true
		return m, m.search.Focus()
	case "tab", "right", "l":
		m.setTab(m.tab + 1)
	case "shift+tab", "left", "h":
		m.setTab(m.tab - 1)
	case "1", "2", "3", "4", "5", "6":
		m.setTab(int(key[0] - '1'))
	case "down", "j":
		m.move(1)
	case "up", "k":
		m.move(-1)
	case "pgdown", "ctrl+d":
		m.move(m.listHeight() / 2)
	case "pgup", "ctrl+u":
		m.move(-m.listHeight() / 2)
	case "g", "home":
		m.moveTo(false)
	case "G", "end":
		m.moveTo(true)
	case "J":
		m.dScroll++
	case "K":
		m.dScroll = max(0, m.dScroll-1)
	case "enter", "space":
		m.jump()
	case "b":
		m.group = (m.group + 1) % 3
		m.offset[m.tab] = 0
		m.clampScroll()
		m.notify("▤", "Grouped by "+m.group.String(), cSubtle)
	case "s":
		m.sort = (m.sort + 1) % 3
		m.clampScroll()
		m.notify("⇅", "Sorted by "+m.sort.String(), cSubtle)
	case "t":
		m.untracked = !m.untracked
		m.clampScroll()
	case "u":
		m.updatesOnly = !m.updatesOnly
		m.clampScroll()
	case "d":
		m.showDeps = !m.showDeps
		m.clampScroll()
	case "r":
		m.scans++
		m.notify("↻", "Rescanning everything", cSubtle)
		return m, m.scanCmd(inv.AllGroups, false)
	case "U":
		if !m.checking {
			m.checking = true
			m.notify("↻", "Checking for updates", cSubtle)
			return m, m.updatesCmd()
		}
	case "o":
		return m, m.act(actOpen)
	case "f":
		return m, m.act(actReveal)
	case "w":
		return m, m.act(actWeb)
	case "c":
		return m, m.act(actCopyPath)
	case "i":
		return m, m.act(actCopyInstall)
	}
	return m, nil
}

// Section: Mouse

func (m Model) handleClick(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}
	lay := m.layout()
	if mouse.Y == lay.tabsY {
		for i, span := range m.tabSpans() {
			if mouse.X >= span[0] && mouse.X < span[1] {
				m.setTab(i)
			}
		}
		return m, nil
	}
	if mouse.Y >= lay.bodyY+1 && mouse.Y < lay.bodyY+1+m.listHeight() && mouse.X < lay.listW {
		rows := m.rows()
		i := m.offset[m.tab] + mouse.Y - lay.bodyY - 1
		if i >= 0 && i < len(rows) && rows[i].item != nil {
			if rows[i].item.ID == m.cursor[m.tab] {
				m.jump() // second click follows the app/CLI link
			} else {
				m.cursor[m.tab] = rows[i].item.ID
				m.dScroll = 0
			}
		}
	}
	return m, nil
}

func (m Model) overDetail(x int) bool {
	lay := m.layout()
	return lay.detailW > 0 && x >= lay.listW
}

// Section: Safe actions

type action int

const (
	actOpen action = iota
	actReveal
	actWeb
	actCopyPath
	actCopyInstall
)

// act performs a non-destructive action on the selected item and reports the outcome as a toast.
func (m *Model) act(a action) tea.Cmd {
	it := m.selected()
	if it == nil {
		return nil
	}
	fail := func(text string) tea.Cmd { m.notify("!", text, cGold); return nil }
	switch a {
	case actOpen:
		if it.Kind != inv.Desktop {
			return m.act(actReveal)
		}
		if it.Path == "" {
			return fail("Not installed")
		}
		m.notify("↗", "Opening "+it.Name, cIris)
		return runDetached("open", it.Path)
	case actReveal:
		if it.Path == "" {
			return fail("No location on disk")
		}
		m.notify("↗", "Revealed in Finder", cIris)
		return runDetached("open", "-R", it.Path)
	case actWeb:
		if it.Homepage == "" {
			return fail("No homepage known for " + it.Name)
		}
		m.notify("↗", "Opening "+shortURL(it.Homepage), cIris)
		return runDetached("open", it.Homepage)
	case actCopyPath:
		if it.Path == "" {
			return fail("No location on disk")
		}
		return m.copy(it.Path, "Copied path")
	case actCopyInstall:
		cmd := installCommand(it)
		if cmd == "" {
			return fail("No reinstall command known for " + string(it.Source) + " installs")
		}
		return m.copy(cmd, "Copied "+cmd)
	}
	return nil
}

func (m *Model) copy(text, label string) tea.Cmd {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err != nil {
		m.notify("!", "Clipboard unavailable", cGold)
		return nil
	}
	m.notify("✓", label, cPine)
	return nil
}

func runDetached(name string, args ...string) tea.Cmd {
	return func() tea.Msg {
		_ = exec.Command(name, args...).Start()
		return nil
	}
}

// installCommand reproduces how an item was installed, for rebuilding a machine or sharing.
func installCommand(it *inv.Item) string {
	switch it.Source {
	case inv.SrcBrew:
		return "brew install " + it.SourceRef
	case inv.SrcCask:
		return "brew install --cask " + it.SourceRef
	case inv.SrcAppStore:
		if it.SourceRef != "" {
			return "mas install " + it.SourceRef
		}
	case inv.SrcNpm:
		return "npm install -g " + it.Name
	case inv.SrcUv:
		return "uv python install " + it.Version
	case inv.SrcVSCode:
		return "code --install-extension " + it.SourceRef
	}
	return ""
}

func shortURL(u string) string {
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	return strings.TrimSuffix(u, "/")
}
