package ui

import (
	"fmt"
	"image/color"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/dboyza/inv/internal/inv"
)

// Section: Timing

const (
	tickIdle      = time.Second
	tickBusy      = 90 * time.Millisecond
	toastLife     = 5 * time.Second
	flashLife     = 12 * time.Second
	updatesMaxAge = 30 * time.Minute // cached update info older than this is rechecked at launch
	narrowBreakpt = 100              // below this width the detail pane becomes an overlay
	maxToasts     = 4
	spinnerFrames = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
)

// Section: View modes

type groupMode int

const (
	groupPurpose groupMode = iota
	groupSource
	groupFlat
)

func (g groupMode) String() string { return [...]string{"purpose", "source", "none"}[g] }

type sortMode int

const (
	sortName sortMode = iota
	sortNewest
	sortLastUsed
)

func (s sortMode) String() string { return [...]string{"name", "newest", "last used"}[s] }

// Section: Messages

type (
	scanDoneMsg struct {
		snap    inv.Snapshot
		groups  []inv.Group
		initial bool
	}
	updatesDoneMsg struct{ updates inv.Updates }
	watchMsg       struct{ groups []inv.Group }
	watcherReady   struct{ w *inv.Watcher }
	tickMsg        time.Time
)

type toast struct {
	icon, text string
	color      color.Color
	expires    time.Time
}

// Section: Model

// Model is the whole application state.
type Model struct {
	env     *inv.Env
	version string
	host    string

	snap    inv.Snapshot
	items   []inv.Item
	byID    map[string]*inv.Item
	hasData bool

	tab     int
	cursor  [6]string // selected item ID per tab
	offset  [6]int    // list scroll offset per tab
	dScroll int       // detail pane scroll

	search      textinput.Model
	searching   bool
	showDeps    bool
	untracked   bool
	updatesOnly bool
	group       groupMode
	sort        sortMode

	scans      int // scans in flight
	checking   bool
	watcher    *inv.Watcher
	watchErr   string
	frame      int
	toasts     []toast
	flash      map[string]time.Time
	showHelp   bool
	detailOpen bool // narrow layout detail overlay

	width, height int
}

// New builds the model and paints from cache when one exists.
func New(env *inv.Env, version string) Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "search every install…"
	ti.CharLimit = 64
	host, _ := os.Hostname()
	host = strings.TrimSuffix(host, ".local")
	m := Model{env: env, version: version, host: host, search: ti, flash: map[string]time.Time{}, width: 120, height: 36, scans: 1}
	if snap, ok := inv.LoadCache(); ok {
		m.snap = snap
		m.rebuild()
		m.hasData = true
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.scanCmd(inv.AllGroups, true), startWatcher(m.env), tick(tickBusy))
}

// Section: Commands

func (m Model) scanCmd(groups []inv.Group, initial bool) tea.Cmd {
	env, base := m.env, m.snap
	return func() tea.Msg {
		snap := inv.Collect(env, base, groups)
		for _, g := range groups {
			if g == inv.GBins {
				snap = inv.ProbeVersions(env, snap)
			}
		}
		return scanDoneMsg{snap: snap, groups: groups, initial: initial}
	}
}

func (m Model) updatesCmd() tea.Cmd {
	env, base := m.env, m.snap
	return func() tea.Msg { return updatesDoneMsg{inv.CheckUpdates(env, base).Updates} }
}

func startWatcher(env *inv.Env) tea.Cmd {
	return func() tea.Msg {
		w, err := inv.NewWatcher(env)
		if err != nil {
			return watcherReady{}
		}
		return watcherReady{w}
	}
}

func waitForChange(w *inv.Watcher) tea.Cmd {
	return func() tea.Msg { return watchMsg{<-w.Changes} }
}

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) busy() bool { return m.scans > 0 || m.checking }

