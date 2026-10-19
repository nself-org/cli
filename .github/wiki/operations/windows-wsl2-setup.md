# Windows WSL2 Setup (Operations)

This is the operations runbook for running ɳSelf on Windows. The full step-by-step user guide lives at [[windows-wsl2]] — this page covers the operational decisions, prerequisites, and troubleshooting checklist.

## Why WSL2 is required

Native Windows binaries ship from v1.0.12 (P97 G3-T01). They build, run `nself --version`, and execute pure-Go commands. They do **not** support the trust subsystem:

| Subsystem | Native Windows | WSL2 |
|---|---|---|
| `nself init` / `build` | Yes | Yes |
| `nself plugin` | Yes | Yes |
| `nself trust install` (DNS, mkcert, ports) | No (returns "unsupported OS") | Yes |
| `nself start` (Docker) | Requires Docker Desktop with WSL backend | Yes |
| `*.local.nself.org` resolution | Requires manual hosts edits | Automatic via dnsmasq |

WSL2 is required for any local-dev workflow that involves Docker containers or local TLS.

## Prerequisites

- Windows 10 22H2 (build 19045+) or Windows 11
- Hyper-V capable CPU (most x64 / arm64 since 2017)
- 8 GB RAM minimum, 16 GB recommended
- 20 GB free disk

## Install order

1. **Enable WSL2.** Elevated PowerShell: `wsl --install`. Reboot when prompted.
2. **Install Ubuntu 22.04 LTS.** `wsl --install -d Ubuntu-22.04`. Launch once to set username + password.
3. **Install Docker Desktop.** Enable "Use WSL 2 based engine" during install. After install: Settings -> Resources -> WSL Integration -> toggle Ubuntu-22.04 ON.
4. **Install ɳSelf inside Ubuntu.** `brew tap nself-org/nself && brew install nself` (Linuxbrew).
5. **Verify.** `nself version` -> expect `nself vX.Y.Z (linux/amd64)` (or current).

Full transcript: [[windows-wsl2]].

## Prerequisites checklist (for support / triage)

When a Windows user reports an issue, confirm in this order:

- [ ] `wsl --version` shows `WSL version: 2.x.x` and `Kernel version: 5.15+`.
- [ ] `wsl -l -v` lists Ubuntu-22.04 with VERSION 2.
- [ ] `docker ps` runs inside Ubuntu without `permission denied` (user is in `docker` group).
- [ ] `nself doctor` inside Ubuntu reports clean.
- [ ] DNS: `dig ummat.local.nself.org @127.0.0.1` returns 127.0.0.1 (after `nself trust install`).
- [ ] Network: from Windows browser, `https://ummat.local.nself.org:8443` loads through WSL2 port forwarding.

## Known Windows-specific issues

| Symptom | Cause | Fix |
|---|---|---|
| `docker: command not found` inside WSL | WSL Integration not enabled in Docker Desktop | Settings -> Resources -> WSL Integration -> toggle target distro |
| `localhost:8443` not reachable from Windows browser | WSL2 NAT didn't forward port | Restart Docker Desktop, or `wsl --shutdown` then relaunch |
| `nself trust install` runs on native binary and reports "unsupported OS" | User installed via `install.ps1` and ran outside WSL | Re-run inside Ubuntu shell |
| File ops on `/mnt/c/...` are 10x slower | WSL2 9P filesystem overhead | Move project to `~/projects` inside WSL home |

## Native Windows binary scope

The Windows binary released at v1.0.12 is a thin shim that supports:

- `nself --version`, `nself --help`
- `nself init`, `nself build` (generates compose; does not start it)
- `nself plugin list`, `install`, `remove` (registry + license operations)
- All commands that don't shell out to `mkcert`, `dnsmasq`, `pfctl`, `iptables`, `osascript`, or `launchctl`

It is intended for CI smoke tests, plugin-author tooling, and users who already have a remote Linux nself stack and only need the CLI client. For local-dev, WSL2 is required.

## Cross-references

- [[windows-wsl2]] — full user-facing install guide
- [[Installation]] — multi-platform install matrix
- `.github/wiki/operations/release-cascade.md` — release workflow including Windows artifact publishing
- `cli/.github/workflows/windows-smoke.yml` — manual CI smoke

[[Home]]
