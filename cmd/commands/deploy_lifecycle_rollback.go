package commands

// Purpose: `nself deploy rollback` for a v1.5 remote apply (rollbackRemoteV15),
//          split out of deploy_lifecycle.go to keep that file under the size ratchet.
//          It restores the pre-deploy tar of every host the record names, the
//          fronting stack's nginx/sites from the recorded front tar (D-0425), and
//          the database backup on backup_host only.
// Constraints: unchanged behaviour for a record without front fields.

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/sdk/go/v2/remote"

	"github.com/spf13/cobra"
)

// rollbackRemoteV15 undoes the last v1.5 remote apply recorded for target on every
// host that kept a pre-deploy tar, restoring the database backup on backup_host
// only. handled=false (no such record) keeps the old rollback. A record already
// rolled back is refused (the database would be restored twice).
func rollbackRemoteV15(cmd *cobra.Command, workdir, target string) (bool, error) {
	st, err := deploy.LoadDeployState(workdir)
	if err != nil && target != "local" { // unreadable or corrupt: never fall back blind (D-0144)
		return true, fmt.Errorf("deploy rollback: %w; nothing was changed (fix or remove %s, then run it again)", err, deploy.DeployStateFile)
	}
	tars := st.PreDeployTars
	if target == "local" || len(tars) == 0 || st.Target != target {
		return false, nil
	}
	if st.RolledBack {
		return true, fmt.Errorf("deploy rollback: the last deploy of %s was already rolled back; nothing was changed (deploy again to get a new rollback point)", target)
	}
	ctx, stop := deploy.WithInterrupt(cmd.Context())
	defer stop()
	inv, _ := controlplane.Load(workdir)
	hosts := make([]string, 0, len(tars))
	for h := range tars {
		hosts = append(hosts, h)
	}
	sort.Slice(hosts, func(i, j int) bool { // the database host first, then by name
		if a, b := hosts[i] == st.BackupHost, hosts[j] == st.BackupHost; a != b {
			return a
		}
		return hosts[i] < hosts[j]
	})
	for _, host := range hosts { // every record is checked before any host is opened
		if len(st.PreDeployChanged[host]) == 0 {
			return true, fmt.Errorf("deploy rollback: the record for %s has no list of changed paths; it cannot be rolled back safely (nothing was changed)", host)
		}
		if strings.Index(tars[host], "/"+deploy.BackupDir+"/pre-deploy-") <= 0 {
			return true, fmt.Errorf("deploy rollback: the recorded tar %q for %s is not a pre-deploy tar; nothing was changed", tars[host], host)
		}
	}
	manualDB := st.BackupID != "" && tars[st.BackupHost] == ""
	if manualDB {
		fmt.Printf("  [warn] database backup %s was not restored: %s kept no changes of this deploy. If the database must go back, run on %s: nself backup restore %s --yes\n", st.BackupID, st.BackupHost, st.BackupHost, st.BackupID)
	}
	var failed []string
	for _, host := range hosts {
		tar := tars[host]
		idx := strings.Index(tar, "/"+deploy.BackupDir+"/pre-deploy-")
		srv, key := controlplane.Server{Name: target + "-app", Host: host, RemotePath: tar[:idx]}, sshKeyPath()
		if inv != nil {
			for _, c := range inv.Environments[target].Servers {
				if spec, err := remote.ParseHostSpec(c.Host); err == nil && spec.String() == host {
					srv.Name, srv.Host = c.Name, c.Host
					if v := os.Getenv(c.SSHKeyRef); v != "" {
						key = v
					}
				}
			}
		}
		r, err := openDeployRemote(ctx, srv, key, target)
		if err == nil {
			backupID := ""
			if host == st.BackupHost {
				backupID = st.BackupID
			}
			err = deploy.RollbackRemote(ctx, deploy.RollbackOptions{
				Remote: r, RemotePath: srv.RemotePath, Tar: tar, BackupID: backupID,
				Changed: st.PreDeployChanged[host], Absent: st.PreDeployAbsent[host],
				FrontTar: st.FrontTars[host], FrontChanged: st.FrontChanged[host], FrontAbsent: st.FrontAbsent[host],
				DBRestored: st.DBRestoredAt != "", StrictHasura: hasuraStrictFor(workdir, target),
				OnDBRestored: func() error {
					st.DBRestoredAt = time.Now().UTC().Format(time.RFC3339)
					return deploy.SaveDeployState(workdir, st)
				},
				Clock: deployGateClock, Out: os.Stdout,
			})
		}
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", host, err))
		}
	}
	if len(failed) > 0 {
		return true, fmt.Errorf("deploy rollback failed on %d host(s): %s", len(failed), strings.Join(failed, "; "))
	}
	st.RolledBack = true
	if !manualDB {
		st.BackupID = ""
	}
	_ = deploy.SaveDeployState(workdir, st)
	return true, nil
}
