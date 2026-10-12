package deploy

// Purpose: the fronting stack's ssl layout in the target snapshot (D-0425). A
//          project fronted by another stack's nginx is served with the
//          certificates under <front>/ssl/certificates, so the scratch build's
//          stub ssl tree for the fronting stack mirrors those names, and the
//          apply checks they did not change since the snapshot.
// Inputs:  a Remote and the front root (path.Dir(remote_path)).
// Outputs: the certificate dirs and which PEM files each holds.
// Constraints: file names only (find), never a PEM's content and never key
//              material; read only, no writes, nothing sourced.

import (
	"context"
	"fmt"
	"path"
	"strings"
)

// TakeFrontSSL lists the certificate dirs of the fronting stack at front.
func TakeFrontSSL(ctx context.Context, r Remote, front string) ([]CertDir, error) {
	pems, err := r.Run(ctx, shScript(fmt.Sprintf(`find -L %s -maxdepth 2 -name '*.pem' -print 2>/dev/null || true`, q(path.Join(front, "ssl", "certificates")))))
	if err != nil {
		return nil, fmt.Errorf("target snapshot: listing the fronting stack's certificates: %w", err)
	}
	return parsePEMNames(string(pems))
}

// frontSSLDrift lists the differences between the fronting stack's certificate
// dirs the build used (want) and the ones the host has now (got).
func frontSSLDrift(want, got []CertDir) []string {
	diffs := LayoutDrift(TargetSnapshot{SSL: want}, TargetSnapshot{SSL: got})
	for i, d := range diffs {
		diffs[i] = "fronting stack: " + strings.TrimSpace(d)
	}
	return diffs
}
