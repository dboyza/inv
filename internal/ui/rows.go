package ui

import (
	"sort"
	"strings"

	"github.com/sahilm/fuzzy"

	"github.com/dboyza/inv/internal/inv"
)

// row is one line of the list: a group header or an item.
type row struct {
	title   string // group header text; empty for item rows
	count   int
	item    *inv.Item
	matches []int // matched rune positions in the item name
}

// match is an item that passed the active filters.
type match struct {
	item    *inv.Item
	matches []int
	score   int
}

// filtered returns the items of kind k that pass every active filter and the search query.
func (m Model) filtered(k inv.Kind) []match {
	q := strings.TrimSpace(m.search.Value())
	var out []match
	for i := range m.items {
		it := &m.items[i]
		if it.Kind != k {
			continue
		}
		if it.Dependency && !m.showDeps {
			continue
		}
		if m.untracked && it.Declared != inv.DeclUntracked && it.Declared != inv.DeclMissing {
			continue
		}
		if m.updatesOnly && !it.Updatable() {
			continue
		}
		mt := match{item: it}
		if q != "" {
			if found := fuzzy.Find(q, []string{it.Name}); len(found) > 0 {
				mt.matches, mt.score = found[0].MatchedIndexes, found[0].Score+1000
			} else if strings.Contains(strings.ToLower(it.SearchText()), strings.ToLower(q)) {
				mt.score = 0
			} else {
				continue
			}
		}
		out = append(out, mt)
	}
	return out
}

// rows groups and orders the current tab's items for display.
func (m Model) rows() []row {
	k := m.kind()
	items := m.filtered(k)
	less := m.lessFunc()
	if m.group == groupFlat || len(items) == 0 {
		sort.SliceStable(items, func(i, j int) bool { return less(items[i], items[j]) })
		out := make([]row, len(items))
		for i, mt := range items {
			out[i] = row{item: mt.item, matches: mt.matches}
		}
		return out
	}
	groups := map[string][]match{}
	var keys []string
	for _, mt := range items {
		key := mt.item.Category
		if m.group == groupSource {
			key = sourceLabel[mt.item.Source]
			if mt.item.Declared == inv.DeclMissing {
				key = "Missing"
			}
		}
		if _, ok := groups[key]; !ok {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], mt)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if m.group == groupPurpose {
			ra, rb := groupRank(k, a), groupRank(k, b)
			if ra != rb {
				return ra < rb
			}
		} else if len(groups[a]) != len(groups[b]) {
			return len(groups[a]) > len(groups[b])
		}
		return strings.ToLower(a) < strings.ToLower(b)
	})
	var out []row
	for _, key := range keys {
		g := groups[key]
		sort.SliceStable(g, func(i, j int) bool { return less(g[i], g[j]) })
		out = append(out, row{title: key, count: len(g)})
		for _, mt := range g {
			out = append(out, row{item: mt.item, matches: mt.matches})
		}
	}
	return out
}

// groupRank orders purpose groups, keeping dependencies and missing items at the bottom.
func groupRank(k inv.Kind, cat string) int {
	switch cat {
	case "Dependencies":
		return 2000
	case "Missing":
		return 3000
	}
	return inv.CategoryRank(k, cat)
}

func (m Model) lessFunc() func(a, b match) bool {
	byName := func(a, b match) bool { return strings.ToLower(a.item.Name) < strings.ToLower(b.item.Name) }
	q := strings.TrimSpace(m.search.Value())
	return func(a, b match) bool {
		if q != "" && a.score != b.score && m.group == groupFlat {
			return a.score > b.score
		}
		switch m.sort {
		case sortNewest:
			if !a.item.InstalledAt.Equal(b.item.InstalledAt) {
				return a.item.InstalledAt.After(b.item.InstalledAt)
			}
		case sortLastUsed:
			ua, ub := usage(a.item), usage(b.item)
			if ua != ub {
				return ua > ub
			}
		}
		return byName(a, b)
	}
}

// usage ranks recency for apps and run counts for CLIs on one comparable scale.
func usage(it *inv.Item) int64 {
	if !it.LastUsed.IsZero() {
		return it.LastUsed.Unix()
	}
	return int64(it.ShellRuns)
}

// tabCount is the number of visible items in a tab under the current filters.
func (m Model) tabCount(k inv.Kind) int { return len(m.filtered(k)) }
