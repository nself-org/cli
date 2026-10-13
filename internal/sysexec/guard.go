package sysexec

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// guard panics when a Go test binary reaches the real exec runner with a
// command that prompts for admin rights or changes system state. Unit tests
// must install a fake with Set; a panic here is the failure that tells the
// author which test forgot to.
func guard(name string, args []string) {
	if !testing.Testing() {
		return
	}
	if why := Privileged(name, args); why != "" {
		panic(fmt.Sprintf("sysexec: unit test reached the real runner with a privileged command (%s): %s %s — install a fake runner with sysexec.Set",
			why, name, strings.Join(args, " ")))
	}
}

// Privileged reports why a command is not allowed from a unit test, or "" when
// it is a known read-only probe. It is an allowlist: anything not listed
// (including shells, env, tee, cp and any command added later) is refused, so a
// new privileged call fails loudly instead of passing by default. It matches
// on the base name of the command so "/usr/bin/osascript" is caught too.
func Privileged(name string, args []string) string {
	base := filepath.Base(name)
	switch base {
	case "osascript", "sudo", "doas", "pkexec":
		return "admin prompt"
	}
	if readOnlyProbe(base, args) {
		return ""
	}
	return "not on the read-only probe allowlist"
}

// readOnlyProbe reports whether base+args only reads system state.
func readOnlyProbe(base string, args []string) bool {
	first := ""
	if len(args) > 0 {
		first = args[0]
	}
	has := func(words ...string) bool {
		for _, a := range args {
			for _, w := range words {
				if a == w {
					return true
				}
			}
		}
		return false
	}
	switch base {
	case "true", "false", "echo":
		return true
	case "brew":
		return first == "list" || first == "--prefix" || first == "info"
	case "security":
		return first == "verify-cert" || first == "find-certificate"
	case "pfctl":
		for _, a := range args {
			if strings.HasPrefix(a, "-s") {
				return !has("-e", "-E", "-ef", "-f", "-d", "-F", "-N", "-t")
			}
		}
		return false
	case "launchctl":
		return first == "print" || first == "list" || first == "blame"
	case "mkcert":
		return len(args) == 1 && first == "-CAROOT"
	case "iptables", "ip6tables":
		return has("-C", "--check", "-S", "--list-rules", "-L", "--list") &&
			!has("-A", "--append", "-I", "--insert", "-D", "--delete", "-R", "--replace",
				"-F", "--flush", "-X", "--delete-chain", "-P", "--policy", "-N", "--new-chain", "-E", "--rename-chain", "-Z", "--zero")
	case "iptables-save", "ip6tables-save":
		return !has("-f", "--file")
	case "systemctl":
		return first == "is-active" || first == "is-enabled" || first == "status" || first == "show"
	case "certutil":
		return first == "-store" || first == "-L"
	}
	return false
}
