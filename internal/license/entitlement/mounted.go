package entitlement

import (
	"context"
	"sync"
	"time"
)

var mountProbeMu sync.RWMutex
var mountProbe func(context.Context, string) (bool, error)

// SetMountProbe supplies host inspection from the application's composition
// root. Library users without a probe conservatively preserve existing inodes.
func SetMountProbe(probe func(context.Context, string) (bool, error)) {
	mountProbeMu.Lock()
	defer mountProbeMu.Unlock()
	mountProbe = probe
}

// Preserve mounted inodes, including when the host cannot establish their status.
var entitlementMounted = func(path string) bool {
	mountProbeMu.RLock()
	probe := mountProbe
	mountProbeMu.RUnlock()
	if probe == nil {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mounted, err := probe(ctx, path)
	return err != nil || mounted
}
