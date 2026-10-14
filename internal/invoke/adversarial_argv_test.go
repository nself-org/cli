package invoke

// Purpose: adversarial argv and flag smuggling tests of TestInvokeAdversarial.
// Constraints: a hostile value must reach the child as exactly one argv
// element at the position the layout gives it, or the request is refused
// (E420, E421 or E425) before any child starts.

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// advArgvSmuggling covers args and flag values that look like flags, `--` and
// `=` tricks, the argv member and flag names that are not spelled by the registry.
func advArgvSmuggling(t *testing.T) {
	hostile := []string{"--json", "-x", "--yes=true", "--flag=value", "--", "-", "--help", "-h", "--no-monorepo",
		"--no-monorepo=false", "--yes", "--plan-id=" + strings.Repeat("a", 64), " --json", "=", "--=", "---"}

	t.Run("args that look like flags stay positional", func(t *testing.T) {
		e := advFixture(t)
		got := echoedArgv(t, e.call("fx echo", doc(t, map[string]any{"args": hostile})))
		want := append([]string{"fx", "echo", "--json", "--"}, hostile...)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("argv %q, want %q", got, want)
		}
	})

	t.Run("flag values that look like flags stay inside one element", func(t *testing.T) {
		for _, v := range hostile {
			e := advFixture(t)
			got := echoedArgv(t, e.call("fx echo", doc(t, map[string]any{"args": []string{"w"}, "flags": map[string]any{"name": v}})))
			want := []string{"fx", "echo", "--json", "--name=" + v, "--", "w"}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("name=%q: argv %q, want %q", v, got, want)
			}
		}
	})

	t.Run("a slice flag item with = or a flag shape is one element each", func(t *testing.T) {
		e := advFixture(t)
		got := echoedArgv(t, e.call("fx echo", doc(t, map[string]any{"args": []string{"w"},
			"flags": map[string]any{"items": []string{"--yes=true", "a=b", "--"}}})))
		want := []string{"fx", "echo", "--json", "--items=--yes=true", "--items=a=b", "--items=--", "--", "w"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("argv %q, want %q", got, want)
		}
	})

	t.Run("flag names the registry does not spell are refused", func(t *testing.T) {
		names := []string{"name=evil", "--name", "-name", "name ", " name", "NAME", "Name", "name\x00", "na\nme", "name\r",
			"json", "help", "no-monorepo", "no-deprecation-warnings", "yes", "plan-id", "reveal", "secretish", "old", "nums", "",
			"NSELF_INVOKED_BY", "nself-invoked-by", "NSELF_MCP_TOKEN", "NSELF_INVOKE_SECRETS", "NSELF_INVOKE_PATH",
			"NSELF_INVOKE_DEADLINE_MS", "nself_v15", "ni\u0307ame", strings.Repeat("n", 100000)}
		for _, n := range names {
			e := advFixture(t)
			res := e.call("fx echo", doc(t, map[string]any{"args": []string{"w"}, "flags": map[string]any{n: "x"}}))
			e.wantRefusal(res, 0, "E420")
			if len(res.Stdout) > 4096 {
				t.Errorf("the refusal for a %d-byte flag name echoes %d bytes", len(n), len(res.Stdout))
			}
		}
	})

	t.Run("a stream flag is refused on a document transport", func(t *testing.T) {
		e := advFixture(t)
		e.wantRefusal(e.call("fx echo", `{"args":["w"],"flags":{"follow":true}}`), 0, "E421")
	})

	t.Run("argv versus args and flags", func(t *testing.T) {
		e := advFixture(t)
		ran := 0
		for _, c := range []struct{ path, doc string }{
			{"fx echo", `{"args":["w"],"argv":["--yes"]}`},
			{"fx echo", `{"argv":["w"]}`},
			{"demo", `{"args":["a"]}`},
			{"demo", `{"flags":{"name":"a"}}`},
			{"demo", `{"argv":["a"],"args":["b"]}`},
			{"demo", `{"argv":["a"],"confirm":"` + strings.Repeat("a", 64) + `"}`},
			{"typed", `{"argv":["x"]}`},
			{"typed", `{"args":["x"],"argv":["x"]}`},
		} {
			e.wantRefusal(e.call(c.path, c.doc), ran, "E420")
		}
	})

	t.Run("free-form argv: root flags are refused wherever they sit before --", func(t *testing.T) {
		e := advFixture(t)
		for _, a := range [][]string{{"--json"}, {"x", "--json"}, {"--json=false"}, {"--no-monorepo"}, {"y", "--no-monorepo=true"}, {"--help"}} {
			e.wantRefusal(e.call("demo", doc(t, map[string]any{"argv": a})), 0, "E420")
		}
	})

	t.Run("free-form argv: after -- is verbatim and the separator is kept", func(t *testing.T) {
		e := advFixture(t)
		in := []string{"a", "--", "--json", "--yes=true", "--"}
		got := echoedArgv(t, e.call("demo", doc(t, map[string]any{"argv": in})))
		want := append([]string{"demo", "--json"}, in...)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("argv %q, want %q", got, want)
		}
	})

	t.Run("a plugin parent never runs the child it names by first word", func(t *testing.T) {
		e := advFixture(t)
		e.wantRefusal(e.call("pwn status", `{"argv":["purge"]}`), 0, "E421")
		e.wantRefusal(e.call("pwn status", `{"argv":["leak","x"]}`), 0, "E421")
		e.wantRefusal(e.call("pwn status purge", `{}`), 0, "E421")
		e.wantRefusal(e.call("pwn status leak", `{}`), 0, "E421")
	})

	t.Run("command paths that are not registry paths never reach argv", func(t *testing.T) {
		e := advFixture(t)
		for _, p := range []string{"fx echo; id", "../fx echo", "fx/echo", "fx echo --json", "fx\x00echo", "fx echo $(id)",
			"-fx echo", "fx -echo", "fx echo\nfx destroy", "FX ECHO", "fx.echo", ""} {
			e.wantRefusal(e.call(p, `{"args":["w"]}`), 0, "E432")
		}
	})

	t.Run("typed plugin: option-looking args and flag values stay data", func(t *testing.T) {
		e := advFixture(t)
		got := echoedArgv(t, e.call("typed", `{"args":["--mode=evil","--json"],"flags":{"mode":"--json"}}`))
		want := []string{"typed", "--json", "--mode=--json", "--", "--mode=evil", "--json"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("argv %q, want %q", got, want)
		}
	})

	t.Run("wrong value types are refused, never coerced", func(t *testing.T) {
		for _, f := range []map[string]any{
			{"count": "1"}, {"count": 1.5}, {"count": []any{1}}, {"count": nil}, {"count": true},
			{"limit": -1}, {"on": "true"}, {"on": 1}, {"on": nil}, {"ratio": "NaN"}, {"ratio": "1e400"}, {"wait": 30}, {"wait": "1h; id"},
			{"tags": "a,b"}, {"tags": []any{1}}, {"tags": map[string]any{"a": "b"}}, {"tags": []any{[]any{"x"}}},
			{"tags": []any{"a,b"}}, {"tags": []any{"a\nb"}}, {"tags": []any{""}}, {"tags": []any{"a\"b"}}, {"name": []any{"a"}},
			{"name": map[string]any{}}, {"name": nil}, {"name": 5}, {"items": "x"},
		} {
			e := advFixture(t)
			e.wantRefusal(e.call("fx echo", doc(t, map[string]any{"args": []string{"w"}, "flags": f})), 0, "E420")
		}
		for _, raw := range []string{`{"flags":{"count":1e999999}}`, `{"flags":{"count":` + strings.Repeat("9", 5000) + `}}`,
			`{"flags":{"limit":18446744073709551616}}`, `{"flags":{"count":9007199254740993.5}}`} {
			e := advFixture(t)
			e.wantRefusal(e.call("fx echo", strings.Replace(raw, `{"flags"`, `{"args":["w"],"flags"`, 1)), 0, "E420")
		}
	})

	t.Run("the child cwd is the server dir whatever the arguments say", func(t *testing.T) {
		e := advFixture(t)
		res := e.call("fx echo", `{"args":["../../../etc","/","~"],"flags":{"name":"../.."}}`)
		data := decodeDoc(t, res.Stdout)["data"].(map[string]any)
		got, _ := filepath.EvalSymlinks(data["cwd"].(string))
		want, _ := filepath.EvalSymlinks(e.dir)
		if got != want {
			t.Errorf("cwd %q, want %q", got, want)
		}
	})

	t.Run("the invoker env wins over a hostile parent env", func(t *testing.T) {
		e := advFixture(t)
		t.Setenv(InvokedByEnv, "attacker")
		t.Setenv(TokenEnv, "nself_test_FAKE_token_0001")
		t.Setenv(SecretsEnv, "attacker")
		t.Setenv(PathEnv, "fx destroy")
		res := e.call("fx echo", `{"args":["w"]}`)
		env := decodeDoc(t, res.Stdout)["data"].(map[string]any)["env"].(map[string]any)
		if env[InvokedByEnv] != "mcp" || env[PathEnv] != "fx echo" {
			t.Errorf("invoker-owned env was overridden: %v=%v %v=%v", InvokedByEnv, env[InvokedByEnv], PathEnv, env[PathEnv])
		}
		if _, ok := env[TokenEnv]; ok {
			t.Error("the server token reached the child env")
		}
		if v, ok := env[SecretsEnv]; ok {
			t.Errorf("a parent %s reached a request with no secrets: %v", SecretsEnv, v)
		}
	})
}

