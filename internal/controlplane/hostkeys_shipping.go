package controlplane

// Purpose: a "shipping run" is a deploy that sends secrets (an env file) to
//          its servers; every ssh connection of that run uses the strict
//          host-key policy, so no probe can enrol a key the secret push trusts.
// Inputs:  a context, the scoped inventory environment.
// Outputs: a marked context, or E487 before any probe, preflight or copy.
// Constraints: the check runs once per remote server, before Resolve and the
//              architecture preflight, and aborts the whole run (a host-key
//              failure is never folded into a "skipped" server).

import "context"

type shippingRunKey struct{}

// WithShippingRun marks ctx as a run that ships secrets. HostKeyOptions then
// treats every connection made with it as shipping (strict in v1.4 and v1.5).
func WithShippingRun(ctx context.Context) context.Context {
	return context.WithValue(ctx, shippingRunKey{}, true)
}

func isShippingRun(ctx context.Context) bool {
	if ctx == nil { // some callers pass a nil ctx
		return false
	}
	v, _ := ctx.Value(shippingRunKey{}).(bool)
	return v
}

// checkShippingHostKeys applies the shipping policy to every remote server of
// env and returns the first failure (E487 with the fingerprint and remedies).
func checkShippingHostKeys(ctx context.Context, name string, env Environment) error {
	if env.Kind == "local" {
		return nil
	}
	for _, srv := range env.Servers {
		if srv.Host == "" {
			continue
		}
		if _, err := HostKeyOptions(ctx, name, srv.Name, normalizedTier(env), srv.Host, true); err != nil {
			return err
		}
	}
	return nil
}
