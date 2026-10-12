// store_testhook.go — a write-failure hook for tests (D-0368).
//
// Purpose: tests of callers need to fail one specific write (for example the
// final ledger write after a checkpoint succeeded).
// Inputs: a func called with the target path before every write to the store.
// Outputs: an error from the func fails that write before any temp file exists.
// Constraints: tests only; production code never calls it.
package ledger

// SetWriteHookForTest installs fn to run before every ledger or intent write.
// nil removes it.
func (s *Store) SetWriteHookForTest(fn func(path string) error) { s.writeHook = fn }
