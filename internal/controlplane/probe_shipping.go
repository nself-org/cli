package controlplane

// Purpose: the shipping mode of SSHProber. SSHReachable and DockerOK take no
//          ctx, so a shipping run cannot reach them through the context marker;
//          the prober itself carries the mode.
// Inputs:  a Prober or archReader handed to the pipeline.
// Outputs: the same value, with a *SSHProber switched to strict host keys for
//          the length of the run.
// Constraints: other Prober implementations (test doubles) are left untouched.

import (
	"sync"
	"sync/atomic"
)

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

// shippedKeyErr keeps the first host-key error that a probe met while a shipping
// run used the prober. Resolve drops probe errors (DockerOK's is not even kept
// in the status), so the run reads it back from here.
type shippedKeyErr struct {
	mu  sync.Mutex
	err error
}

// noteHostKeyErr records err when a shipping run is active on the prober.
func (sp *SSHProber) noteHostKeyErr(err error) {
	if err == nil || !sp.shipping.on() {
		return
	}
	sp.keyErr.mu.Lock()
	defer sp.keyErr.mu.Unlock()
	if sp.keyErr.err == nil {
		sp.keyErr.err = err
	}
}

// takeProbeHostKeyErr returns and clears the host-key error a shipping run's
// probes met on v (nil for any prober that is not a *SSHProber).
func takeProbeHostKeyErr(v any) error {
	sp, ok := v.(*SSHProber)
	if !ok || sp == nil {
		return nil
	}
	sp.keyErr.mu.Lock()
	defer sp.keyErr.mu.Unlock()
	err := sp.keyErr.err
	sp.keyErr.err = nil
	return err
}
