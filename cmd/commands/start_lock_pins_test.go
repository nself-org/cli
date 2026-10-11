package commands

// start_lock_pins_test.go — which env variables may drop a project lock pin
// (P7-LIVE-09, round 8; the class behind four rounds of one-variable fixes).
//
// Purpose: for EVERY image the generator renders (compose.ImageInputs, the one
// table) and every variable of its row, assert the pin outcome in each env state
// -- *_IMAGE only, *_VERSION only, both, neither, and variables the generator
// ignores -- both on the build path (the real generator with the pins installed)
// and on the --frozen path (checkStartLock against the compose that build wrote).
// Inputs: compose.ImageInputs and rowEnv (what makes a row render).
// Outputs: failures naming mode, service/entry and env state.
// Constraints: the expected outcome comes from rendering, not from the table's
// order: a pin is kept when no variable the generator reads is set, or when the
// image the generator really renders equals the pin; the variable named in the
// notice is the one whose lone render equals the full render.

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/lock"
	"github.com/nself-org/cli/internal/version"
)

const (
	matrixPinDigest   = "sha256:4444444444444444444444444444444444444444444444444444444444444444"
	matrixOtherDigest = "sha256:7777777777777777777777777777777777777777777777777777777777777777"
)

type matrixRow struct {
	compose.ImageInput
	env                 map[string]string
	iVar, vVar          string // the *_IMAGE and *_VERSION variable the states set (decoys when the row has none)
	ver0, ver1, ver2    string
	pin, eqImg, diffImg string
	decoys              []string
}

func newMatrixRow(t *testing.T, in compose.ImageInput) *matrixRow {
	t.Helper()
	env, ok := rowEnv[in.Service+"/"+in.Entry]
	if !ok {
		t.Fatalf("row %s/%s has no rowEnv fixture: add one", in.Service, in.Entry)
	}
	r := &matrixRow{ImageInput: in, env: env, iVar: envName(in.Service) + "_IMAGE", vVar: envName(in.Service) + "_VERSION", ver0: "1.2.3", ver1: "1.2.4", ver2: "1.2.5"}
	if in.Service == "postgres" {
		r.ver0, r.ver1, r.ver2 = "15-alpine", "17-alpine", "18-alpine"
	}
	listed := map[string]bool{}
	for _, v := range in.Vars {
		listed[v] = true
		if strings.HasSuffix(v, "_IMAGE") {
			r.iVar = v
		}
		if strings.HasSuffix(v, "_VERSION") {
			r.vVar = v
		}
	}
	for _, p := range []string{envName(in.Service), envName(in.Entry)} {
		for _, suffix := range []string{"_VERSION", "_IMAGE"} {
			if !listed[p+suffix] && !contains(r.decoys, p+suffix) {
				r.decoys = append(r.decoys, p+suffix)
			}
		}
	}
	return r
}

func (r *matrixRow) feeds(name string) bool { return contains(r.Vars, name) }

