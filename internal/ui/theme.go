// Package ui renders the inventory as a Bubble Tea app themed with Rose Pine Moon.
package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"

	"github.com/dboyza/inv/internal/inv"
)

// Section: Rose Pine Moon palette (matches dotfiles theme/palette.json)

var (
	cBase      = lipgloss.Color("#232136")
	cSurface   = lipgloss.Color("#2a273f")
	cOverlay   = lipgloss.Color("#393552")
	cHighlight = lipgloss.Color("#44415a")
	cMuted     = lipgloss.Color("#6e6a86")
	cSubtle    = lipgloss.Color("#908caa")
	cText      = lipgloss.Color("#e0def4")
	cBright    = lipgloss.Color("#eeecff")
	cLove      = lipgloss.Color("#eb6f92")
	cGold      = lipgloss.Color("#f6c177")
	cRose      = lipgloss.Color("#ea9a97")
	cPine      = lipgloss.Color("#3e8fb0")
	cFoam      = lipgloss.Color("#9ccfd8")
	cIris      = lipgloss.Color("#c4a7e7")
)

// kindAccent gives every tab its own hue so the two main kinds never blur together.
var kindAccent = map[inv.Kind]color.Color{
	inv.Desktop:   cIris,
	inv.Terminal:  cFoam,
	inv.Runtime:   cGold,
	inv.Font:      cRose,
	inv.Extension: cPine,
	inv.Service:   cLove,
}

// kindIcon is a single-cell glyph per tab; plain Unicode so no Nerd Font is required.
var kindIcon = map[inv.Kind]string{
	inv.Desktop:   "◫",
	inv.Terminal:  "❯",
	inv.Runtime:   "λ",
	inv.Font:      "Aa",
	inv.Extension: "⧉",
	inv.Service:   "↻",
}

// Section: Source badges

var sourceColor = map[inv.Source]color.Color{
	inv.SrcBrew: cGold, inv.SrcCask: cGold, inv.SrcAppStore: cFoam, inv.SrcDotfiles: cIris,
	inv.SrcNpm: cLove, inv.SrcUv: cPine, inv.SrcPythonOrg: cPine, inv.SrcInstaller: cRose,
	inv.SrcManual: cSubtle, inv.SrcBundled: cPine, inv.SrcDirect: cSubtle, inv.SrcVSCode: cPine,
	inv.SrcLazy: cFoam, inv.SrcNix: cIris, inv.SrcSystem: cMuted, inv.SrcPlugin: cRose,
}

// sourceLabel is the long form shown in the detail pane.
var sourceLabel = map[inv.Source]string{
	inv.SrcBrew: "Homebrew formula", inv.SrcCask: "Homebrew cask", inv.SrcAppStore: "Mac App Store",
	inv.SrcDotfiles: "Dotfiles", inv.SrcNpm: "npm global", inv.SrcUv: "uv", inv.SrcPythonOrg: "python.org installer",
	inv.SrcInstaller: "Vendor installer", inv.SrcManual: "Manual", inv.SrcBundled: "Bundled with an app",
	inv.SrcDirect: "Direct download", inv.SrcVSCode: "VS Code marketplace", inv.SrcLazy: "lazy.nvim",
	inv.SrcNix: "Nix", inv.SrcSystem: "System", inv.SrcPlugin: "Plugin",
}

// Section: Shared styles

var (
	sText    = lipgloss.NewStyle().Foreground(cText)
	sBright  = lipgloss.NewStyle().Foreground(cBright).Bold(true)
	sMuted   = lipgloss.NewStyle().Foreground(cMuted)
	sSubtle  = lipgloss.NewStyle().Foreground(cSubtle)
	sKey     = lipgloss.NewStyle().Foreground(cText).Bold(true)
	sLabel   = lipgloss.NewStyle().Foreground(cMuted).Width(11)
	sWarn    = lipgloss.NewStyle().Foreground(cGold)
	sErr     = lipgloss.NewStyle().Foreground(cLove)
	sGood    = lipgloss.NewStyle().Foreground(cFoam)
	sUpdate  = lipgloss.NewStyle().Foreground(cFoam).Bold(true)
	sBorder  = lipgloss.NewStyle().Foreground(cOverlay)
	sSection = lipgloss.NewStyle().Foreground(cSubtle).Bold(true)
)

func fg(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }
