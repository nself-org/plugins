package main

// Purpose: `link <local-path>` and `unlink <name>`: maintain the plugin shadow
// registry ~/.nself/plugin-links.json. Port of core cmd/commands/plugin_link.go.
// Inputs:  a plugin directory (link) or a plugin name (unlink); --host, --list.
// Outputs: the registry file (0600) and the same status lines as core.
// Constraints: only ~/.nself/plugin-links.json is written; link only records
// the directory it was given, which must hold a plugin.yaml.

import (
	"github.com/nself-org/nself-plugin-dev/internal/clui"
	"github.com/nself-org/nself-plugin-dev/internal/links"
)

func runLink(p *parsed) error {
	if p.boolean("list") {
		return runLinkList()
	}

	absPath, err := links.ValidatePath(p.args[0])
	if err != nil {
		return err
	}
	name, err := links.ResolveName(absPath)
	if err != nil {
		return err
	}
	reg, err := links.Load()
	if err != nil {
		return err
	}
	reg[name] = absPath
	if err := links.Save(reg); err != nil {
		return err
	}

	mode := "container-mount"
	if p.boolean("host") {
		mode = "host-process"
	}
	clui.Successf("Linked %s -> %s (%s mode)", name, absPath, mode)
	clui.Dimmed("Run `nself build` to activate the local version.")
	clui.Dimmedf("Run `nself plugin unlink %s` to restore the registry version.", name)
	return nil
}

func runUnlink(p *parsed) error {
	name := p.args[0]
	reg, err := links.Load()
	if err != nil {
		return err
	}
	if _, ok := reg[name]; !ok {
		clui.Infof("Plugin %q is not linked (no-op).", name)
		return nil
	}
	delete(reg, name)
	if err := links.Save(reg); err != nil {
		return err
	}
	clui.Successf("Unlinked %s. Run `nself build` to use the registry version.", name)
	return nil
}

// runLinkList prints the linked plugins, sorted by name (core ranges over a
// map, so its order is arbitrary; sorted is one of the orders core can print).
func runLinkList() error {
	reg, err := links.Load()
	if err != nil {
		return err
	}
	if len(reg) == 0 {
		clui.Info("No plugins currently linked.")
		return nil
	}
	tbl := clui.NewTable("Name", "Local Path")
	for _, name := range sortedKeys(reg) {
		tbl.AddRow(name, reg[name])
	}
	tbl.Render()
	return nil
}
