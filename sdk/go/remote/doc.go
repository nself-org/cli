// Package remote is the one place nSelf runs ssh, scp, rsync and ssh-keyscan.
//
// Purpose:
//
//	Give the CLI, the ci plugin and every future caller a single, audited exec
//	funnel for remote work, plus the pieces that make that work safe: remote
//	path validation, a pinned known_hosts file, the CI option set and an
//	`ssh -G` host resolver.
//
// Rules this package enforces:
//
//   - One funnel per tool. Every process the package starts is created by
//     Command in exec.go from an argv slice. No shell is ever invoked locally,
//     and no command string is built locally by concatenation. Run passes the
//     caller's remote command as one argv element (ssh itself hands it to the
//     remote shell); RunArgv quotes each element with ShellQuote first.
//   - Operands after options. With the default Target (Compat false) the
//     destination, source and remote path come after a "--" separator, the
//     destination and the remote path are validated before any exec, and the
//     process environment is EnvAllowlist(). Compat reproduces cli deploy's
//     historical argv and inherited environment byte for byte and exists only
//     for internal/deploy until its host-key policy changes (P7-DEPL D13).
//   - Remote paths are an allowlist: ValidateRemotePath accepts only
//     [a-zA-Z0-9/_.-], never a leading "-", never a ".." segment.
//   - The CI option set (CISSHFlags, CIOptions) turns off agent, X11 and port
//     forwarding, multiplexing, local and remote commands, DNS and
//     command-based host keys, and sets StrictHostKeyChecking=yes against a
//     pinned known_hosts file. -o options win over ssh_config, so the
//     operator's configuration cannot re-enable them.
//   - PinnedHostKeys reads and writes only the file it was given. It never
//     touches ~/.ssh/known_hosts.
//
// Constraints: standard library only; no cgo; no network access of its own.
package remote
