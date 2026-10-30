// Package inv discovers custom installs on macOS and models them as display-ready items.
package inv

import (
	"sort"
	"strings"
	"time"
)

// Section: Kinds

// Kind is the top-level tab an item belongs to.
type Kind int

const (
	Desktop Kind = iota
	Terminal
	Runtime
	Font
	Extension
	Service
)

// Kinds lists every kind in tab order.
var Kinds = []Kind{Desktop, Terminal, Runtime, Font, Extension, Service}

func (k Kind) String() string {
	return [...]string{"Desktop", "Terminal", "Runtimes", "Fonts", "Extensions", "Services"}[k]
}

// Section: Sources

// Source names the mechanism that installed an item.
type Source string

const (
	SrcBrew      Source = "brew"
	SrcCask      Source = "cask"
	SrcAppStore  Source = "app store"
	SrcDotfiles  Source = "dotfiles"
	SrcNpm       Source = "npm"
	SrcUv        Source = "uv"
	SrcInstaller Source = "installer"
	SrcManual    Source = "manual"
	SrcDirect    Source = "direct"
	SrcBundled   Source = "bundled"
	SrcPythonOrg Source = "python.org"
	SrcVSCode    Source = "vscode"
	SrcLazy      Source = "lazy.nvim"
	SrcNix       Source = "nix"
	SrcSystem    Source = "system"
	SrcPlugin    Source = "plugin"
)

// Declared reports how an item relates to the dotfiles declarations.
type Declared int

const (
	DeclUnknown   Declared = iota // no dotfiles repo was found
	DeclTracked                   // declared by dotfiles and present
	DeclUntracked                 // present but not declared
	DeclMissing                   // declared but not installed
)

// Section: Item

// Item is one installed thing, normalized across every source.
type Item struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Kind        Kind      `json:"kind"`
	Category    string    `json:"category"`
	Source      Source    `json:"source"`
	SourceRef   string    `json:"source_ref,omitempty"` // cask token, formula, app store id, package name
	Version     string    `json:"version,omitempty"`
	Latest      string    `json:"latest,omitempty"` // set only when an update is available
	Desc        string    `json:"desc,omitempty"`
	Homepage    string    `json:"homepage,omitempty"`
	Path        string    `json:"path,omitempty"`
	BundleID    string    `json:"bundle_id,omitempty"`
	InstalledAt time.Time `json:"installed_at,omitzero"`
	LastUsed    time.Time `json:"last_used,omitzero"`
	ShellRuns   int       `json:"shell_runs,omitempty"`
	Declared    Declared  `json:"declared"`
	DeclaredIn  string    `json:"declared_in,omitempty"`
	Dependency  bool      `json:"dependency,omitempty"` // brew formula not installed on request
	Apple       bool      `json:"apple,omitempty"`
	Commands    []string  `json:"commands,omitempty"`
	Members     []string  `json:"members,omitempty"` // font styles
	Files       []string  `json:"files,omitempty"`   // exact font files in a family
	Deps        []string  `json:"deps,omitempty"`
	Provides    []string  `json:"provides,omitempty"`    // IDs of CLIs bundled in this app
	ProvidedBy  string    `json:"provided_by,omitempty"` // ID of the app bundling this CLI
	Running     bool      `json:"running,omitempty"`
	Notes       []string  `json:"notes,omitempty"`
}

// Updatable reports whether a newer version is known.
func (it Item) Updatable() bool { return it.Latest != "" && it.Latest != it.Version }

// SearchText is the haystack used for fuzzy filtering.
func (it Item) SearchText() string {
	return strings.Join([]string{it.Name, it.Category, string(it.Source), it.SourceRef, it.Desc}, " ")
}

// SortItems orders items by name, case-insensitively.
func SortItems(items []Item) {
	sort.SliceStable(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
}
