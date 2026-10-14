package controlplane

// Purpose: the shipping mode of SSHProber. SSHReachable and DockerOK take no
//          ctx, so a shipping run cannot reach them through the context marker;
//          the prober itself carries the mode.
// Inputs:  a Prober or archReader handed to the pipeline.
// Outputs: the same value, with a *SSHProber switched to strict host keys for
//          the length of the run.
// Constraints: other Prober implementations (test doubles) are left untouched.

import "sync/atomic"

// shippingMode counts the shipping runs using one prober: runs may overlap, and
// a bool restored by the first run to finish would clear the second run's mode.
type shippingMode struct{ atomic.Int32 }

// on reports whether any shipping run is using the prober.
func (m *shippingMode) on() bool { return m.Load() > 0 }

// markShipping switches v to shipping mode when it is a *SSHProber and returns
// a function that leaves the mode again. Any other value is a no-op.
func markShipping(v any) (restore func()) {
	sp, ok := v.(*SSHProber)
	if !ok || sp == nil {
		return func() {}
	}
	sp.shipping.Add(1)
	return func() { sp.shipping.Add(-1) }
}
