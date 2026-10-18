package compose

// images_lock_monitoring_test.go — the monitoring pins in the cli image lock
// agree with the plugins image lock fragment images.d/monitoring.json (D-0935,
// P7-PLUG-58).
//
// Purpose: free/monitoring pins tempo 2.10.8 and alloy v1.20.1 by index digest.
// The cli lock must carry the same pins, so enforcing either lock gives one
// answer, and must not keep promtail, which Alloy replaced.
// Constraints: no network; the lock is the committed file.

import "testing"

func TestMonitoringLockMatchesPluginsFragment(t *testing.T) {
	for _, want := range []struct{ name, repo, version, digest string }{
		{"tempo", "docker.io/grafana/tempo", "2.10.8", "sha256:f0561deb1c68ec44d6e6e7e4487f30106c4e5e768642077695b37958b105812a"},
		{"alloy", "docker.io/grafana/alloy", "v1.20.1", "sha256:2aa2099af76c0098d4af7a4d6e48f86cb66dc1a000222ad927a1c67c6542d13f"},
	} {
		r, ok := LockedRef(want.name)
		if !ok {
			t.Errorf("lock has no %q entry", want.name)
			continue
		}
		if r.Repository != want.repo || r.Version != want.version || r.IndexDigest != want.digest {
			t.Errorf("%s = %s, want %s:%s@%s", want.name, r.String(), want.repo, want.version, want.digest)
		}
		if r.Role != RoleMonitoring {
			t.Errorf("%s role = %q, want %q", want.name, r.Role, RoleMonitoring)
		}
	}
	if _, ok := LockedRef("promtail"); ok {
		t.Error("lock still has a promtail entry; Alloy replaced it")
	}
}
