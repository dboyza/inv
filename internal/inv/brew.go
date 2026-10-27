package inv

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"
)

// BrewData is the installed Homebrew state.
type BrewData struct {
	Formulae []BrewFormula `json:"formulae"`
	Casks    []BrewCask    `json:"casks"`
}

type BrewFormula struct {
	Name      string   `json:"name"`
	Tap       string   `json:"tap"`
	Desc      string   `json:"desc"`
	Homepage  string   `json:"homepage"`
	Version   string   `json:"version"`
	OnRequest bool     `json:"on_request"`
	Time      int64    `json:"time"`
	Deps      []string `json:"deps"`
	Bins      []string `json:"bins"`
	Keg       string   `json:"keg"`
}

type BrewCask struct {
	Token       string   `json:"token"`
	Name        string   `json:"name"`
	Desc        string   `json:"desc"`
	Homepage    string   `json:"homepage"`
	Installed   string   `json:"installed"`
	Time        int64    `json:"time"`
	AutoUpdates bool     `json:"auto_updates"`
	Apps        []string `json:"apps"`
	Binaries    []string `json:"binaries"`
	Fonts       []string `json:"fonts"`
}

// brewInfo mirrors the subset of `brew info --json=v2 --installed` we read.
type brewInfo struct {
	Formulae []struct {
		Name      string `json:"name"`
		Tap       string `json:"tap"`
		Desc      string `json:"desc"`
		Homepage  string `json:"homepage"`
		LinkedKeg string `json:"linked_keg"`
		Installed []struct {
			Version            string `json:"version"`
			InstalledOnRequest bool   `json:"installed_on_request"`
			Time               int64  `json:"time"`
			RuntimeDeps        []struct {
				FullName         string `json:"full_name"`
				DeclaredDirectly bool   `json:"declared_directly"`
			} `json:"runtime_dependencies"`
		} `json:"installed"`
	} `json:"formulae"`
	Casks []struct {
		Token         string            `json:"token"`
		Name          []string          `json:"name"`
		Desc          string            `json:"desc"`
		Homepage      string            `json:"homepage"`
		Installed     string            `json:"installed"`
		InstalledTime int64             `json:"installed_time"`
		AutoUpdates   bool              `json:"auto_updates"`
		Artifacts     []json.RawMessage `json:"artifacts"`
	} `json:"casks"`
}

func collectBrew(env *Env) (BrewData, error) {
	var d BrewData
	if env.BrewPrefix == "" {
		return d, nil
	}
	out, err := run(60*time.Second, filepath.Join(env.BrewPrefix, "bin", "brew"), "info", "--json=v2", "--installed")
	if err != nil {
		return d, err
	}
	var info brewInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return d, err
	}
	for _, f := range info.Formulae {
		if len(f.Installed) == 0 {
			continue
		}
		in := f.Installed[0]
		bf := BrewFormula{Name: f.Name, Tap: f.Tap, Desc: f.Desc, Homepage: f.Homepage, Version: in.Version,
			OnRequest: in.InstalledOnRequest, Time: in.Time, Keg: f.LinkedKeg}
		for _, dep := range in.RuntimeDeps {
			if dep.DeclaredDirectly {
				bf.Deps = append(bf.Deps, dep.FullName[strings.LastIndex(dep.FullName, "/")+1:])
			}
		}
		keg := f.LinkedKeg
		if keg == "" {
			keg = in.Version
		}
		bf.Bins = readDirNames(filepath.Join(env.BrewPrefix, "Cellar", f.Name, keg, "bin"))
		d.Formulae = append(d.Formulae, bf)
	}
	for _, c := range info.Casks {
		bc := BrewCask{Token: c.Token, Desc: c.Desc, Homepage: c.Homepage, Installed: c.Installed,
			Time: c.InstalledTime, AutoUpdates: c.AutoUpdates}
		if len(c.Name) > 0 {
			bc.Name = c.Name[0]
		}
		for _, raw := range c.Artifacts {
			var art map[string]json.RawMessage
			if json.Unmarshal(raw, &art) != nil {
				continue
			}
			bc.Apps = append(bc.Apps, firstStrings(art["app"])...)
			bc.Binaries = append(bc.Binaries, firstStrings(art["binary"])...)
			bc.Fonts = append(bc.Fonts, firstStrings(art["font"])...)
		}
		d.Casks = append(d.Casks, bc)
	}
	return d, nil
}

// firstStrings reads cask artifact lists, which mix plain strings with option objects.
func firstStrings(raw json.RawMessage) []string {
	var parts []json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &parts) != nil {
		return nil
	}
	var out []string
	for _, p := range parts {
		var s string
		if json.Unmarshal(p, &s) == nil {
			out = append(out, s)
		}
	}
	return out
}
