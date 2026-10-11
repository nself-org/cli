package commands

// Purpose: prove `secrets set` reads VALUE from stdin when it is omitted or
// "-" (D-0468), and refuses an empty or oversized value.
// Inputs: fake values on an in-memory reader or a real pipe. Outputs: pass/fail.

import (
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/secrets"
	"github.com/spf13/cobra"
)

func stdinCmd(in string) *cobra.Command {
	c := &cobra.Command{Use: "set"}
	c.SetIn(strings.NewReader(in))
	c.SetErr(&strings.Builder{})
	return c
}

func TestReadSecretValue(t *testing.T) {
	cases := []struct {
		name, in, want string
		wantErr        bool
	}{
		{"plain", "FAKE-planted-0468-value", "FAKE-planted-0468-value", false},
		{"LF trimmed once", "FAKE-planted-0468-value\n\n", "FAKE-planted-0468-value\n", false},
		{"CRLF trimmed", "FAKE-planted-0468-value\r\n", "FAKE-planted-0468-value", false},
		{"inner newline kept", "line1\nline2\n", "line1\nline2", false},
		{"empty refused", "", "", true},
		{"newline only refused", "\n", "", true},
		{"CRLF only refused", "\r\n", "", true},
		{"too large refused", strings.Repeat("a", maxSecretStdinBytes+1), "", true},
	}
	for _, c := range cases {
		got, err := readSecretValue(stdinCmd(c.in), "FAKEKEY")
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("%s: got %q err %v, want %q (err %v)", c.name, got, err, c.want, c.wantErr)
		}
	}
	if got, err := readSecretValue(stdinCmd(strings.Repeat("a", maxSecretStdinBytes)), "K"); err != nil || len(got) != maxSecretStdinBytes {
		t.Errorf("value at the cap must be accepted: len %d err %v", len(got), err)
	}
}

func TestReadSecretValue_RealPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	go func() {
		_, _ = w.WriteString("FAKE-planted-0468-value\n")
		_ = w.Close()
	}()
	c := &cobra.Command{Use: "set"}
	c.SetIn(r)
	c.SetErr(&strings.Builder{})
	got, err := readSecretValue(c, "FAKEKEY")
	if err != nil || got != "FAKE-planted-0468-value" {
		t.Errorf("pipe: got %q err %v", got, err)
	}
}

// TestSecretsSet_ValueFromStdin runs the real command: omitted VALUE, the "-"
// form and the two-argument form all store the value.
func TestSecretsSet_ValueFromStdin(t *testing.T) {
	requireAge(t)
	withProjectRoot(t, func(root string) {
		initSecretsProject(t, root)
		secretsEnvFlag = "dev"
		cases := []struct {
			key  string
			args []string
			in   string
			want string
		}{
			{"FAKE_OMITTED", []string{"FAKE_OMITTED"}, "FAKE-planted-0468-value\n", "FAKE-planted-0468-value"},
			{"FAKE_DASH", []string{"FAKE_DASH", "-"}, "FAKE-planted-0468-dash\r\n", "FAKE-planted-0468-dash"},
			{"FAKE_ARGV", []string{"FAKE_ARGV", "FAKE-planted-0468-argv"}, "ignored\n", "FAKE-planted-0468-argv"},
		}
		for _, c := range cases {
			cmd := &cobra.Command{Use: "set"}
			cmd.SetIn(strings.NewReader(c.in))
			cmd.SetErr(&strings.Builder{})
			if err := secretsSetCmd.RunE(cmd, c.args); err != nil {
				t.Fatalf("%s: %v", c.key, err)
			}
			got, err := secrets.Get(root, "dev", c.key)
			if err != nil || got != c.want {
				t.Errorf("%s: stored %q err %v, want %q", c.key, got, err, c.want)
			}
		}
		empty := &cobra.Command{Use: "set"}
		empty.SetIn(strings.NewReader(""))
		empty.SetErr(&strings.Builder{})
		if err := secretsSetCmd.RunE(empty, []string{"FAKE_EMPTY"}); err == nil {
			t.Error("an empty stdin value must be refused")
		}
	})
}
