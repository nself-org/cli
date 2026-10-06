package commands

// Process-wide command registry for the invocation decorator.
//
// Purpose: the decorator needs the registry entry of the command being run
// (does it support --json, which flag overrides apply). Building it parses
// the embedded canon.yaml and walks the whole cobra tree, so it is built
// lazily, once per compat mode, and only when --json (or --format json) is
// actually in play. A run without --json never reaches this file.
//
// Inputs: RootCmd (read only), the embedded canon data, jsonDataTypes.
//
// Outputs: a memoised *cmdregistry.Registry per compat mode.
//
// Constraints: the registry is always built from a tree on which
// ApplyCommandGroups and InitDefaultHelpCmd have run, so `help` resolves and
// the group field is final. A build failure is returned, never turned into a
// default: a command with no entry must not silently print text in JSON mode.

import (
	"fmt"
	"sync"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/compat"
)

// canonLoad is the canon loader. A variable so tests can count calls and prove
// the decorator never parses canon.yaml when --json is absent.
var canonLoad = canon.Effective

// regCache memoises one build result per compat mode. The mode is part of the
// key because BuildOptions.V15 changes the reported exit codes and which
// V15-only envelopes are visible, and tests flip the mode within one process.
var regCache struct {
	mu   sync.Mutex
	done map[bool]*regResult
}

type regResult struct {
	reg *cmdregistry.Registry
	err error
}

// commandRegistry returns the registry of the live RootCmd tree for the
// current compat mode, building it on first use.
func commandRegistry() (*cmdregistry.Registry, error) {
	// compat.V15(P7-REG-05): registry reports v1.4 exit codes and hides v1.5-only envelopes -> v1.5 exit codes and envelopes
	v15 := compat.V15()

	regCache.mu.Lock()
	defer regCache.mu.Unlock()
	if r, ok := regCache.done[v15]; ok {
		return r.reg, r.err
	}
	if regCache.done == nil {
		regCache.done = map[bool]*regResult{}
	}
	r := &regResult{}
	r.reg, r.err = buildRegistry(v15)
	regCache.done[v15] = r
	return r.reg, r.err
}

// buildRegistry runs the idempotent tree preparation and Build.
func buildRegistry(v15 bool) (*cmdregistry.Registry, error) {
	ApplyCommandGroups()
	// cobra adds `help` only inside Execute; canon.yaml has an entry for it, so
	// the tree must have the command before Build checks completeness.
	RootCmd.InitDefaultHelpCmd()
	c, err := canonLoad(v15)
	if err != nil {
		return nil, fmt.Errorf("command canon: %w", err)
	}
	reg, err := cmdregistry.Build(RootCmd, c, jsonDataTypes, cmdregistry.BuildOptions{
		V15:             v15,
		V15OnlyEnvelope: jsonV15OnlyEnvelope,
	})
	if err != nil {
		return nil, fmt.Errorf("command registry: %w", err)
	}
	return reg, nil
}

// resetRegistryCache drops the memoised registries (tests only).
func resetRegistryCache() {
	regCache.mu.Lock()
	defer regCache.mu.Unlock()
	regCache.done = nil
}
