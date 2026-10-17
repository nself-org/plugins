package main

// Purpose: `link <local-path>` and `unlink <name>`: maintain the plugin shadow
// registry ~/.nself/plugin-links.json. Port of core cmd/commands/plugin_link.go.
// Inputs:  a plugin directory (link) or a plugin name (unlink); --host, --list.
// Outputs: the registry file (0600) and the same status lines as core.
// Constraints: only ~/.nself/plugin-links.json is written; link only records
// the directory it was given, which must hold a plugin.yaml.

import (
	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/nself-plugin-dev/internal/clui"
	"github.com/nself-org/nself-plugin-dev/internal/links"
)

// linkData is the --json data of `link`; linkListData of `link --list`;
// unlinkData of `unlink`.
type linkData struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Mode string `json:"mode"`
}

type linkEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type linkListData struct {
	Links []linkEntry `json:"links"`
}

type unlinkData struct {
	Name     string `json:"name"`
	Unlinked bool   `json:"unlinked"`
}

// linksIOErr is E774: the registry file could not be read or written.
func linksIOErr(err error) error {
	return coded(err, CodeLinksIO, output.ClassInfra, "the plugin links file (~/.nself/plugin-links.json) could not be read or written",
		"check the file is valid JSON and that ~/.nself is writable")
}

// notPluginDirErr is E771: the path is not a directory that holds a plugin.yaml.
func notPluginDirErr(err error) error {
	return coded(err, CodeNotPluginDir, output.ClassUser, "the path is not a plugin directory: it must exist and hold a plugin.yaml",
		"run the command with the plugin's source directory")
}

func runLink(p *parsed) (any, error) {
	if p.boolean("list") {
		return runLinkList()
	}

	absPath, err := links.ValidatePath(p.args[0])
	if err != nil {
		return nil, notPluginDirErr(err)
	}
	name, err := links.ResolveName(absPath)
	if err != nil {
		return nil, notPluginDirErr(err)
	}
	reg, err := links.Load()
	if err != nil {
		return nil, linksIOErr(err)
	}
	reg[name] = absPath
	if err := links.Save(reg); err != nil {
		return nil, linksIOErr(err)
	}

	mode := "container-mount"
	if p.boolean("host") {
		mode = "host-process"
	}
	clui.Successf("Linked %s -> %s (%s mode)", name, absPath, mode)
	clui.Dimmed("Run `nself build` to activate the local version.")
	clui.Dimmedf("Run `nself plugin unlink %s` to restore the registry version.", name)
	return linkData{Name: name, Path: absPath, Mode: mode}, nil
}

func runUnlink(p *parsed) (any, error) {
	name := p.args[0]
	reg, err := links.Load()
	if err != nil {
		return nil, linksIOErr(err)
	}
	if _, ok := reg[name]; !ok {
		clui.Infof("Plugin %q is not linked (no-op).", name)
		return unlinkData{Name: name, Unlinked: false}, nil
	}
	delete(reg, name)
	if err := links.Save(reg); err != nil {
		return nil, linksIOErr(err)
	}
	clui.Successf("Unlinked %s. Run `nself build` to use the registry version.", name)
	return unlinkData{Name: name, Unlinked: true}, nil
}

// runLinkList prints the linked plugins, sorted by name (core ranges over a
// map, so its order is arbitrary; sorted is one of the orders core can print).
func runLinkList() (any, error) {
	reg, err := links.Load()
	if err != nil {
		return nil, linksIOErr(err)
	}
	entries := []linkEntry{}
	for _, name := range sortedKeys(reg) {
		entries = append(entries, linkEntry{Name: name, Path: reg[name]})
	}
	if len(reg) == 0 {
		clui.Info("No plugins currently linked.")
		return linkListData{Links: entries}, nil
	}
	tbl := clui.NewTable("Name", "Local Path")
	for _, e := range entries {
		tbl.AddRow(e.Name, e.Path)
	}
	tbl.Render()
	return linkListData{Links: entries}, nil
}
