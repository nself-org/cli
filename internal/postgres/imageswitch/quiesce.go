package imageswitch

// quiesce.go — stop the writers and make the old cluster read-only (EPIC D6 step 2b).
//
// Purpose: nothing writes between the dump and the switch. Every service but
// postgres stops, each database gets default_transaction_read_only = on, and
// other sessions are ended.
// Inputs: the running stack and the state. Outputs: PhaseQuiesced.
// Constraints: the stopped services are written to the state BEFORE they are
// stopped, so an abort or a rollback after a kill still restarts them. The
// setting is persisted in the old cluster's catalog; abort and rollback reset
// it. Re-running quiesce is safe.

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// quiesce stops the writers and sets the databases read-only.
func (s *Switch) quiesce(ctx context.Context) error {
	running, err := s.d.Stack.Running(ctx)
	if err != nil {
		return fmt.Errorf("listing running services: %w", err)
	}
	seen := map[string]bool{}
	for _, n := range s.st.StoppedServices {
		seen[n] = true
	}
	for _, n := range running {
		if n != s.cfg.Service && !seen[n] {
			s.st.StoppedServices = append(s.st.StoppedServices, n)
		}
	}
	sort.Strings(s.st.StoppedServices)
	s.st.ReadOnlySet = true
	if err := s.persist(""); err != nil {
		return err
	}
	if len(s.st.StoppedServices) > 0 {
		s.say("stopping %s", strings.Join(s.st.StoppedServices, ", "))
		if err := s.d.Stack.Stop(ctx, s.st.StoppedServices...); err != nil {
			return fmt.Errorf("stopping services: %w", err)
		}
	}
	for _, db := range s.st.Databases {
		sql := "ALTER DATABASE " + quoteIdent(db) + " SET default_transaction_read_only = on"
		if _, err := s.d.Runtime.ExecSQL(ctx, s.cfg.Container, s.cfg.User, "template1", sql); err != nil {
			return fmt.Errorf("setting %s read-only: %s", db, redactTail(err.Error(), 2))
		}
	}
	return s.endSessions(ctx)
}

// endSessions terminates every client session and checks none is left.
func (s *Switch) endSessions(ctx context.Context) error {
	const sel = `FROM pg_stat_activity WHERE backend_type = 'client backend' AND pid <> pg_backend_pid()`
	if _, err := s.d.Runtime.ExecSQL(ctx, s.cfg.Container, s.cfg.User, "template1", "SELECT pg_terminate_backend(pid) "+sel); err != nil {
		return fmt.Errorf("ending sessions: %s", redactTail(err.Error(), 2))
	}
	out, err := s.d.Runtime.ExecSQL(ctx, s.cfg.Container, s.cfg.User, "template1", "SELECT count(*) "+sel)
	if err != nil {
		return fmt.Errorf("checking sessions: %s", redactTail(err.Error(), 2))
	}
	if n := strings.TrimSpace(out); n != "0" {
		return fmt.Errorf("%s client sessions are still connected to the old cluster after they were ended; something reconnects", n)
	}
	return nil
}

// resetReadOnly undoes the quiesce setting on the old cluster (postgres must run on the old volume).
func (s *Switch) resetReadOnly(ctx context.Context) error {
	for _, db := range s.st.Databases {
		sql := "ALTER DATABASE " + quoteIdent(db) + " RESET default_transaction_read_only"
		if _, err := s.d.Runtime.ExecSQL(ctx, s.cfg.Container, s.cfg.User, "template1", sql); err != nil {
			return fmt.Errorf("resetting %s: %s", db, redactTail(err.Error(), 2))
		}
	}
	s.st.ReadOnlySet = false
	return nil
}
