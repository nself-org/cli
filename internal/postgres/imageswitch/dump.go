package imageswitch

// dump.go — capture and dump the old cluster inside one exported snapshot per database.
//
// Purpose: a one-shot of the target image (so pg_dump matches the server major)
// connects to the old postgres over the project network, exports a snapshot per
// database, captures the database inside it and dumps the same snapshot.
// Inputs: the quiesced cluster. Outputs: PhaseDumped; globals.old.sql,
// cluster.old.json and per database db-N.dump, db-N.cap.old.json, db-N.schema.old.sql.
// Constraints: the password is PGPASSWORD through EnvPass; the one-shot runs as
// the host uid:gid with every capability dropped, so stage files belong to the host user.

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/docker"
)

// oldClusterSpec is the one-shot that talks to the OLD cluster (dump and recheck).
func (s *Switch) oldClusterSpec(role, script string) docker.RunSpec {
	uid, gid := hostIDs()
	env := map[string]string{"PGHOST": s.cfg.Container, "PGUSER": s.cfg.User, "PGPASSWORD": s.cfg.Password}
	return docker.RunSpec{
		Image: s.st.NewImage, Name: s.oneShot(role), Network: s.st.Network, User: strconv.Itoa(uid) + ":" + strconv.Itoa(gid),
		EnvPass: env, Args: []string{"bash", "/stage/" + script},
		Mounts:  []docker.Mount{{Source: s.stage, Destination: "/stage"}},
		CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges:true"},
		Labels: map[string]string{"org.nself.image-switch": s.st.RunID},
	}
}

// dump runs dump.sh and checks the files it must have written.
func (s *Switch) dump(ctx context.Context) error {
	s.clearOneShots(ctx)
	if err := s.cleanStage(); err != nil {
		return err
	}
	if err := s.prepareStage(); err != nil {
		return err
	}
	if err := writeStage(s.stage, "dblist.txt", []byte(dblist(s.st.Databases))); err != nil {
		return err
	}
	s.say("dumping %d database(s) from %s", len(s.st.Databases), s.st.OldVolume)
	if _, stderr, err := s.d.Runtime.RunOneShot(ctx, s.oldClusterSpec("dump", "dump.sh")); err != nil {
		return fmt.Errorf("the dump failed: %s", s.redact(stderr+"\n"+err.Error(), 4))
	}
	want := []string{"globals.old.sql", "cluster.old.json"}
	for i := range s.st.Databases {
		want = append(want, dbFile(i, "dump"), dbFile(i, "cap.old.json"), dbFile(i, "schema.old.sql"))
	}
	for _, name := range want {
		if err := nonEmpty(filepath.Join(s.stage, name)); err != nil {
			return fmt.Errorf("the dump is incomplete: %w", err)
		}
	}
	raw, err := s.readStage("cluster.old.json")
	if err != nil {
		return err
	}
	cl, err := parseCluster(raw)
	if err != nil {
		return err
	}
	if got, want := strings.Join(cl.names(), "\n"), strings.Join(s.st.Databases, "\n"); got != want {
		return coded("E510", "the databases changed while they were dumped", "the dump covers "+strings.ReplaceAll(want, "\n", ", ")+" but the cluster now has "+strings.ReplaceAll(got, "\n", ", "), "")
	}
	return nil
}
