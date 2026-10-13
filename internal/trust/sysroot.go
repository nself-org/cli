package trust

import "path/filepath"

// sysRoot prefixes every system path the trust code reads or writes directly
// (/etc/resolver, /etc/pf.anchors, /Library/LaunchDaemons, dnsmasq.conf,
// /etc/systemd, /etc/iptables). Production leaves it empty. Tests point it at
// a temp directory so they never touch the real machine.
var sysRoot string

// sysPath maps an absolute system path under sysRoot.
func sysPath(p string) string {
	if sysRoot == "" {
		return p
	}
	return filepath.Join(sysRoot, p)
}
