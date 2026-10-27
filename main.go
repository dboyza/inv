// Command inv shows every custom install on this Mac in a live terminal UI.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/dboyza/inv/internal/inv"
	"github.com/dboyza/inv/internal/ui"
)

var version = "dev"

func main() {
	asJSON := flag.Bool("json", false, "print the inventory as JSON and exit")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("inv", version)
		return
	}
	env := inv.NewEnv()
	if *asJSON {
		snap := inv.Collect(env, inv.Snapshot{}, inv.AllGroups)
		snap = inv.CheckUpdates(env, inv.ProbeVersions(env, snap))
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(inv.Build(env, snap)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if _, err := tea.NewProgram(ui.New(env, version)).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "inv:", err)
		os.Exit(1)
	}
}
