package controlplane

// Purpose: the shipping mode of SSHProber. SSHReachable and DockerOK take no
//          ctx, so a shipping run cannot reach them through the context marker;
//          the prober itself carries the mode.
// Inputs:  a Prober or archReader handed to the pipeline.
// Outputs: the same value, with a *SSHProber switched to strict host keys for
//          the length of the run.
// Constraints: other Prober implementations (test doubles) are left untouched.

import "sync/atomic"

// shippingMode is an atomic flag: probes run concurrently.
type shippingMode struct{ atomic.Bool }

// markShipping switches v to shipping mode when it is a *SSHProber and returns
// a function that restores the previous mode. Any other value is a no-op.
func markShipping(v any) (restore func()) {
	sp, ok := v.(*SSHProber)
	if !ok || sp == nil {
		return func() {}
	}
	prev := sp.shipping.Swap(true)
	return func() { sp.shipping.Store(prev) }
}
