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
// (print "binary<TAB>command" for the commands block), -wiki <page> (regenerate
// the field table of the Plugin-Manifest wiki page from the schema). -write
// stores the result back into -in (temp file + rename) instead of printing it.
//
// Outputs: stdout, or the file with -write. Exit: 0 ok, 1 invalid or not
// canonical, 2 usage or I/O failure. Running it on its own output is a no-op.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

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
	case *check:
		return runCheck(*in, data, stderr)
	case *targets:
		return runTargets(data, stdout, stderr)
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
func convert(data []byte) ([]byte, error) {
	m, err := manifestv2.ParseQuiet(data)
	if err != nil {
		return nil, err
	}
	return manifestv2.Marshal(m)
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
	if m.Commands != nil {
		say(stdout, "%s\t%s\n", m.Commands.Binary, m.Commands.Command)
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
