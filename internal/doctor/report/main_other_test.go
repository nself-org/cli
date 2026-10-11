//go:build windows

package report

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	cleanup := isolateHome()
	code := m.Run()
	cleanup()
	os.Exit(code)
}
