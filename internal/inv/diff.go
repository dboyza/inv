package inv

// ChangeKind classifies a difference between two builds.
type ChangeKind int

const (
	Added ChangeKind = iota
	Removed
	Changed // version moved, e.g. after an upgrade
)

// Change is one install appearing, disappearing, or changing version between scans.
type Change struct {
	Kind ChangeKind
	Item Item
	From string // previous version for Changed
}

// Diff reports user-visible changes; dependencies and declared-but-missing placeholders are ignored.
func Diff(before, after []Item) []Change {
	present := func(items []Item) map[string]Item {
		m := map[string]Item{}
		for _, it := range items {
			if !it.Dependency && it.Declared != DeclMissing {
				m[it.ID] = it
			}
		}
		return m
	}
	old, cur := present(before), present(after)
	var out []Change
	for _, it := range after {
		prev, ok := old[it.ID]
		switch {
		case it.Dependency || it.Declared == DeclMissing:
		case !ok:
			out = append(out, Change{Kind: Added, Item: it})
		case prev.Version != it.Version && prev.Version != "" && it.Version != "":
			out = append(out, Change{Kind: Changed, Item: it, From: prev.Version})
		}
	}
	for _, it := range before {
		if _, ok := cur[it.ID]; !ok && !it.Dependency && it.Declared != DeclMissing {
			out = append(out, Change{Kind: Removed, Item: it})
		}
	}
	return out
}
