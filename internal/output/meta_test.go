package output

import (
	"errors"
	"strings"
	"testing"
)

func TestNoMetaWithoutNotices(t *testing.T) {
	reset(t)
	w, out, _ := bufs()
	if err := EmitData(w, "x", 1); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "meta") {
		t.Fatalf("meta key present with no notices: %s", out.String())
	}
}

func TestMetaDeprecationGolden(t *testing.T) {
	reset(t)
	AddDeprecation("env list", "config env list", "v1.6.0")
	w, out, _ := bufs()
	if err := EmitData(w, "status", sampleValue); err != nil {
		t.Fatal(err)
	}
	golden(t, "meta-deprecation.golden.json", out.Bytes())
	want := "  \"meta\": {\n    \"deprecations\": [\n      {\n        \"old\": \"env list\",\n        \"new\": \"config env list\",\n        \"removal_at\": \"v1.6.0\"\n      }\n    ]\n  }\n}\n"
	if !strings.HasSuffix(out.String(), want) {
		t.Fatalf("envelope does not end with the meta member:\n%s", out.String())
	}
}

func TestMetaOnErrorEnvelopeWithWarnings(t *testing.T) {
	reset(t)
	AddWarning("first")
	AddWarning("second")
	AddWarning("first")
	AddDeprecation("a", "b", "v1.6.0")
	AddDeprecation("a", "b", "v1.6.0")
	AddDeprecation("c", "d", "v1.7.0")
	w, out, _ := bufs()
	if err := EmitError(w, "x", errors.New("boom")); err != nil {
		t.Fatal(err)
	}
	golden(t, "meta-error.golden.json", out.Bytes())
}

func TestInvocationState(t *testing.T) {
	reset(t)
	if _, _, known := Invocation(); known {
		t.Fatal("known before SetInvocation")
	}
	SetInvocation("config get", true)
	c, j, known := Invocation()
	if c != "config get" || !j || !known {
		t.Fatalf("got %q %v %v", c, j, known)
	}
	SetInvocation("", false)
	if c, j, known = Invocation(); c != "" || j || !known {
		t.Fatalf("second set: %q %v %v", c, j, known)
	}
	ResetState()
	if _, _, known = Invocation(); known {
		t.Fatal("known after ResetState")
	}
}
