package deploy

import "testing"

// TestValidateRemotePath_RejectsInjectionPayloads (T31) asserts the shared
// remote-path validator rejects shell metacharacters that would otherwise
// reach a remote shell via DeployViaSsh's fmt.Sprintf-built commands.
func TestValidateRemotePath_RejectsInjectionPayloads(t *testing.T) {
	payloads := []string{
		"/opt/x; id",
		"/opt/x$(id)",
		"/opt/x`id`",
		"/opt/x | id",
		"/opt/x && id",
		"/opt/x\nid",
		"/opt/x id",
	}
	for _, p := range payloads {
		if err := ValidateRemotePath(p); err == nil {
			t.Errorf("ValidateRemotePath(%q): expected error, got nil", p)
		}
	}
}

// TestValidateRemotePath_AcceptsLegitimatePaths proves no regression for the
// paths real deployments use.
func TestValidateRemotePath_AcceptsLegitimatePaths(t *testing.T) {
	ok := []string{
		"",
		"/opt/nself",
		"/opt/nself-staging",
		"/home/ubuntu/app_v2",
		"opt/nself",
		"/opt/nself.d",
	}
	for _, p := range ok {
		if err := ValidateRemotePath(p); err != nil {
			t.Errorf("ValidateRemotePath(%q): unexpected error: %v", p, err)
		}
	}
}

// TestValidateRemotePath_AcceptsDotDotSegments: origin/main accepted ".."
// segments (filepath.Join cleans them), so deploy, inventory Load and env
// target CRUD still do. The ".." refusal lives only in the sdk's CopyTo and
// Rsync validator.
func TestValidateRemotePath_AcceptsDotDotSegments(t *testing.T) {
	for _, p := range []string{"/srv/../app", "../x", "/opt/x/..", "a/../b"} {
		if err := ValidateRemotePath(p); err != nil {
			t.Errorf("ValidateRemotePath(%q): unexpected error: %v", p, err)
		}
	}
}

// TestValidateRemotePath_RejectsLeadingDash: the one refusal added to
// origin/main's behaviour; a remote rsync --server or scp would read a
// leading '-' as an option.
func TestValidateRemotePath_RejectsLeadingDash(t *testing.T) {
	for _, p := range []string{"-", "-rf", "-oProxyCommand=x", "--delete"} {
		if err := ValidateRemotePath(p); err == nil {
			t.Errorf("ValidateRemotePath(%q): expected error, got nil", p)
		}
	}
}

// TestValidateRemotePath_ErrorTextVerbatim: the charset error text is the
// one origin/main produced.
func TestValidateRemotePath_ErrorTextVerbatim(t *testing.T) {
	err := ValidateRemotePath("/opt/x; id")
	want := `remote path contains unsafe characters (got "/opt/x; id"): only [a-zA-Z0-9/_.-] allowed`
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}
