// remove_guard_testhook.go — a test hook for the bundle catalog (P7-PLUG-20).
//
// Purpose: tests of callers that run before any command has loaded the bundle
// catalog (remove <plugin>, remove <bundle>) need the "not loaded yet" state.
// Inputs: none. Outputs: the catalog is unloaded; the returned func restores it.
// Constraints: tests only; production code never calls it.
package bundle

// UnloadCatalogForTest empties the loaded bundle catalog (the state of a
// process that has not run Load yet) and returns a func that restores it.
func UnloadCatalogForTest() (restore func()) {
	stateMu.RLock()
	prev := state
	stateMu.RUnlock()
	setState(nil)
	return func() { setState(prev) }
}
