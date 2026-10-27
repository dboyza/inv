package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/dboyza/inv/internal/inv"
)

type Model struct{}

func New(env *inv.Env, version string) Model        { return Model{} }
func (m Model) Init() tea.Cmd                       { return nil }
func (m Model) Update(tea.Msg) (tea.Model, tea.Cmd) { return m, tea.Quit }
func (m Model) View() tea.View                      { return tea.NewView("") }
