package report

// Tests for fix round 4 (CR r3): a symbol-only tail behind a known literal,
// every fragment env_file root, interpolation against the env inputs, partial
// log output at the deadline, structure-name spelling drop, short-note scope.
// Every fake secret is built at run time; HOME is isolated (main_test.go).

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// S-1: a tail with no letter or digit is still a tail.
func TestScrubKnownLiteralDoesNotShieldASymbolTail(t *testing.T) {
	u := "u" + rep("Kz", 3)
	for _, tail := range []string{"!@#$%^", "+/=+/=", "_-_-_-", "!!!!!!"} {
		in := "password=" + u + tail + "\n"
		if out := NewScrubber(ExpandSecretValues("BASIC_AUTH_USER", u)).Scrub(in); strings.Contains(out, tail) {
			t.Errorf("tail %q survived behind the literal: %q", tail, out)
		}
		if out := NewScrubber(nil).Scrub(in); strings.Contains(out, tail) { // control
			t.Errorf("control leaked %q: %q", tail, out)
		}
	}
	both := "password=" + u + ":" + u + "\n" // two placeholders and a separator stay as they are
	if out := NewScrubber(ExpandSecretValues("BASIC_AUTH_USER", u)).Scrub(both); out != "password=[REDACTED:BASIC_AUTH_USER]:[REDACTED:BASIC_AUTH_USER]\n" {
		t.Errorf("placeholder pair rewritten: %q", out)
	}
}

// S-2: project dir and fragment dir both hold an env_file of that name; both
// are read.
func TestDoctorReportFragmentEnvFileReadsEveryRoot(t *testing.T) {
	proj, fragDir := t.TempDir(), t.TempDir()
	pv, fv := "pj"+rep("Ab3", 8), "fr"+rep("Cd4", 8)
	writeFile(t, filepath.Join(proj, "app.env"), "PROJ_SECRET="+pv+"\n")
	writeFile(t, filepath.Join(fragDir, "app.env"), "FRAG_SECRET="+fv+"\n")
	frag := filepath.Join(fragDir, "docker-compose.plugin.yml")
	writeFile(t, frag, "services:\n  p:\n    image: x\n    env_file: app.env\n")
	logs := map[string]string{"p": "a " + pv + " b " + fv + " c\n"}
	files := collectFiles(t, Options{ProjectDir: proj, ComposeFragments: []string{frag}, Environ: noEnviron}, logsSource(logs))
	assertAbsent(t, files, pv, fv)
}

// S-3: a secret-named compose key that references variables is interpolated
// against the env inputs; the referenced values are learned under any name.
func TestDoctorReportComposeInterpolatedAgainstEnv(t *testing.T) {
	dir := t.TempDir()
	head, suf := "hd"+rep("Ef5", 6), "sf"+rep("Gh6", 6)
	plain, mixed := "pl"+rep("Ij7", 8), "mx"+rep("Kl8", 6)
	writeFile(t, filepath.Join(dir, ".env"), "SUF_SET="+suf+"\nPLAIN_VALUE="+plain+"\nMIX_PART="+mixed+"\n")
	writeFile(t, filepath.Join(dir, "docker-compose.yml"), strings.Join([]string{
		"services:", "  a:", "    image: x", "    environment:",
		`      ADMIN_SECRET: "` + head + `${UNSET_SUF}"`,
		`      OTHER_SECRET: "` + head + `x${SUF_SET}"`,
		"      THIRD_SECRET: ${PLAIN_VALUE}",
		`      FOURTH_TOKEN: "$MIX_PART-tail"`,
		"",
	}, "\n"))
	want := []string{head, head + "x" + suf, suf, plain, mixed + "-tail", mixed}
	files := collectFiles(t, Options{ProjectDir: dir, Environ: noEnviron}, logsSource(map[string]string{"a": "saw " + strings.Join(want, " | ") + " done\n"}))
	assertAbsent(t, files, want...)
	if !strings.Contains(files["logs/a.log"], " done") {
		t.Errorf("log over-redacted: %q", files["logs/a.log"])
	}
}

