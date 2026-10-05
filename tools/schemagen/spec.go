// Registration API for tools/schemagen (contract cli.json-schemas, P7-REG-08).
//
// Purpose: every JSON Schema under schemas/ is generated from a Go type that is
// registered here, never hand-written. Later Epics add one tools/schemagen/<topic>.go
// file whose init() calls Register; they never edit spec.go, builtin.go or
// commands.go.
//
// Inputs: Spec values passed to Register from init().
// Outputs: the registered set (sorted by Out) and a list of generator problems.
// Constraints: Register never panics. A duplicate Out or an Out that does not
// match outPattern is recorded as a problem; generation and -check then fail.
package main

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"sync"
)

// Override edits one node of the inferred schema.
type Override struct {
	// Pointer is a JSON Pointer into the inferred schema ("" is the root),
	// e.g. "/properties/class".
	Pointer string
	// Set holds keys merged (shallow, replacing) into that node: enum,
	// pattern, oneOf, const, description and so on. Values come from exported
	// constants, never from duplicated literals.
	Set map[string]any
}

// Spec registers one generated schema.
type Spec struct {
	// Out is the path under schemas/, matching outPattern.
	Out string
	// Type is the zero value of the Go type, e.g. manifestv2.Manifest{}.
	Type any
	// Overrides are applied in order after inference.
	Overrides []Override
}

// outPattern is the only accepted shape of Spec.Out.
var outPattern = regexp.MustCompile(`^[a-z0-9/_-]+\.v[0-9]+\.schema\.json$`)

// typeOverrides holds overrides keyed by Go type; build applies them before a
// spec's own Overrides, so every schema generated from the same type (for
// example command-registry and the `help` data schema) shares them.
var typeOverrides = map[reflect.Type][]Override{}

// RegisterTypeOverrides attaches overrides to every schema generated from the
// type of zero. Call it only from init().
func RegisterTypeOverrides(zero any, ovs ...Override) {
	t := reflect.TypeOf(zero)
	typeOverrides[t] = append(typeOverrides[t], ovs...)
}

// specSet is a registration set. The package keeps one default set; tests
// build their own.
type specSet struct {
	mu       sync.Mutex
	specs    map[string]Spec
	problems []string
}

func newSpecSet() *specSet { return &specSet{specs: map[string]Spec{}} }

// defaultSet is the set Register writes to.
var defaultSet = newSpecSet()

// Register records s in the default set. Call it only from init().
func Register(s Spec) { defaultSet.register(s) }

// register records s or a problem; it never panics.
func (ss *specSet) register(s Spec) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	switch {
	case !outPattern.MatchString(s.Out):
		ss.problems = append(ss.problems, fmt.Sprintf("registration %q: Out must match %s", s.Out, outPattern))
	case s.Type == nil:
		ss.problems = append(ss.problems, fmt.Sprintf("registration %q: Type is nil", s.Out))
	default:
		if _, dup := ss.specs[s.Out]; dup {
			ss.problems = append(ss.problems, fmt.Sprintf("registration %q: duplicate Out", s.Out))
			return
		}
		ss.specs[s.Out] = s
	}
}

// sorted returns the registered specs ordered by Out.
func (ss *specSet) sorted() []Spec {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	out := make([]Spec, 0, len(ss.specs))
	for _, s := range ss.specs {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Out < out[j].Out })
	return out
}

// problemList returns a copy of the recorded registration problems.
func (ss *specSet) problemList() []string {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return append([]string(nil), ss.problems...)
}