// advOwnedFlags: consent, plan and plan-id flags belong to the invoker (E425).
func advOwnedFlags(t *testing.T) {
	cases := []struct{ path, flag string }{
		{"gx plain", "yes"}, {"gx push", "yes"}, {"gx wipe", "yes"}, {"gx wipe", "force"}, {"gx apply", "force"},
		{"gx apply", "plan"}, {"gx apply", "plan-id"}, {"gx purge", "yes"}, {"gx purge", "plan"}, {"gx purge", "plan-id"}, {"gx scan", "yes"},
	}
	values := []any{true, false, "true", "false", 1, 0, nil, []string{"x"}, map[string]any{}}
	t.Run("a request that names an owned flag is E425 whatever the value", func(t *testing.T) {
		for _, c := range cases {
			for _, v := range values {
				e := advGate(t)
				res := e.call(c.path, doc(t, map[string]any{"flags": map[string]any{c.flag: v}}))
				e.wantRefusal(res, 0, "E425")
				if n := e.guard.liveNonces(); n != 0 {
					t.Errorf("%s --%s=%v: a refused request issued %d nonces", c.path, c.flag, v, n)
				}
			}
		}
	})
	t.Run("owned flags are refused ahead of every other check", func(t *testing.T) {
		e := advGate(t)
		bad := doc(t, map[string]any{"args": []string{"a", "b", "c"}, "flags": map[string]any{"yes": true, "zzz": 1}, "confirm": strings.Repeat("a", 64)})
		e.wantRefusal(e.call("gx wipe", bad), 0, "E425")
	})
	t.Run("a single-spelling variant of an owned flag is an unknown flag", func(t *testing.T) {
		for _, n := range []string{"Yes", "YES", "--yes", "yes=true", "y", "yes ", "force\x00", "plan_id", "PLAN-ID", "Plan"} {
			e := advGate(t)
			e.wantRefusal(e.call("gx purge", doc(t, map[string]any{"flags": map[string]any{n: true}})), 0, "E420")
		}
	})
	t.Run("an owned flag on a command with no confirm block is just an unknown flag", func(t *testing.T) {
		e := advGate(t)
		e.wantRefusal(e.call("gx read", `{"flags":{"yes":true}}`), 0, "E420")
		e.wantRefusal(e.call("gx put", `{"args":["k","v"],"flags":{"force":true}}`), 0, "E420")
	})
	t.Run("a flag named like an invoker variable is data-less: unknown", func(t *testing.T) {
		e := advGate(t)
		for _, n := range []string{"NSELF_INVOKED_BY", "NSELF_MCP_TOKEN", "NSELF_INVOKE_PATH", "NSELF_V15", "NSELF_NONINTERACTIVE"} {
			e.wantRefusal(e.call("gx wipe", doc(t, map[string]any{"flags": map[string]any{n: "x"}})), 0, "E420")
		}
		if _, err := os.Stat(e.argvLog); !os.IsNotExist(err) {
			t.Error("a child ran")
		}
	})
}
