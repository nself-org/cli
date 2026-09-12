package doctor

// Tests for G-014's healthcheck-validity check. extractHealthcheckBinary and
// classifyBinaryProbe are pure — exercised directly, no live docker daemon
// or real project containers required. diagnoseUnhealthyContainer itself
// shells out (docker inspect/exec) so it is not unit-tested here; its
// building blocks are, which is what actually encodes the decision logic.

import "testing"

func TestExtractHealthcheckBinary(t *testing.T) {
	cases := []struct {
		name       string
		test       []string
		wantMode   string
		wantBinary string
	}{
		{
			name:       "CMD-SHELL curl (the measured prod case)",
			test:       []string{"CMD-SHELL", "curl -f http://localhost:8080/health || exit 1"},
			wantMode:   "CMD-SHELL",
			wantBinary: "curl",
		},
		{
			name:       "CMD exec-form",
			test:       []string{"CMD", "curl", "-f", "http://localhost:8080/health"},
			wantMode:   "CMD",
			wantBinary: "curl",
		},
		{
			name:       "CMD-SHELL wget",
			test:       []string{"CMD-SHELL", "wget --spider -q http://localhost/health"},
			wantMode:   "CMD-SHELL",
			wantBinary: "wget",
		},
		{
			name:       "NONE disables the healthcheck",
			test:       []string{"NONE"},
			wantMode:   "NONE",
			wantBinary: "",
		},
		{
			name:       "empty test slice",
			test:       nil,
			wantMode:   "",
			wantBinary: "",
		},
		{
			name:       "CMD-SHELL with no command string",
			test:       []string{"CMD-SHELL"},
			wantMode:   "CMD-SHELL",
			wantBinary: "",
		},
		{
			name:       "CMD-SHELL with blank command string",
			test:       []string{"CMD-SHELL", "   "},
			wantMode:   "CMD-SHELL",
			wantBinary: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mode, binary := extractHealthcheckBinary(c.test)
			if mode != c.wantMode {
				t.Errorf("mode = %q, want %q", mode, c.wantMode)
			}
			if binary != c.wantBinary {
				t.Errorf("binary = %q, want %q", binary, c.wantBinary)
			}
		})
	}
}

func TestClassifyBinaryProbe(t *testing.T) {
	cases := []struct {
		name            string
		stdout          string
		ranToCompletion bool
		wantFound       bool
		wantVerifiable  bool
	}{
		{
			name:            "binary found: command -v prints its path",
			stdout:          "/usr/bin/curl\n",
			ranToCompletion: true,
			wantFound:       true,
			wantVerifiable:  true,
		},
		{
			name:            "binary confirmed missing: exit ran, empty stdout",
			stdout:          "",
			ranToCompletion: true,
			wantFound:       false,
			wantVerifiable:  true,
		},
		{
			name:            "probe never ran (no shell in image, daemon unreachable, etc.)",
			stdout:          "",
			ranToCompletion: false,
			wantFound:       false,
			wantVerifiable:  false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			found, verifiable := classifyBinaryProbe(c.stdout, c.ranToCompletion)
			if found != c.wantFound {
				t.Errorf("found = %v, want %v", found, c.wantFound)
			}
			if verifiable != c.wantVerifiable {
				t.Errorf("verifiable = %v, want %v", verifiable, c.wantVerifiable)
			}
		})
	}
}
