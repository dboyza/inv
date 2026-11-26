package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/dboyza/inv/internal/inv"
)

// testModel builds a model from a small snapshot without touching the real machine.
func testModel(w, h int) Model {
	env := &inv.Env{Home: "/Users/me", BrewPrefix: "/opt/homebrew", Dotfiles: "/Users/me/dotfiles"}
	snap := inv.Snapshot{
		Brew: inv.BrewData{Formulae: []inv.BrewFormula{
			{Name: "terraform", Version: "1.16.2", OnRequest: true, Desc: "Terraform", Bins: []string{"terraform"}},
			{Name: "a-formula-with-an-extremely-long-name-that-overflows", Version: "0.2026.10.21.08.25.stable_02", OnRequest: true},
		}},
		Apps:     []inv.Bundle{{Path: "/Applications/Docker.app", Name: "Docker", ID: "com.docker.docker", Version: "4.91.0"}},
		Bins:     []inv.BinEntry{{Name: "docker", Path: "/usr/local/bin/docker", Target: "/Applications/Docker.app/Contents/Resources/bin/docker"}},
		Fonts:    []inv.FontFile{{File: "HackNerdFont-Bold.ttf", Path: "/Users/me/Library/Fonts/HackNerdFont-Bold.ttf"}},
		Services: []inv.ServiceRaw{{Label: "com.example.agent", Scope: "user agent", Program: "/Users/me/.example/bin/agent", Path: "/Users/me/Library/LaunchAgents/com.example.agent.plist"}},
		Dotfiles: inv.DotfilesData{Repo: "/Users/me/dotfiles", Brews: []string{"terraform", "jq"}},
		Updates:  inv.Updates{Formulae: map[string]string{"terraform": "1.17.0"}},
	}
	m := Model{env: env, version: "test", host: "test-mac", flash: map[string]time.Time{}, failed: map[string]string{}, width: w, height: h, snap: snap, hasData: true}
	m.rebuild()
	return m
}

func TestViewFitsEveryTerminalSize(t *testing.T) {
	for _, size := range [][2]int{{60, 18}, {80, 24}, {100, 30}, {140, 40}, {220, 60}} {
		for tab := range inv.Kinds {
			for _, overlay := range []string{"", "help", "toast", "detail", "search"} {
				m := testModel(size[0], size[1])
				m.setTab(tab)
				switch overlay {
				case "help":
					m.showHelp = true
				case "toast":
					m.notify("+", "a very long toast message about something that was just installed", cFoam)
				case "detail":
					m.detailOpen = true
				case "search":
					m.searching = true
					m.search.SetValue("dock")
				}
				lines := strings.Split(m.View().Content, "\n")
				if len(lines) > size[1] {
					t.Errorf("%dx%d tab %d %s: %d lines tall", size[0], size[1], tab, overlay, len(lines))
				}
				for i, l := range lines {
					if w := lipgloss.Width(l); w > size[0] {
						t.Errorf("%dx%d tab %d %s: line %d is %d wide", size[0], size[1], tab, overlay, i, w)
						break
					}
				}
			}
		}
	}
}

func TestNavigationSkipsGroupHeaders(t *testing.T) {
	m := testModel(140, 40)
	m.setTab(1) // Terminal
	first := m.selected()
	m.move(1)
	second := m.selected()
	if first == nil || second == nil || first.ID == second.ID {
		t.Fatalf("cursor did not advance: %v -> %v", first, second)
	}
	m.move(-100)
	if m.selected().ID != first.ID {
		t.Error("moving far up should clamp to the first item")
	}
}

func TestJumpFollowsBundledCLILinks(t *testing.T) {
	m := testModel(140, 40)
	m.setTab(0)
	m.jump()
	if m.kind() != inv.Terminal || m.selected() == nil || m.selected().Name != "docker" {
		t.Fatalf("jump from Docker landed on %v in %v", m.selected(), m.kind())
	}
	m.jump()
	if m.kind() != inv.Desktop {
		t.Error("jumping back from the CLI should return to its app")
	}
}

func TestUninstallButtonOpensConfirmationThatDefaultsToCancel(t *testing.T) {
	m := testModel(140, 40)
	m.setTab(1)
	m.cursor[m.tab] = "brew:terraform"
	m.askOp(inv.OpUninstall)
	if m.confirm == nil || m.confirmFocus != 1 {
		t.Fatalf("confirm = %v focus = %d", m.confirm, m.confirmFocus)
	}
	next, _ := m.handleConfirmKey("enter")
	if next.(Model).confirm != nil {
		t.Error("enter on the default Cancel button should close the dialog")
	}
	for _, size := range [][2]int{{80, 24}, {140, 40}} {
		m := testModel(size[0], size[1])
		m.setTab(1)
		m.cursor[m.tab] = "brew:terraform"
		m.askOp(inv.OpUpgrade)
		if m.confirm == nil {
			t.Fatal("terraform has an update, so upgrade should be offered")
		}
		for i, l := range strings.Split(m.View().Content, "\n") {
			if lipgloss.Width(l) > size[0] {
				t.Errorf("%v: line %d overflows", size, i)
			}
		}
	}
}

func TestDetailButtonsAreClickable(t *testing.T) {
	m := testModel(140, 40)
	m.setTab(1)
	m.cursor[m.tab] = "brew:terraform"
	lay := m.layout()
	lines, hits := m.detailLines(m.selected(), lay.detailW-4)
	if len(hits) == 0 {
		t.Fatalf("no action buttons in details:\n%s", strings.Join(lines, "\n"))
	}
	h := hits[0]
	kind, ok := m.actionAt(lay.listW+2+h.x0, lay.bodyY+1+h.line)
	if !ok || kind != h.kind {
		t.Errorf("click missed the %v button", h.kind)
	}
}

func TestUpgradeRunsInBackgroundAndFailureRetriesInTerminal(t *testing.T) {
	m := testModel(80, 24)
	m.setTab(1)
	m.cursor[m.tab] = "brew:terraform"
	m.askOp(inv.OpUpgrade)
	next, cmd := m.handleConfirmKey("enter")
	m = next.(Model)
	if len(m.jobs) != 1 || cmd == nil {
		t.Fatalf("confirming an upgrade should queue a background job, got %d", len(m.jobs))
	}
	if !strings.Contains(m.View().Content, "upgrading") {
		t.Error("the running upgrade should show in the header and row")
	}
	m.askOp(inv.OpUninstall)
	if m.confirm != nil {
		t.Error("an item that is upgrading should not offer uninstall")
	}
	next, _ = m.finishOp(opDoneMsg{plan: m.jobs[0], err: errors.New("exit 1"), output: "==> Upgrading\nError: sudo: a terminal is required\n", background: true})
	m = next.(Model)
	if len(m.jobs) != 0 || m.failed["brew:terraform"] != "==> Upgrading\nError: sudo: a terminal is required" {
		t.Fatalf("jobs = %d failed = %q", len(m.jobs), m.failed["brew:terraform"])
	}
	m.askOp(inv.OpUpgrade)
	if m.confirm == nil || m.confirm.Label != "Retry in terminal" {
		t.Fatalf("a failed upgrade should offer a terminal retry, got %v", m.confirm)
	}
	for i, l := range strings.Split(m.View().Content, "\n") {
		if lipgloss.Width(l) > 80 {
			t.Errorf("line %d overflows", i)
		}
	}
}
