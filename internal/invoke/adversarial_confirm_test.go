package invoke

// Purpose: adversarial confirmation tests of TestInvokeAdversarial: nonce
// replay, cross-request reuse, expiry, flood eviction, confirm string tricks.
// Constraints: time is the Guard's injected fake clock; maxLiveNonces is
// lowered through its test hook and restored.

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
	"time"
)

// withConfirm returns the request document for m with confirm set to c.
func withConfirm(t *testing.T, m map[string]any, c string) string {
	t.Helper()
	out := map[string]any{"confirm": c}
	for k, v := range m {
		out[k] = v
	}
	return doc(t, out)
}

// nonceFor asks for a confirmation of path with request m and returns the nonce.
func nonceFor(t *testing.T, e *gateEnv, path string, m map[string]any) string {
	t.Helper()
	res := e.call(path, doc(t, m))
	return confirmationOf(t, res).Confirm
}

func advConfirmReplay(t *testing.T) {
	target := func(v string) map[string]any { return map[string]any{"flags": map[string]any{"target": v}} }

	t.Run("a spent nonce cannot run the command again", func(t *testing.T) {
		e := advGate(t)
		n := nonceFor(t, e, "gx wipe", target("a"))
		if res := e.call("gx wipe", withConfirm(t, target("a"), n)); res.ErrorCode != "" || res.ExitCode != 0 {
			t.Fatalf("the first spend must run: %s", res.Stdout)
		}
		if len(e.argvs()) != 1 {
			t.Fatalf("want one child run, got %v", e.argvs())
		}
		for i := 0; i < 3; i++ {
			e.wantRefusal(e.call("gx wipe", withConfirm(t, target("a"), n)), 1, "E424")
		}
		// a replay still asks for a fresh confirmation when sent without one
		if n2 := nonceFor(t, e, "gx wipe", target("a")); n2 == n {
			t.Error("the server reissued a spent nonce")
		}
	})

	t.Run("a nonce issued for one request fails for another and is burned", func(t *testing.T) {
		for name, other := range map[string]struct {
			path string
			m    map[string]any
		}{
			"other flag value":          {"gx wipe", target("b")},
			"other arg":                 {"gx wipe", map[string]any{"args": []string{"x"}, "flags": map[string]any{"target": "a"}}},
			"dropped flag":              {"gx wipe", map[string]any{}},
			"other command":             {"gx push", target("a")},
			"other destructive command": {"gx scan", map[string]any{"flags": map[string]any{"prune": true}}},
		} {
			e := advGate(t)
			n := nonceFor(t, e, "gx wipe", target("a"))
			e.wantRefusal(e.call(other.path, withConfirm(t, other.m, n)), 0, "E424")
			// the nonce was presented for another request: it is gone for the real one too
			e.wantRefusal(e.call("gx wipe", withConfirm(t, target("a"), n)), 0, "E424")
			if t.Failed() {
				t.Logf("case %q", name)
			}
		}
	})

	t.Run("a nonce from one Guard is worthless on another", func(t *testing.T) {
		a, b := advGate(t), advGate(t)
		n := nonceFor(t, a, "gx wipe", target("a"))
		b.wantRefusal(b.call("gx wipe", withConfirm(t, target("a"), n)), 0, "E424")
	})

	t.Run("confirm digest ignores nothing that changes argv", func(t *testing.T) {
		e := advGate(t)
		n := nonceFor(t, e, "gx wipe", map[string]any{"args": []string{"a"}, "flags": map[string]any{"target": "t"}})
		// the same request spelled differently (key order) is the same request
		same := `{"flags":{"target":"t"},"confirm":"` + n + `","args":["a"]}`
		if res := e.call("gx wipe", same); res.ErrorCode != "" {
			t.Errorf("an identical request must spend the nonce: %s", res.Stdout)
		}
		n = nonceFor(t, e, "gx wipe", map[string]any{"args": []string{"a"}})
		e.wantRefusal(e.call("gx wipe", `{"args":["a "],"confirm":"`+n+`"}`), 1, "E424") // trailing space is another request
	})

	t.Run("a nonce for a request cannot be spent on a refusing variant that never runs", func(t *testing.T) {
		e := advGate(t)
		n := nonceFor(t, e, "gx wipe", target("a"))
		// a variant that cannot be built is refused before the nonce is looked up
		e.wantRefusal(e.call("gx wipe", withConfirm(t, map[string]any{"args": []string{"a", "b"}}, n)), 0, "E420")
		if res := e.call("gx wipe", withConfirm(t, target("a"), n)); res.ErrorCode != "" {
			t.Errorf("an unrelated refusal must not burn the nonce: %s", res.Stdout)
		}
	})
}

