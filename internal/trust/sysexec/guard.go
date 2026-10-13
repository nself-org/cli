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
// it is a read-only probe. It matches on the base name of the command so
// "/usr/bin/osascript" is caught too.
func Privileged(name string, args []string) string {
	base := filepath.Base(name)
	switch base {
	case "osascript", "sudo", "doas", "pkexec":
		return "admin prompt"
	case "killall", "dscacheutil", "update-ca-certificates", "service", "visudo":
		return "system change"
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
	case "brew":
		if has("install", "uninstall", "reinstall", "upgrade", "services") {
			return "package or service change"
		}
	case "security":
		if has("add-trusted-cert", "remove-trusted-cert", "add-certificates", "delete-certificate") {
			return "trust store change"
		}
	case "launchctl":
		if has("load", "unload", "bootstrap", "bootout", "enable", "disable", "kickstart") {
			return "launchd change"
		}
	case "pfctl":
		if has("-e", "-ef", "-f", "-d", "-F") {
			return "pf change"
		}
	case "mkcert":
		if has("-install", "-uninstall") {
			return "trust store change"
		}
	case "certutil":
		if has("-addstore", "-delstore") {
			return "trust store change"
		}
	case "systemctl":
		if has("restart", "start", "stop", "enable", "disable", "reload", "daemon-reload") {
			return "systemd change"
		}
	case "iptables", "ip6tables":
		if has("-A", "-I", "-D", "-R", "-F", "-X", "-P") {
			return "firewall change"
		}
	}
	return ""
}
