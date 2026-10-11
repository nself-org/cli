package reconcile

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestApplySecretValuesIsLinear: thousands of distinct secret entries plus one
// changed line must diff in well under two seconds (the value pass was
// lines x values) and leak none of the values.
func TestApplySecretValuesIsLinear(t *testing.T) {
	const n = 15000
	build := func(changed string) []byte {
		var sb strings.Builder
		for i := 0; i < n; i++ {
			fmt.Fprintf(&sb, "K%d_SECRET: fakevalue%d\n", i, i)
		}
		sb.WriteString("note: " + changed + "\n")
		return []byte(sb.String())
	}
	before, after := build("one"), build("two")
	start := time.Now()
	got := Unified("docker-compose.yml", before, after)
	elapsed := time.Since(start)
	t.Logf("Unified over %d secret entries: %v", n, elapsed)
	if got == "" {
		t.Fatal("empty diff")
	}
	for _, i := range []int{0, n / 2, n - 1} {
		v := fmt.Sprintf("fakevalue%d\n", i)
		if !strings.Contains(string(before), v) {
			t.Fatalf("positive control: input lacks %q", v)
		}
		if strings.Contains(got, strings.TrimSpace(v)) {
			t.Errorf("diff leaks %q", v)
		}
	}
	if elapsed > 2*time.Second {
		t.Errorf("took %v, want under 2s", elapsed)
	}
}
