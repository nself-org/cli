package main

// Port of scripts/release/lib/hashes.sh (part 2): the published-checksums
// check (S7) and the release.yml job gate (S6). GET-only.

import (
	"os"
	"path/filepath"
)

const gateJQ = `
    [.jobs[] | select((.name | test("Homebrew|Sign and publish"; "i")) | not)] as $j
    | ($j | map(select(.status == "completed" and (.conclusion == "failure" or .conclusion == "cancelled" or .conclusion == "timed_out")))) as $f
    | ($j | map(select(.status != "completed"))) as $p
    | if ($f | length) > 0 then "failed:" + $f[0].name elif ($p | length) > 0 then "pending" else "ok" end`

// head12 is ${s:0:12}.
func head12(s string) string { return head(s, 12) }

// head is ${s:0:n}.
func head(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// VerifyPublished is verify_published (S7): the formula on main must carry the
// hashes of the checksums.txt that was actually published.
func (r *Release) VerifyPublished() error {
	d := filepath.Join(r.Work, "published")
	os.RemoveAll(d)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	if _, err := r.ghNeed("the published checksums.txt", "release", "download", r.Tag, "-R", "nself-org/cli",
		"-p", "checksums.txt", "-D", d, "--clobber"); err != nil {
		return err
	}
	b, _ := os.ReadFile(filepath.Join(d, "checksums.txt"))
	arm, intel := r.SumsHash(string(b), "arm64"), r.SumsHash(string(b), "amd64")
	if arm == "" || intel == "" {
		return r.die("S7", "release-wait", "the published checksums.txt has no darwin arm64/amd64 lines")
	}
	if err := r.LoadFormula(); err != nil {
		return err
	}
	if r.FVer != r.Version || r.FArm != arm || r.FIntel != intel {
		return r.die("S7", "release-wait", "Formula/nself.rb on homebrew-nself main does not match the published checksums.txt (arm formula "+
			head12(r.FArm)+" vs published "+head12(arm)+"; intel formula "+head12(r.FIntel)+" vs published "+head12(intel)+
			"). brew install would fail its checksum. Re-write the formula from the published file", "S6")
	}
	return nil
}

// GateState is gate_state: Gate = ok, pending or failed:<job name>.
func (r *Release) GateState() error {
	out, err := r.ghNeed("the jobs of run "+r.RunID, "run", "view", r.RunID, "-R", "nself-org/cli", "--json", "jobs", "--jq", gateJQ)
	r.Gate = out
	return err
}

func (r *Release) gateSettled() (bool, error) {
	if err := r.GateState(); err != nil {
		return false, err
	}
	return r.Gate != "pending", nil
}

// ReleaseGate is release_gate (S6): every release.yml job except the Homebrew
// lockstep job and the publish job must have succeeded before the formula is
// merged.
func (r *Release) ReleaseGate() error {
	if err := r.FindRun(); err != nil {
		return err
	}
	if r.RunID == "" {
		return r.die("S6", "homebrew-formula", "no release.yml run found for "+r.Tag+" (run S5 first)", "S5")
	}
	if _, err := r.poll("release.yml jobs (build, sbom, provenance, Pre-Release Gate)", 120, r.gateSettled); err != nil {
		return err
	}
	if err := r.GateState(); err != nil {
		return err
	}
	switch {
	case r.Gate == "ok":
		return nil
	case len(r.Gate) > 7 && r.Gate[:7] == "failed:":
		return r.die("S6", "homebrew-formula", "release.yml job '"+r.Gate[7:]+"' failed in run "+r.RunID+
			"; the formula is not merged for a release that cannot publish. Fix the cause and re-run the failed jobs: gh run rerun "+
			r.RunID+" -R nself-org/cli --failed")
	}
	return r.die("S6", "homebrew-formula", "release.yml jobs are still running after the wait budget (run "+r.RunID+")")
}
