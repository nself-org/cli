package repoqa

import "testing"

// dockerBasisTotal is the hard-coded total of testdata/docker-exec-allowlist.txt
// (rule D4). The allowlist may only shrink: lowering a line without lowering
// this constant fails, and raising either needs a reviewed edit of both.
const dockerBasisTotal = 133

// TestDockerFunnel freezes direct docker process execution outside
// internal/docker (EPIC G1, G4).
func TestDockerFunnel(t *testing.T) {
	runFunnelRatchet(t, funnelSpec{
		kind: "docker", funnel: dockerFunnelDir, list: "testdata/docker-exec-allowlist.txt",
		basisName: "dockerBasisTotal", basisFile: "docker_funnel_test.go", basis: dockerBasisTotal,
		pick: func(_, d countList) countList { return d },
	})
}
