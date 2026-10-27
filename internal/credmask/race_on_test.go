//go:build race

package credmask

// raceEnabled: the race detector shadows allocations, so allocation counts and
// bytes are not comparable across sizes; TestSchemelessLinear checks steps only.
const raceEnabled = true