// S-5: partial output at the deadline (docker logs killed, no error) is noted.
func TestReadContainerLogPartialOutputAtDeadlineIsANote(t *testing.T) {
	var notes []string
	fetch := func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "partial\n", nil
	}
	got := readContainerLog(context.Background(), 20*time.Millisecond, "c1", func(s string) { notes = append(notes, s) }, fetch)
	if !strings.Contains(got, "timed out") || len(notes) != 1 {
		t.Errorf("text %q notes %v", got, notes)
	}
}

// N-1: only the structure-equal spelling of a compose-only value goes; the
// other spellings stay. R9: the same value from an env input is never dropped.
func TestDropStructureValuesKeepsOtherSpellings(t *testing.T) {
	in := newInputSet(t.TempDir())
	in.learnAs("POSTGRES_PASSWORD", "postgres", true) // a compose default (R9)
	in.addStructure("Postgres")
	if n := in.dropStructureValues(); n != 1 {
		t.Fatalf("dropped = %d, want 1", n)
	}
	if len(in.values) != 1 || len(in.values[0].Variants) == 0 {
		t.Fatalf("the other spellings were dropped: %+v", in.values)
	}
	for _, v := range in.values[0].Variants {
		if v == "postgres" {
			t.Error("the structure spelling was kept")
		}
	}
	env := newInputSet(t.TempDir())
	env.learn("POSTGRES_PASSWORD", "postgres")
	env.addStructure("Postgres")
	if n := env.dropStructureValues(); n != 0 || len(env.values) != 1 || env.values[0].Variants[0] != "postgres" {
		t.Errorf("an env-given value was dropped: n=%d %+v", n, env.values)
	}
	if env.envStructureNote() == "" {
		t.Error("no manifest note for an env value equal to a structure name")
	}
}

// N-2: short secret-named values of non-cascade files are not named in the
// manifest; those of the cascade are.
func TestDoctorReportShortNoteSkipsNonCascadeFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env"), "CASCADE_API_TOKEN=ab\n")
	writeFile(t, filepath.Join(dir, ".env.demo"), "DEMOONLY_API_KEY=cd\n")
	files := collectFiles(t, Options{ProjectDir: dir, Environ: noEnviron}, logsSource(nil))
	if !strings.Contains(files["manifest.json"], "CASCADE_API_TOKEN") {
		t.Error("cascade short value not noted")
	}
	if strings.Contains(files["manifest.json"], "DEMOONLY_API_KEY") {
		t.Error("a non-cascade file's variable name reached the manifest")
	}
}

// S-3: the interpolated whole is learned even when every referenced value is
// too short to be a literal on its own, and a literal piece between two
// references is learned on its own.
func TestDoctorReportComposeInterpolationShortPartsAndPieces(t *testing.T) {
	dir := t.TempDir()
	piece, piece2 := "pc"+rep("Mn9", 5), "pd"+rep("Qr3", 5)
	writeFile(t, filepath.Join(dir, ".env"), "PART_A=ab1\nPART_B=cd2\n")
	writeFile(t, filepath.Join(dir, "docker-compose.yml"), strings.Join([]string{
		"services:", "  a:", "    image: x", "    environment:",
		`      JOINED_SECRET: "${PART_A}-${PART_B}"`,
		`      PIECE_SECRET: "${NOPE_ONE}` + piece + `${NOPE_TWO}` + piece2 + `"`,
		"",
	}, "\n"))
	// Each literal piece is learned on its own: a log may carry just one.
	files := collectFiles(t, Options{ProjectDir: dir, Environ: noEnviron}, logsSource(map[string]string{"a": "saw ab1-cd2 and " + piece + " done\n"}))
	assertAbsent(t, files, "ab1-cd2", piece)
}
