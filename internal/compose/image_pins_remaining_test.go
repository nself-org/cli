package compose

import "testing"

func TestRemainingImagePins(t *testing.T) {
	for _, tc := range []struct{ name, version string }{
		{"cadvisor", "v0.49.2"},
		{"elasticsearch", "8.11.4"},
		{"grafana", "11.1.13"},
		{"loki", "3.1.2"},
		{"mlflow", "v2.10.2"},
		{"node-exporter", "v1.8.2"},
		{"otel-collector-contrib", "0.102.1"},
		{"prometheus", "v2.53.5"},
		{"promtail", "3.1.2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, ok := LockedRef(tc.name)
			if !ok || r.Version != tc.version {
				t.Fatalf("locked version = %q (present %v), want %q", r.Version, ok, tc.version)
			}
			setMode(t, PinningLock)
			if got := ImageRef(tc.name, r.LegacyVersion()); got != r.String() {
				t.Errorf("default image = %q, want %q", got, r.String())
			}
			setMode(t, PinningLegacy)
			if got := ImageRef(tc.name, ""); got != r.LegacyRef {
				t.Errorf("legacy image = %q, want %q", got, r.LegacyRef)
			}
		})
	}
}
