package main

import (
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
)

func TestTopLevelSelection(t *testing.T) {
	grp := "g"
	c := func(path, name, parent, cn string, hidden bool) cmdregistry.Command {
		return cmdregistry.Command{Path: path, Name: name, Parent: parent, Canon: cn, Hidden: hidden}
	}
	reg := &cmdregistry.Registry{Commands: []cmdregistry.Command{
		c("nself start", "start", "nself", canon.CanonCore, false),
		c("nself acme", "acme", "nself", canon.CanonPlugin, false),
		c("nself old", "old", "nself", canon.CanonShim, true),
		c("nself help", "help", "nself", canon.CanonBuiltin, false),
		c("nself db", "db", "nself", canon.CanonCore, false),
		c("nself db seed", "seed", "nself db", canon.CanonSubcommand, false),
	}}
	reg.Commands[4].Group = &grp
	got := topLevel(reg)
	if len(got) != 2 || got[0].Name != "db" || got[0].GroupID != "g" || got[1].Name != "start" {
		t.Fatalf("topLevel = %+v", got)
	}
}