func merge(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// fixture works out the pin and the image values of the states from the real
// generator (each in its own subtest, so the environment is restored).
func (r *matrixRow) fixture(t *testing.T) {
	t.Helper()
	render := func(name string, extra map[string]string) string {
		var got string
		t.Run(name, func(t *testing.T) { got = compose.CanonicalRef(renderStack(t, merge(r.env, extra))[r.Service]) })
		return got
	}
	if r.feeds(r.vVar) {
		r.pin = render("pin", map[string]string{r.vVar: r.ver0})
		r.diffImg = render("diff", map[string]string{r.vVar: r.ver2})
	} else {
		base := render("pin", nil)
		i := strings.LastIndex(base, ":")
		r.pin = base[:i] + ":pinned-old"
		r.diffImg = "example.com/decoy/img:1"
	}
	r.eqImg = r.pin
	r.pin += "@" + matrixPinDigest
	if !r.feeds(r.iVar) {
		r.eqImg = "example.com/decoy/img:0"
	}
}

// states are the env states of one row.
func (r *matrixRow) states() map[string]map[string]string {
	st := map[string]map[string]string{
		"d-neither":         {},
		"a-image-eq":        {r.iVar: r.eqImg},
		"a-image-diff":      {r.iVar: r.diffImg},
		"a-image-digest-eq": {r.iVar: r.eqImg + "@" + matrixPinDigest},
		"a-image-digest-ne": {r.iVar: r.eqImg + "@" + matrixOtherDigest},
		"b-version-eq":      {r.vVar: r.ver0},
		"b-version-diff":    {r.vVar: r.ver1},
		"c-eq-eq":           {r.iVar: r.eqImg, r.vVar: r.ver0},
		"c-eq-diff":         {r.iVar: r.eqImg, r.vVar: r.ver1},
		"c-diff-eq":         {r.iVar: r.diffImg, r.vVar: r.ver0},
		"c-diff-diff":       {r.iVar: r.diffImg, r.vVar: r.ver1},
	}
	all := map[string]string{}
	for _, d := range r.decoys {
		all[d] = "9.9.9-decoy"
		if strings.HasSuffix(d, "_IMAGE") {
			all[d] = "registry.example.com/decoy/img:1@" + matrixOtherDigest
		}
	}
	st["e-every-ignored-name"] = all
	return st
}

// expect works out kept/dropped and the variable the notice names.
func (r *matrixRow) expect(t *testing.T, env map[string]string, plain string) (kept bool, name string) {
	t.Helper()
	var set []string
	for _, v := range r.Vars {
		if strings.TrimSpace(env[v]) != "" {
			set = append(set, v)
		}
	}
	if len(set) == 0 {
		return true, ""
	}
	kept = compose.CanonicalRef(plain) == compose.CanonicalRef(r.pin)
	for _, v := range set {
		if _, d, ok := strings.Cut(env[v], "@"); ok && strings.HasSuffix(v, "_IMAGE") && d != matrixPinDigest {
			kept = false
		}
	}
	if kept {
		return true, ""
	}
	for _, v := range set {
		lone := ""
		only := map[string]string{}
		for _, o := range set {
			only[o] = "" // a lone render sees one variable of the state
		}
		only[v] = env[v]
		t.Run("lone-"+v, func(t *testing.T) { lone = renderStack(t, merge(env, only))[r.Service] })
		if lone == plain {
			if name != "" {
				t.Fatalf("fixture is ambiguous: %s and %s render the same image %q", name, v, plain)
			}
			name = v
		}
	}
	if name == "" {
		t.Fatalf("no single variable renders %q", plain)
	}
	return false, name
}

func TestProjectLockPinMatrix(t *testing.T) {
	for _, mode := range []string{"legacy", "lock"} {
		for _, in := range compose.ImageInputs() {
			r := newMatrixRow(t, in)
			t.Run(mode+"/"+in.Service+"/"+in.Entry, func(t *testing.T) {
				blankImageVars(t)
				t.Setenv("IMAGE_PINNING", mode)
				t.Setenv("NSELF_PLUGIN_DIR", filepath.Join(t.TempDir(), "plugins"))
				oldVersion := version.Version
				version.Version = "1.4.0"
				t.Cleanup(func() { version.Version = oldVersion })
				r.fixture(t)
				for name, st := range r.states() {
					t.Run(name, func(t *testing.T) { r.run(t, merge(r.env, st)) })
				}
			})
		}
	}
}

func (r *matrixRow) run(t *testing.T, env map[string]string) {
	t.Helper()
	restore := compose.SetProjectPins(nil)
	plain := renderStack(t, env)[r.Service]
	restore()
	kept, name := r.expect(t, env, plain)

	defer compose.SetProjectPins(map[string]string{r.Service: r.pin})()
	var notices bytes.Buffer
	defer compose.PinNoticeSink(&notices)()
	pinned := renderStack(t, env)[r.Service]
	_ = renderStack(t, env) // a build installs the pins and renders more than once
	n := strings.Count(notices.String(), "notice:")
	switch {
	case kept && (pinned != r.pin || n != 0):
		t.Errorf("pin must be kept silently: image %q (pin %q), notices %q", pinned, r.pin, notices.String())
	case !kept && (pinned != plain || pinned == r.pin || n != 1 || !strings.Contains(notices.String(), name+" is set")):
		t.Errorf("pin must drop with one notice naming %s: image %q (plain %q), notices %q", name, pinned, plain, notices.String())
	}

	dir := t.TempDir()
	body := "services:\n  " + r.Service + ":\n    image: " + pinned + "\n"
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	l := lock.New()
	l.CLI = "1.4"
	l.Images[r.Service] = r.pin
	raw, err := l.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock.Path(dir), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = checkStartLock(dir, true, &out)
	var ce *errs.CLIError
	switch {
	case kept && err != nil:
		t.Errorf("--frozen must pass when the pin is kept: %v", err)
	case !kept && (err == nil || !errors.As(err, &ce) || ce.Code != "E455"):
		t.Errorf("--frozen must fail with E455 when the pin was dropped, got %v", err)
	}
}