func advConfirmExpiryFlood(t *testing.T) {
	target := func(v string) map[string]any { return map[string]any{"flags": map[string]any{"target": v}} }

	t.Run("a nonce lives strictly less than ten minutes", func(t *testing.T) {
		e := advGate(t)
		live := nonceFor(t, e, "gx wipe", target("live"))
		edge := nonceFor(t, e, "gx wipe", target("edge"))
		e.clock.advance(nonceTTL - time.Second)
		if res := e.call("gx wipe", withConfirm(t, target("live"), live)); res.ErrorCode != "" {
			t.Fatalf("a nonce one second before expiry must work: %s", res.Stdout)
		}
		e.clock.advance(time.Second) // exactly the TTL
		e.wantRefusal(e.call("gx wipe", withConfirm(t, target("edge"), edge)), 1, "E424")
	})

	t.Run("an expired nonce stays dead when the clock is read again", func(t *testing.T) {
		e := advGate(t)
		n := nonceFor(t, e, "gx wipe", target("a"))
		e.clock.advance(nonceTTL + time.Hour)
		e.wantRefusal(e.call("gx wipe", withConfirm(t, target("a"), n)), 0, "E424")
		e.wantRefusal(e.call("gx wipe", withConfirm(t, target("a"), n)), 0, "E424")
		if n := e.guard.liveNonces(); n != 0 {
			t.Errorf("an expired nonce was kept: %d", n)
		}
	})

	t.Run("flood: the store is bounded and evicts the oldest", func(t *testing.T) {
		old := maxLiveNonces
		maxLiveNonces = 8
		t.Cleanup(func() { maxLiveNonces = old })
		e := advGate(t)
		var nonces []string
		for i := 0; i < 20; i++ {
			nonces = append(nonces, nonceFor(t, e, "gx wipe", target("t"+strconv.Itoa(i))))
			if n := e.guard.liveNonces(); n > 8 {
				t.Fatalf("after %d requests the store holds %d nonces, max 8", i+1, n)
			}
		}
		e.wantRefusal(e.call("gx wipe", withConfirm(t, target("t0"), nonces[0])), 0, "E424")
		e.wantRefusal(e.call("gx wipe", withConfirm(t, target("t11"), nonces[11])), 0, "E424")
		if res := e.call("gx wipe", withConfirm(t, target("t12"), nonces[12])); res.ErrorCode != "" {
			t.Errorf("the oldest surviving nonce must still work: %s", res.Stdout)
		}
		if res := e.call("gx wipe", withConfirm(t, target("t19"), nonces[19])); res.ErrorCode != "" {
			t.Errorf("the newest nonce must work: %s", res.Stdout)
		}
	})

	t.Run("flood: expired nonces go first", func(t *testing.T) {
		old := maxLiveNonces
		maxLiveNonces = 4
		t.Cleanup(func() { maxLiveNonces = old })
		e := advGate(t)
		for i := 0; i < 4; i++ {
			nonceFor(t, e, "gx wipe", target("old"+strconv.Itoa(i)))
		}
		e.clock.advance(nonceTTL + time.Second)
		fresh := nonceFor(t, e, "gx wipe", target("fresh"))
		if n := e.guard.liveNonces(); n != 1 {
			t.Errorf("the four expired nonces should have been dropped, store holds %d", n)
		}
		if res := e.call("gx wipe", withConfirm(t, target("fresh"), fresh)); res.ErrorCode != "" {
			t.Errorf("the fresh nonce must work: %s", res.Stdout)
		}
	})

	t.Run("flood at the production bound", func(t *testing.T) {
		g := NewGuard("")
		var digest [32]byte
		first, err := g.issue(digest)
		if err != nil {
			t.Fatal(err)
		}
		var last string
		for i := 0; i < 3*maxLiveNonces; i++ {
			if last, err = g.issue(digest); err != nil {
				t.Fatal(err)
			}
		}
		if n := g.liveNonces(); n != maxLiveNonces {
			t.Errorf("store size %d, want %d", n, maxLiveNonces)
		}
		if g.consume(first, digest) {
			t.Error("the first nonce survived a flood three times the bound")
		}
		if !g.consume(last, digest) {
			t.Error("the newest nonce was lost")
		}
	})
}

