package simharness

// Purpose: tear live fleets down when the test process is interrupted (SIGINT, SIGTERM), when t.Cleanup cannot run.
// Inputs: the set of live fleets, maintained by Start and Close.
// Outputs: containers and networks removed, then exit status 128+signal.
// Constraints: the handler is installed only while a fleet is live; it removes only registered fleets.

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var (
	liveMu sync.Mutex
	live   = map[*Fleet]struct{}{}
	stopCh chan os.Signal
)

// register adds f to the live set and installs the signal handler on first use.
func register(f *Fleet) {
	liveMu.Lock()
	defer liveMu.Unlock()
	live[f] = struct{}{}
	if stopCh != nil {
		return
	}
	stopCh = make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)
	go handle(stopCh)
}

// unregister removes f and uninstalls the handler when no fleet is live.
func unregister(f *Fleet) {
	liveMu.Lock()
	defer liveMu.Unlock()
	delete(live, f)
	if len(live) == 0 && stopCh != nil {
		signal.Stop(stopCh)
		close(stopCh)
		stopCh = nil
	}
}

func handle(ch chan os.Signal) {
	sig, ok := <-ch
	if !ok {
		return
	}
	liveMu.Lock()
	fleets := make([]*Fleet, 0, len(live))
	for f := range live {
		fleets = append(fleets, f)
	}
	liveMu.Unlock()
	var wg sync.WaitGroup
	for _, f := range fleets {
		wg.Add(1)
		go func(f *Fleet) {
			defer wg.Done()
			f.closeWith(func(string, ...any) {})
		}(f)
	}
	wg.Wait()
	code := 130
	if sig == syscall.SIGTERM {
		code = 143
	}
	os.Exit(code)
}
