package invoke

// Purpose: prove secret request values stay off the child argv (D-0468): the
// argv builder, the Exec environment hand-over and the child-side restore.
// Inputs: the fixture registry and the stub child. Outputs: pass/fail.
// Constraints: values are obviously fake. Every "absent" check has a positive
// control proving the scan could have found the value.

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/cmdregistry"
)

const fakePlanted = "FAKE-planted-0468-value"

// secretEcho is fx echo with a secret variadic arg and a secret slice flag, on
// top of the canon-secret flag "tok".
func secretEcho(t *testing.T) *cmdregistry.Command {
	t.Helper()
	c := *mustCmd(t, fixtureRegistry(t), "fx echo")
	c.Args = append([]cmdregistry.Arg{}, c.Args...)
	for i := range c.Args {
		if c.Args[i].Variadic {
			c.Args[i].Secret = true
		}
	}
	c.Flags = append([]cmdregistry.Flag{}, c.Flags...)
	for i := range c.Flags {
		if c.Flags[i].Name == "items" {
			c.Flags[i].Secret = true
		}
	}
	return &c
}

func TestBuildArgvSecretsKeepsValuesOffArgv(t *testing.T) {
	reg := fixtureRegistry(t)
	set := mustCmd(t, reg, "fx secretset")
	argv, b, err := BuildArgvSecrets(set, Request{Args: []string{"FAKEKEY", fakePlanted}}, TransportMCP)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range argv {
		if strings.Contains(a, fakePlanted) {
			t.Fatalf("the secret value is on argv: %q", argv)
		}
	}
	if !contains(argv, "FAKEKEY") {
		t.Errorf("positive control: the non-secret arg must stay on argv: %q", argv)
	}
	if b == nil || len(b.Values) != 1 || b.Values[0] != fakePlanted || len(b.Nonce) != 16 {
		t.Fatalf("bundle = %+v", b)
	}
	if want := placeholderPrefix + b.Nonce + ":0"; argv[len(argv)-1] != want {
		t.Errorf("last element %q, want placeholder %q", argv[len(argv)-1], want)
	}
	// BuildArgv (no bundle) yields the same placeholder shape, never the value.
	plain, err := BuildArgv(set, Request{Args: []string{"FAKEKEY", fakePlanted}})
	if err != nil || strings.Contains(strings.Join(plain, " "), fakePlanted) {
		t.Errorf("BuildArgv leaked the value: %q, %v", plain, err)
	}
}

func TestBuildArgvSecretsFlagsAndSlices(t *testing.T) {
	echo := secretEcho(t)
	doc := `{"args":["w","` + fakePlanted + `-a","` + fakePlanted + `-b"],"flags":{"tok":"` + fakePlanted + `-tok","name":"visible-name","items":["` + fakePlanted + `-i1","` + fakePlanted + `-i2"]}}`
	argv, b, err := BuildArgvSecrets(echo, req(t, doc), TransportMCP)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(argv, "\n"), fakePlanted) {
		t.Fatalf("a secret value is on argv: %q", argv)
	}
	for _, keep := range []string{"--name=visible-name", "w", "--json"} {
		if !contains(argv, keep) {
			t.Errorf("positive control: %q missing from %q", keep, argv)
		}
	}
	if b == nil || len(b.Values) != 5 {
		t.Fatalf("bundle = %+v", b)
	}
	// Each flag item keeps its --name= prefix; only the value is replaced.
	n := 0
	for _, a := range argv {
		if strings.HasPrefix(a, "--items="+placeholderPrefix+b.Nonce+":") || strings.HasPrefix(a, "--tok="+placeholderPrefix+b.Nonce+":") {
			n++
		}
	}
	if n != 3 {
		t.Errorf("want 3 placeholder flag items, got %d in %q", n, argv)
	}
	// The child side puts the real argv back.
	t.Setenv(InvokedByEnv, "mcp")
	t.Setenv(SecretsEnv, mustJSON(t, b))
	got, vals, err := RestoreSecretArgv(argv)
	if err != nil || !strings.Contains(strings.Join(got, "\n"), "--tok="+fakePlanted+"-tok") || len(vals) != 5 {
		t.Errorf("restore: %q %v %v", got, vals, err)
	}
}

