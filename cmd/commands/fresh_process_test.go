package commands

import (
	"bytes"
	"os"
	"os/exec"
	"testing"
)

const machineChild = "NSELF_MACHINE_REGISTRY_CHILD"

// inFreshProcess isolates the MCP and help integration harnesses, which
// execute commands on the process tree. Registry checks do not use it.
// The parent requires the child's sentinel and PASS line so an empty run
// cannot pass silently.
func inFreshProcess(t *testing.T, name string) (child bool) {
	t.Helper()
	if os.Getenv(machineChild) == name {
		return true
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+name+"$", "-test.count=1", "-test.v")
	cmd.Env = append(os.Environ(), machineChild+"="+name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s in a fresh process failed: %v\n%s", name, err, out)
	}
	if !bytes.Contains(out, []byte("CHECKED-"+name)) || !bytes.Contains(out, []byte("--- PASS: "+name)) {
		t.Fatalf("the fresh process did not run %s; output:\n%s", name, out)
	}
	return false
}
