// Command manifestv2migrate converts, regenerates and checks plugin.json files
// for manifest v2 (P7-PLUG-01).
//
// Purpose: the one codemod for the v1 to v2 migration. It reads a plugin.json
// through internal/plugin/manifestv2 (v1 through the one normalizer) and prints
// the canonical v2 file: keys sorted, 2-space indent, trailing newline.
//
// Inputs: -in <plugin.json>. Modes: default (print canonical v2), -compat
// (rewrite only the generated compatibility keys of a v2 file), -check (exit 1
// unless the file is canonical v2 with current compatibility keys), -targets
// (print "binary<TAB>command" for every binary the installer publishes, one line each), -wiki <page> (regenerate
// the field table of the Plugin-Manifest wiki page from the schema). -write
// stores the result back into -in (temp file + rename) instead of printing it.
//
// A v1 file is never converted lossily. Every non-empty v1 key with no v2 home
// (config, hooks, actions, routes, notes, api_version, entry, ...) is listed on
// stderr by value path and the run exits 1 with nothing written and nothing on
// stdout, in default, -write and -check modes alike. Registry-owned keys
// (bundles, checksum, tier_pair, ...) are removed on purpose and reported as a
// note. The one way to drop data is -drop <file>: a reviewed list of dead keys
// (tools/manifestv2migrate/dead-keys.txt, keys no code reads), one per line with
// a comment naming the evidence. Only listed keys are dropped; default and
// -check runs say what would be dropped, -write says what was. Anything neither
// mapped nor listed still refuses. A routes, capabilities, env or env_vars value
// converts only when it does so losslessly (routes to rest_routes, capabilities
// to the v2 list, env to env {required, optional}); otherwise it stays refused.
//
// Outputs: stdout, or the file with -write. Exit: 0 ok, 1 invalid or not
// canonical, 2 usage or I/O failure. Running it on its own output is a no-op.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func say(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }

// run executes one invocation and returns the exit code.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("manifestv2migrate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	in := fs.String("in", "", "plugin.json to read")
	write := fs.Bool("write", false, "write the result back to -in")
	compatOnly := fs.Bool("compat", false, "rewrite only the compatibility keys of a v2 file")
	check := fs.Bool("check", false, "exit 1 unless -in is canonical v2")
	targets := fs.Bool("targets", false, "print binary<TAB>command for the commands block")
	drop := fs.String("drop", "", "file of dead v1 keys (one per line, # comments) that may be dropped")
	wiki := fs.String("wiki", "", "regenerate the field table of this wiki page from -schema")
	schema := fs.String("schema", "schemas/plugin-manifest.v2.schema.json", "schema read by -wiki")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *wiki != "" {
		return runWiki(*wiki, *schema, stderr)
	}
	if *in == "" {
		say(stderr, "manifestv2migrate: -in is required\n")
		return 2
	}
	data, err := os.ReadFile(*in)
	if err != nil {
		say(stderr, "manifestv2migrate: %v\n", err)
		return 2
	}
	switch {
	case *targets:
		return runTargets(data, stdout, stderr)
	case !*compatOnly && refuseLossy(*in, data, *drop, *write, stderr):
		return 1
	case *check:
		return runCheck(*in, data, stderr)
	}
	var out []byte
	if *compatOnly {
		out, err = manifestv2.ApplyCompat(data)
	} else {
		out, err = convert(data)
	}
	if err != nil {
		say(stderr, "manifestv2migrate: %s: %v\n", *in, err)
		return 1
	}
	if !*write {
		_, _ = stdout.Write(out)
		return 0
	}
	if err := writeAtomic(*in, out); err != nil {
		say(stderr, "manifestv2migrate: %v\n", err)
		return 2
	}
	return 0
}

// convert returns the canonical v2 bytes of a v1 or v2 plugin.json.
// A v1 file the released CLI accepts may lack keys v2 requires (category); the
// normalizer allows that, the tool does not write an invalid v2 file.
func convert(data []byte) ([]byte, error) {
	m, err := manifestv2.ParseQuiet(data)
	if err != nil {
		return nil, err
	}
	if err := manifestv2.Validate(m); err != nil {
		return nil, err
	}
	return manifestv2.Marshal(m)
}

// refuseLossy reports v1 data that has no v2 home and returns true when the
// run must stop. Keys listed in the -drop file are reported as dropped (or as
// would-drop unless writing) and do not stop it. Only v1 input is checked: a v2
// file decodes strictly, so an unknown key there already fails with E106.
func refuseLossy(path string, data []byte, dropFile string, writing bool, stderr io.Writer) bool {
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return false
	}
	if v, ok := raw["manifest_version"]; ok && !manifestv2.IsV1Version(v) {
		return false
	}
	dead, err := loadDropList(dropFile)
	if err != nil {
		say(stderr, "manifestv2migrate: %v\n", err)
		return true
	}
	if keys := manifestv2.RegistryKeys(data); len(keys) > 0 {
		say(stderr, "manifestv2migrate: %s: note: registry-owned key(s) removed on purpose (ADR 0008, release pipeline): %s\n", path, strings.Join(keys, ", "))
	}
	un, err := manifestv2.UnmappedV1Keys(data)
	if err != nil {
		return false
	}
	var drops, refused []manifestv2.Unmapped
	for _, u := range un {
		if dead[u.Key] {
			drops = append(drops, u)
		} else {
			refused = append(refused, u)
		}
	}
	if len(drops) > 0 {
		verb := "would drop"
		if writing && len(refused) == 0 {
			verb = "dropping"
		}
		say(stderr, "manifestv2migrate: %s: %s %d dead v1 key%s listed in %s (no code reads them):\n", path, verb, len(drops), plural(len(drops)), dropFile)
		for _, u := range drops {
			say(stderr, "  %s\n", u)
		}
	}
	if len(refused) == 0 {
		return false
	}
	say(stderr, "manifestv2migrate: %s: refusing: %d v1 key%s with no v2 home would be dropped (nothing written):\n", path, len(refused), plural(len(refused)))
	for _, u := range refused {
		say(stderr, "  %s\n", u)
	}
	return true
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func runCheck(path string, data []byte, stderr io.Writer) int {
	m, err := manifestv2.DecodeV2(data)
	if err == nil {
		err = manifestv2.CheckCompat(m)
	}
	if err != nil {
		say(stderr, "manifestv2migrate: %s: %v\n", path, err)
		return 1
	}
	want, err := manifestv2.Marshal(m)
	if err != nil || !bytes.Equal(want, data) {
		say(stderr, "manifestv2migrate: %s is not canonical v2 (run with -write)\n", path)
		return 1
	}
	return 0
}

func runTargets(data []byte, stdout, stderr io.Writer) int {
	m, err := manifestv2.ParseQuiet(data)
	if err != nil {
		say(stderr, "manifestv2migrate: %v\n", err)
		return 1
	}
	for _, t := range manifestv2.CLITargetsOf(m) {
		say(stdout, "%s\t%s\n", t.Binary, t.Command)
	}
	return 0
}

// writeAtomic replaces path through a temp file in the same directory.
func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".manifestv2-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if info, err := os.Stat(path); err == nil {
		_ = os.Chmod(tmp.Name(), info.Mode())
	}
	return os.Rename(tmp.Name(), path)
}