func advConfirmStrings(t *testing.T) {
	good := strings.Repeat("ab", 32)
	t.Run("malformed confirm strings are E420 at decode", func(t *testing.T) {
		for name, c := range map[string]string{
			"63 chars":           good[:63],
			"65 chars":           good + "a",
			"128 chars":          good + good,
			"uppercase":          strings.ToUpper(good),
			"mixed case":         "A" + good[1:],
			"leading space":      " " + good[1:],
			"trailing space":     good[1:] + " ",
			"padded":             " " + good + " ",
			"trailing newline":   good + "\n",
			"trailing newline64": good[:63] + "\n",
			"embedded NUL":       good[:30] + "\x00" + good[31:],
			"NUL escaped":        good[:30] + "\\u0000" + good[31:],
			"0x prefix":          "0x" + good[2:],
			"non-hex letter":     "g" + good[1:],
			"full-width digits":  strings.Repeat("\uff10", 64),
			"arabic digits":      strings.Repeat("\u0660", 64),
			"tab":                good[:10] + "\t" + good[11:],
			"newline inside":     good[:10] + "\n" + good[11:],
			"empty with space":   " ",
			"hyphenated":         good[:32] + "-" + good[33:],
			"null char":          "\u0000",
		} {
			body := `{"confirm":` + doc(t, c) + `}`
			_, err := DecodeRequest([]byte(body))
			wantE420(t, err, name)
		}
	})

	t.Run("a well-formed confirm that was never issued is E424 and runs nothing", func(t *testing.T) {
		e := advGate(t)
		for _, c := range []string{good, strings.Repeat("0", 64), strings.Repeat("f", 64), shaHex("x")} {
			e.wantRefusal(e.call("gx wipe", `{"confirm":"`+c+`"}`), 0, "E424")
			e.wantRefusal(e.call("gx push", `{"confirm":"`+c+`"}`), 0, "E424")
		}
	})

	t.Run("the digest of a nonce is not a nonce", func(t *testing.T) {
		e := advGate(t)
		n := nonceFor(t, e, "gx wipe", map[string]any{})
		raw, _ := hex.DecodeString(n)
		sum := sha256.Sum256([]byte(n))
		sumRaw := sha256.Sum256(raw)
		for _, c := range []string{hex.EncodeToString(sum[:]), hex.EncodeToString(sumRaw[:])} {
			e.wantRefusal(e.call("gx wipe", `{"confirm":"`+c+`"}`), 0, "E424")
		}
		if res := e.call("gx wipe", `{"confirm":"`+n+`"}`); res.ErrorCode != "" {
			t.Errorf("the real nonce must still work: %s", res.Stdout)
		}
	})

	t.Run("nonces are random, unique and well formed", func(t *testing.T) {
		e := advGate(t)
		seen := map[string]bool{}
		for i := 0; i < 200; i++ {
			n := nonceFor(t, e, "gx wipe", map[string]any{})
			if !confirmPattern.MatchString(n) || seen[n] {
				t.Fatalf("nonce %q at %d is malformed or repeated", n, i)
			}
			seen[n] = true
		}
	})

	t.Run("a confirm on a command with no gate is refused or ignored, never a bypass", func(t *testing.T) {
		e := advGate(t)
		e.wantRefusal(e.call("gx read", `{"confirm":"`+good+`"}`), 0, "E420")
		e.wantRefusal(e.call("gx put", `{"args":["k","v"],"confirm":"`+good+`"}`), 0, "E420")
		e.wantRefusal(e.call("gx bare", `{"confirm":"`+good+`"}`), 0, "E421") // never exposed: no confirm block
		// gx plain: a write with a confirm block and no plan form runs on its own; confirm adds nothing
		if res := e.call("gx plain", `{"confirm":"`+good+`"}`); res.ErrorCode != "" {
			t.Fatalf("plain: %s", res.Stdout)
		}
		for _, a := range e.argvs()[0] {
			if strings.HasPrefix(a, "--yes") || a == good {
				t.Errorf("a confirm on an ungated write leaked into argv: %q", e.argvs()[0])
			}
		}
	})
}
