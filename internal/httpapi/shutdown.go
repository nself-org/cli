package httpapi

// Purpose: retain invocation ownership until child cleanup and auditing finish.
// Inputs: invocation requests and the post-HTTP shutdown drain deadline.
// Outputs: invocation results and a bounded shutdown completion wait.
// Constraints: admission and drain are serialized to prevent Add/Wait races.

import (
	"context"
	"fmt"
	"time"

	"github.com/nself-org/cli/internal/invoke"
)

// invokeTracked counts the entire invoker call, including its audit write.
// Draining closes admission before Wait so no new zero-counter Add can race it.
func (s *Server) invokeTracked(ctx context.Context, path string, req invoke.Request, o invoke.Options) (invoke.Result, error) {
	s.invokeMu.Lock()
	if s.draining {
		s.invokeMu.Unlock()
		return invoke.Result{}, context.Canceled
	}
	s.invocations.Add(1)
	s.activeInvocations++
	s.invokeMu.Unlock()
	defer func() {
		s.invokeMu.Lock()
		s.activeInvocations--
		s.invocations.Done()
		s.invokeMu.Unlock()
	}()
	return s.o.Invoker(ctx, s.o.Registry, path, req, o)
}

// invocationDrainBudget closes admission and budgets for all serialized writers.
// Each audit append can try the five-second flock eight times if the file is
// rotated underneath it. Include polling slack, child cleanup (5 s), and
// scheduling slack (1 s). Reads are counted conservatively as possible writers.
func (s *Server) invocationDrainBudget() time.Duration {
	s.invokeMu.Lock()
	defer s.invokeMu.Unlock()
	s.draining = true
	return 6*time.Second + time.Duration(s.activeInvocations)*41*time.Second
}

// drainInvocations waits for cleanup and audit completion. A cap expiry logs once.
func (s *Server) drainInvocations(limit time.Duration) {
	s.invokeMu.Lock()
	s.draining = true
	s.invokeMu.Unlock()
	done := make(chan struct{})
	go func() {
		s.invocations.Wait()
		close(done)
	}()
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		s.logMu.Lock()
		_, _ = fmt.Fprintln(s.o.Log, "nself: HTTP shutdown invocation drain timed out")
		s.logMu.Unlock()
	}
}
