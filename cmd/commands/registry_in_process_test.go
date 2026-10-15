package commands

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRegistryChecksRunInProcess exercises the registry checks with no test
// executable to relaunch. The checks must still run after env-target tests
// have used their isolated command copies, including on repeated runs.
func TestRegistryChecksRunInProcess(t *testing.T) {
	executable := os.Args[0]
	os.Args[0] = filepath.Join(t.TempDir(), "no-test-executable")
	t.Cleanup(func() { os.Args[0] = executable })
	// Do not let a caller's old child markers bypass a subprocess wrapper.
	t.Setenv("NSELF_REGISTRY_GOLDEN_CHILD", "")
	t.Setenv("NSELF_MACHINE_REGISTRY_CHILD", "")
	t.Run("env-target", TestFreshCmdLeavesSharedTreeIntact)
	t.Run("golden", TestRegistryGolden)
	t.Run("machine", TestMachineRegistry)
	t.Run("plugin-mount", TestMachineRegistryPluginMount)
	t.Run("mounted-plugins", TestMachineRegistryKeepsMountedPlugins)
	t.Run("env-restored", TestMachineRegistryEnvRestored)
}
