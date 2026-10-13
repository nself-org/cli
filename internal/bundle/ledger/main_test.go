package ledger

import (
	"testing"
	"time"

	"github.com/nself-org/cli/internal/testwatch"
)

// TestMain bounds the package: a lock that is never released fails the run in
// 90 s with a goroutine dump instead of waiting for go test's 10-minute limit.
func TestMain(m *testing.M) { testwatch.Main(m, 90*time.Second) }
