package docker

import (
	"context"
	"sync"
	"time"
)

// postStartCleanups tracks the background post-start cleanups, so a test that starts one can wait
// before its docker stub and environment are torn down.
var postStartCleanups sync.WaitGroup

// StartPostStartCleanup runs RunPostStartCleanup in the background with a 30 s budget; errors are
// best-effort and dropped, as after every start.
func StartPostStartCleanup(projectName string) {
	postStartCleanups.Add(1)
	go func() {
		defer postStartCleanups.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = RunPostStartCleanup(ctx, projectName)
	}()
}

// WaitPostStartCleanups blocks until every background post-start cleanup has returned.
func WaitPostStartCleanups() { postStartCleanups.Wait() }
