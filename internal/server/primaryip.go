package server

// Purpose: protect (or deliberately release) a server's primary IPs before
// destroy deletes it. Design requirement 2: Hetzner primary IPs default to
// auto_delete=true, so deleting the server permanently destroys the IP too
// — the exact state all 8 of this org's production primary IPs were found
// in. Destroy must flip auto_delete=false first unless the operator
// explicitly opts into releasing the IP with --release-ip.
// Inputs: a Client, a server ID, and whether the operator asked to release.
// Outputs: which IPs were retained (auto_delete now false) vs. released
// (auto_delete left/set true), so the command layer can print an accurate
// summary — never assumed, always read back from what was actually set.

import (
	"context"
	"fmt"
)

// ProtectOrReleaseIPs reads serverID's primary IPs and, unless release is
// true, sets auto_delete=false on each so the upcoming server deletion does
// not take the IP with it. When release is true, it explicitly ensures
// auto_delete=true (Hetzner's default) so the IP is freed along with the
// server, and does so as a real, deliberate write rather than "leave it
// alone" — protecting a previous accidental protect-then-release cycle from
// ever landing on a false negative.
func ProtectOrReleaseIPs(ctx context.Context, client Client, serverID int64, release bool) (retained, released []PrimaryIP, err error) {
	ips, err := client.ListPrimaryIPs(ctx, serverID)
	if err != nil {
		return nil, nil, fmt.Errorf("list primary IPs for server %d: %w", serverID, err)
	}

	for _, ip := range ips {
		wantAutoDelete := release
		if err := client.SetPrimaryIPAutoDelete(ctx, ip.ID, wantAutoDelete); err != nil {
			return nil, nil, fmt.Errorf("set auto_delete=%v on IP %s: %w", wantAutoDelete, ip.IP, err)
		}
		ip.AutoDelete = wantAutoDelete
		if release {
			released = append(released, ip)
		} else {
			retained = append(retained, ip)
		}
	}
	return retained, released, nil
}