// Section: Update

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampScroll()
		return m, nil

	case tickMsg:
		m.frame++
		now := time.Time(msg)
		live := m.toasts[:0]
		for _, t := range m.toasts {
			if now.Before(t.expires) {
				live = append(live, t)
			}
		}
		m.toasts = live
		for id, until := range m.flash {
			if now.After(until) {
				delete(m.flash, id)
			}
		}
		d := tickIdle
		if m.busy() {
			d = tickBusy
		}
		return m, tick(d)

	case scanDoneMsg:
		return m.applyScan(msg)

	case updatesDoneMsg:
		m.checking = false
		m.snap.Updates = msg.updates
		before := m.countUpdates()
		m.rebuild()
		_ = inv.SaveCache(m.snap)
		if n := m.countUpdates(); n > 0 && n != before {
			m.notify("↑", fmt.Sprintf("%d updates available", n), cFoam)
		}
		return m, nil

	case watcherReady:
		if msg.w == nil {
			m.watchErr = "file watching unavailable"
			return m, nil
		}
		m.watcher = msg.w
		return m, waitForChange(msg.w)

	case watchMsg:
		m.scans++
		return m, tea.Batch(m.scanCmd(msg.groups, false), waitForChange(m.watcher))

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.MouseClickMsg:
		return m.handleClick(msg.Mouse())

	case tea.MouseWheelMsg:
		mouse := msg.Mouse()
		delta := 3
		if mouse.Button == tea.MouseWheelUp {
			delta = -3
		}
		if m.overDetail(mouse.X) {
			m.dScroll = max(0, m.dScroll+delta)
		} else {
			m.move(delta)
		}
		return m, nil
	}

	if m.searching {
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		return m, cmd
	}
	return m, nil
}

// applyScan merges a finished scan, announces what changed, and schedules follow-up work.
func (m Model) applyScan(msg scanDoneMsg) (tea.Model, tea.Cmd) {
	m.scans = max(0, m.scans-1)
	before := m.items
	hadData := m.hasData
	m.snap.Merge(msg.snap, msg.groups)
	m.rebuild()
	m.hasData = true
	_ = inv.SaveCache(m.snap)

	if hadData {
		m.announce(inv.Diff(before, m.items), msg.initial)
	}
	var cmds []tea.Cmd
	if msg.initial && time.Since(m.snap.Updates.CheckedAt) > updatesMaxAge {
		m.checking = true
		cmds = append(cmds, m.updatesCmd())
	}
	return m, tea.Batch(cmds...)
}

// announce turns scan differences into toasts and row highlights.
func (m *Model) announce(changes []inv.Change, sinceLaunch bool) {
	if len(changes) == 0 {
		return
	}
	now := time.Now()
	for _, c := range changes {
		if c.Kind != inv.Removed {
			m.flash[c.Item.ID] = now.Add(flashLife)
		}
	}
	if sinceLaunch || len(changes) > maxToasts {
		var added, removed, changed int
		for _, c := range changes {
			switch c.Kind {
			case inv.Added:
				added++
			case inv.Removed:
				removed++
			case inv.Changed:
				changed++
			}
		}
		parts := []string{}
		if added > 0 {
			parts = append(parts, fmt.Sprintf("+%d new", added))
		}
		if removed > 0 {
			parts = append(parts, fmt.Sprintf("−%d removed", removed))
		}
		if changed > 0 {
			parts = append(parts, fmt.Sprintf("%d updated", changed))
		}
		label := "Changes since last launch: "
		if !sinceLaunch {
			label = ""
		}
		m.notify("◆", label+strings.Join(parts, " · "), cIris)
		return
	}
	for _, c := range changes {
		where := c.Item.Kind.String()
		switch c.Kind {
		case inv.Added:
			m.notify("+", fmt.Sprintf("%s installed  %s", c.Item.Name, sMuted.Render(where)), cFoam)
		case inv.Removed:
			m.notify("−", fmt.Sprintf("%s removed  %s", c.Item.Name, sMuted.Render(where)), cLove)
		case inv.Changed:
			m.notify("↑", fmt.Sprintf("%s %s → %s", c.Item.Name, c.From, c.Item.Version), cGold)
		}
	}
}