func TestBuildArgvSecretsLimitsApplyToRealValues(t *testing.T) {
	set := mustCmd(t, fixtureRegistry(t), "fx secretset")
	if _, _, err := BuildArgvSecrets(set, Request{Args: []string{"K", strings.Repeat("a", MaxElementBytes+1)}}, TransportMCP); err == nil {
		t.Error("an oversized secret value must be refused")
	}
	if _, _, err := BuildArgvSecrets(set, Request{Args: []string{"K", "a\x00b"}}, TransportMCP); err == nil {
		t.Error("a NUL in a secret value must be refused")
	}
	if argv, b, err := BuildArgvSecrets(set, Request{Args: []string{"K", ""}}, TransportMCP); err != nil || b != nil {
		t.Errorf("an empty value needs no placeholder: %q %+v %v", argv, b, err)
	}
	// No secret in the request: no bundle.
	echo := mustCmd(t, fixtureRegistry(t), "fx echo")
	if _, b, err := BuildArgvSecrets(echo, Request{Args: []string{"w"}}, TransportMCP); err != nil || b != nil {
		t.Errorf("no secret, want nil bundle: %+v %v", b, err)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestExecSecretsTravelByEnvNotArgv runs the real stub child: the planted value
// is in its environment and nowhere in its argv.
func TestExecSecretsTravelByEnvNotArgv(t *testing.T) {
	useStub(t)
	t.Setenv(SecretsEnv, `{"nonce":"evil","values":["inherited"]}`)
	set := mustCmd(t, fixtureRegistry(t), "fx secretset")
	argv, b, err := BuildArgvSecrets(set, Request{Args: []string{"FAKEKEY", fakePlanted}}, TransportMCP)
	if err != nil {
		t.Fatal(err)
	}
	res, d := runStub(t, Spec{Argv: argv, Transport: TransportMCP, Secrets: b})
	if strings.Contains(strings.Join(d.Data.Argv, "\n"), fakePlanted) {
		t.Fatalf("child argv holds the value: %q", d.Data.Argv)
	}
	if !contains(d.Data.Argv, "FAKEKEY") || len(d.Data.Argv) != len(argv) {
		t.Errorf("positive control: child argv %q, want %q", d.Data.Argv, argv)
	}
	raw, ok := d.Data.Env[SecretsEnv]
	if !ok || !strings.Contains(raw, fakePlanted) || strings.Contains(raw, "inherited") {
		t.Fatalf("child env %s = %q (present %v), stdout %s", SecretsEnv, raw, ok, res.Stdout)
	}
	// Exactly what the child needs to restore its argv.
	for k, v := range d.Data.Env {
		if k == SecretsEnv || k == InvokedByEnv {
			t.Setenv(k, v)
		}
	}
	got, vals, err := RestoreSecretArgv(d.Data.Argv)
	if err != nil || got[len(got)-1] != fakePlanted || !reflect.DeepEqual(vals, []string{fakePlanted}) {
		t.Errorf("restore of the child argv: %q %v %v", got, vals, err)
	}

	// No bundle: an inherited value is dropped, none is set.
	_, d = runStub(t, Spec{Argv: []string{"x"}, Transport: TransportMCP})
	if v, ok := d.Data.Env[SecretsEnv]; ok {
		t.Errorf("an inherited %s reached the child: %q", SecretsEnv, v)
	}
}

func TestRestoreSecretArgv(t *testing.T) {
	const nonce = "0123456789abcdef"
	ph := func(n string) string { return placeholderPrefix + nonce + ":" + n }
	bundle := func(vals ...string) string { return mustJSON(t, SecretBundle{Nonce: nonce, Values: vals}) }
	cases := []struct {
		name      string
		env       string // "" = variable unset
		invokedBy string
		argv      []string
		want      []string
		wantVals  []string
		wantErr   string // "" = no error
	}{
		{"arg, flag and slice item", bundle("v0", "v1", "v2"), "mcp",
			[]string{"nself", "x", "--tok=" + ph("1"), "--items=" + ph("2"), "--", "k", ph("0")},
			[]string{"nself", "x", "--tok=v1", "--items=v2", "--", "k", "v0"}, []string{"v0", "v1", "v2"}, ""},
		{"value with equals sign", bundle("a=b=c"), "http", []string{"--k=" + ph("0")}, []string{"--k=a=b=c"}, []string{"a=b=c"}, ""},
		{"no env: placeholder-looking arg stays literal", "", "mcp", []string{"x", ph("0")}, []string{"x", ph("0")}, nil, ""},
		{"env without invoker marker is ignored", bundle("v0"), "", []string{"x", ph("0")}, []string{"x", ph("0")}, nil, ""},
		{"other nonce stays literal", bundle("v0"), "mcp",
			[]string{placeholderPrefix + "ffffffffffffffff:0", ph("0")}, []string{placeholderPrefix + "ffffffffffffffff:0", "v0"}, []string{"v0"}, ""},
		{"index out of range", bundle("v0"), "mcp", []string{ph("1"), ph("0")}, nil, nil, "E421"},
		{"negative or odd index", bundle("v0"), "mcp", []string{ph("-0"), ph("0")}, nil, nil, "E421"},
		{"value never consumed", bundle("v0", "v1"), "mcp", []string{ph("0")}, nil, nil, "E421"},
		{"value consumed twice", bundle("v0"), "mcp", []string{ph("0"), "--k=" + ph("0")}, nil, nil, "E421"},
		{"malformed JSON", "{not json", "mcp", []string{"x"}, nil, nil, "E421"},
		{"empty nonce", `{"nonce":"","values":["v"]}`, "mcp", []string{"x"}, nil, nil, "E421"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(InvokedByEnv, c.invokedBy)
			if c.env == "" {
				t.Setenv(SecretsEnv, "x")
				os.Unsetenv(SecretsEnv)
			} else {
				t.Setenv(SecretsEnv, c.env)
			}
			in := append([]string{}, c.argv...)
			got, vals, err := RestoreSecretArgv(in)
			if _, still := os.LookupEnv(SecretsEnv); still {
				t.Errorf("%s must be removed from the environment after the call", SecretsEnv)
			}
			if c.wantErr != "" {
				wantCode(t, err, c.wantErr)
				if strings.Contains(err.Error(), "v0") {
					t.Errorf("error must not echo a value: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) || !reflect.DeepEqual(vals, c.wantVals) {
				t.Errorf("got %q %q, want %q %q", got, vals, c.want, c.wantVals)
			}
		})
	}
}
