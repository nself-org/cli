//go:build windows

package docker

// compose_states_windows.go — no soft cancel on Windows.

import "os/exec"

// softCancel keeps exec's default Kill: Windows has no SIGTERM to send.
func softCancel(*exec.Cmd) {}
