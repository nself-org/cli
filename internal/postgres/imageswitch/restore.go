package imageswitch

// restore.go — restore into the NEW volume inside a one-shot server (EPIC D6 step 5).
//
// Purpose: the data moves to a fresh volume of the target image. The one-shot
// runs initdb, starts a server with no network and no initdb.d (so the
// np_plugins seed of postgres/init cannot collide with the restored data),
// restores the roles and every database with pg_restore --exit-on-error,
// replays database ACLs and settings, captures the result and stops cleanly.
// Inputs: the dump in the stage. Outputs: the new volume, PhaseRestored.
// Constraints: the only skipped statement is CREATE ROLE of the bootstrap role
// (logged); any other error fails the restore (E511). The one-shot runs as root so
// it can read the 0600 stage; it hands the stage back to the host uid:gid on exit.

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/nself-org/cli/internal/docker"
)

// volumeLabels are what compose puts on the volume it creates, so `compose up`
// accepts the volume this command made instead of warning that it is foreign.
func (s *Switch) volumeLabels() map[string]string {
	return map[string]string{
		"com.docker.compose.project": s.st.ComposeProject,
		"com.docker.compose.volume":  "postgres_data",
		"org.nself.image-switch":     s.st.RunID,
	}
}

// restore creates the new volume and fills it.
func (s *Switch) restore(ctx context.Context) error {
	s.clearOneShots(ctx)
	if err := s.freshNewVolume(ctx); err != nil {
		return err
	}
	globals, err := s.readStage("globals.old.sql")
	if err != nil {
		return err
	}
	clusterJSON, err := s.readStage("cluster.old.json")
	if err != nil {
		return err
	}
	cl, err := parseCluster(clusterJSON)
	if err != nil {
		return err
	}
	rf := buildRestoreFiles(globals, cl)
	s.st.Tolerated = rf.Tolerated
	for name, body := range map[string]string{"globals.restore.sql": rf.Globals, "create-dbs.sql": rf.CreateDBs, "replay.sql": rf.Replay} {
		if err := writeStage(s.stage, name, []byte(body)); err != nil {
			return err
		}
	}
	collate, ctype := cl.collation()
	uid, gid := hostIDs()
	spec := docker.RunSpec{
		Image: s.st.NewImage, Name: s.oneShot("restore"), User: "0:0", Network: "none",
		Args: []string{"bash", "/stage/restore.sh"},
		EnvPass: map[string]string{
			"BOOT_USER": cl.Bootstrap, "LC_COLLATE": collate, "LC_CTYPE": ctype,
			"HOST_UID": strconv.Itoa(uid), "HOST_GID": strconv.Itoa(gid),
		},
		Mounts: []docker.Mount{
			{Source: s.stage, Destination: "/stage"},
			{Source: s.st.NewVolume, Destination: "/var/lib/postgresql/data"},
		},
		Labels: map[string]string{"org.nself.image-switch": s.st.RunID},
	}
	s.say("restoring into the new volume %s", s.st.NewVolume)
	if _, stderr, err := s.d.Runtime.RunOneShot(ctx, spec); err != nil {
		return coded("E511", "the restore into the new volume failed",
			redactTail(stderr+"\n"+err.Error(), 4), "")
	}
	want := []string{"cluster.new.json", "globals.new.sql"}
	for i := range s.st.Databases {
		want = append(want, dbFile(i, "cap.new.json"), dbFile(i, "schema.new.sql"))
	}
	for _, name := range want {
		if err := nonEmpty(filepath.Join(s.stage, name)); err != nil {
			return coded("E511", "the restore did not finish", err.Error(), "")
		}
	}
	return nil
}

// freshNewVolume makes sure the new volume exists and is empty and is ours.
func (s *Switch) freshNewVolume(ctx context.Context) error {
	exists, err := s.d.Runtime.VolumeExists(ctx, s.st.NewVolume)
	if err != nil {
		return fmt.Errorf("checking the new volume: %w", err)
	}
	if exists && !s.st.NewVolumeCreated {
		return coded("E509", "the new volume name is already taken",
			s.st.NewVolume+" exists and this switch did not create it; it will not be reused or removed", "Remove or rename that volume by hand, then run the switch again.")
	}
	// Owned before created: a kill between the two still lets abort remove it.
	s.st.NewVolumeCreated = true
	if err := s.persist(""); err != nil {
		return err
	}
	if exists { // a resumed restore starts from an empty volume
		if err := s.d.Runtime.VolumeRemove(ctx, s.st.NewVolume); err != nil {
			return fmt.Errorf("removing the half-restored volume: %w", err)
		}
	}
	if err := s.d.Runtime.VolumeCreate(ctx, s.st.NewVolume, s.volumeLabels()); err != nil {
		return fmt.Errorf("creating the new volume: %w", err)
	}
	return nil
}

// removeNewVolume removes the new volume if this switch created it.
func (s *Switch) removeNewVolume(ctx context.Context) error {
	if !s.st.NewVolumeCreated {
		return nil
	}
	if err := s.d.Runtime.VolumeRemove(ctx, s.st.NewVolume); err != nil {
		return fmt.Errorf("removing the new volume %s: %w", s.st.NewVolume, err)
	}
	s.st.NewVolumeCreated = false
	return nil
}
