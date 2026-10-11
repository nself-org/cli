//go:build windows

package deploy

import "os/exec"

// Windows has no process groups in this sense: nothing to do.
func detachProcessGroup(*exec.Cmd) {}
