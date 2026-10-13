//go:build darwin

package trust

import "context"

func adminProbe(context.Context) error { return setupResolver() }