func (m *Model) notify(icon, text string, c color.Color) {
	m.toasts = append(m.toasts, toast{icon: icon, text: text, color: c, expires: time.Now().Add(toastLife)})
	if len(m.toasts) > maxToasts {
		m.toasts = m.toasts[len(m.toasts)-maxToasts:]
	}
}

func (m *Model) rebuild() {
	m.items = inv.Build(m.env, m.snap)
	m.byID = make(map[string]*inv.Item, len(m.items))
	for i := range m.items {
		m.byID[m.items[i].ID] = &m.items[i]
	}
	m.clampScroll()
}

func (m Model) countUpdates() int {
	n := 0
	for _, it := range m.items {
		if it.Updatable() && !it.Dependency {
			n++
		}
	}
	return n
}

// Section: Navigation

func (m Model) kind() inv.Kind { return inv.Kinds[m.tab] }

// selectedIndex finds the cursor among visible rows, falling back to the first item.
func (m Model) selectedIndex(rows []row) int {
	first := -1
	for i, r := range rows {
		if r.item == nil {
			continue
		}
		if first < 0 {
			first = i
		}
		if r.item.ID == m.cursor[m.tab] {
			return i
		}
	}
	return first
}

func (m Model) selected() *inv.Item {
	rows := m.rows()
	if i := m.selectedIndex(rows); i >= 0 {
		return rows[i].item
	}
	return nil
}

// move shifts the cursor by delta item rows, skipping group headers.
func (m *Model) move(delta int) {
	rows := m.rows()
	idx := m.selectedIndex(rows)
	if idx < 0 {
		return
	}
	var items []int
	pos := 0
	for i, r := range rows {
		if r.item != nil {
			if i == idx {
				pos = len(items)
			}
			items = append(items, i)
		}
	}
	pos = min(max(pos+delta, 0), len(items)-1)
	m.cursor[m.tab] = rows[items[pos]].item.ID
	m.dScroll = 0
	m.clampScroll()
}

func (m *Model) moveTo(end bool) {
	if end {
		m.move(1 << 20)
	} else {
		m.move(-(1 << 20))
	}
}

func (m *Model) setTab(t int) {
	m.tab = (t + len(inv.Kinds)) % len(inv.Kinds)
	m.dScroll = 0
	m.clampScroll()
}

// clampScroll keeps the selected row visible inside the list viewport.
func (m *Model) clampScroll() {
	rows := m.rows()
	idx := m.selectedIndex(rows)
	if idx >= 0 && m.cursor[m.tab] == "" {
		m.cursor[m.tab] = rows[idx].item.ID
	}
	h := m.listHeight()
	off := m.offset[m.tab]
	if idx >= 0 {
		// Keep the group header above the first item in view.
		top := idx
		if idx > 0 && rows[idx-1].item == nil {
			top = idx - 1
		}
		if top < off {
			off = top
		}
		if idx >= off+h {
			off = idx - h + 1
		}
	}
	off = min(off, max(0, len(rows)-h))
	m.offset[m.tab] = max(0, off)
}

// jump follows bundled-CLI links between an app and its command-line tools.
func (m *Model) jump() {
	it := m.selected()
	if it == nil {
		return
	}
	if m.width < narrowBreakpt && !m.detailOpen {
		m.detailOpen = true // narrow screens show details on demand
		return
	}
	target := it.ProvidedBy
	if target == "" && len(it.Provides) > 0 {
		target = it.Provides[0]
	}
	dest := m.byID[target]
	if dest == nil {
		return
	}
	for i, k := range inv.Kinds {
		if k == dest.Kind {
			m.tab = i
		}
	}
	// Make sure filters don't hide the destination.
	m.untracked, m.updatesOnly = false, false
	m.search.SetValue("")
	m.cursor[m.tab] = dest.ID
	m.flash[dest.ID] = time.Now().Add(1500 * time.Millisecond)
	m.dScroll = 0
	m.clampScroll()
}
