package commands

// Purpose: probe every platform digest in the image lock and report upstream
// and mirror availability without changing images or project state.
// Inputs: the embedded image lock and Docker's manifest inspection funnel.
// Outputs: one row per image and the regular doctor status exit code.
// Constraints: registry/network failures are never reported as passing.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

var doctorImagesCmd = &cobra.Command{
	Use:   "images",
	Short: "Check availability of every locked image",
	RunE:  runDoctorImages,
}

func init() { doctorCmd.AddCommand(doctorImagesCmd) }

type imageProbe struct {
	Name, Upstream, Mirror, Status, Remedy string
}

type manifestProbe func(context.Context, string, bool) ([]byte, error)

func probeRegistry(ctx context.Context, repository string, ref compose.Ref, inspect manifestProbe) string {
	if repository == "" {
		return "not configured"
	}
	digests := make([]string, 0, len(ref.Platforms)+1)
	digests = append(digests, ref.IndexDigest)
	platforms := make([]string, 0, len(ref.Platforms))
	for platform := range ref.Platforms {
		platforms = append(platforms, platform)
	}
	sort.Strings(platforms)
	for _, platform := range platforms {
		digests = append(digests, ref.Platforms[platform])
	}
	for _, digest := range digests {
		probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		_, err := inspect(probeCtx, repository+"@"+digest, false)
		cancel()
		if err != nil {
			if docker.Diagnose(err.Error()) == "network unavailable" {
				return "unreachable"
			}
			return docker.Diagnose(err.Error())
		}
	}
	return "present"
}

func probeLockedImage(ctx context.Context, ref compose.Ref, inspect manifestProbe) imageProbe {
	row := imageProbe{Name: ref.Name}
	row.Upstream = probeRegistry(ctx, ref.Repository, ref, inspect)
	if ref.Mirror != nil {
		row.Mirror = probeRegistry(ctx, *ref.Mirror, ref, inspect)
	} else {
		row.Mirror = "not configured"
	}
	switch {
	case row.Upstream == "unreachable" || row.Mirror == "unreachable":
		row.Status, row.Remedy = "warn", "Check network and registry access, then retry."
	case row.Upstream == "present" && row.Mirror == "present":
		row.Status, row.Remedy = "pass", ""
	case row.Upstream == "present":
		row.Status, row.Remedy = "warn", "Publish or repair the digest-identical mirror."
	case row.Mirror == "present":
		row.Status, row.Remedy = "warn", "Start can use the mirror; restore upstream availability."
	default:
		row.Status, row.Remedy = "fail", "Restore the locked digest upstream or in the mirror; check authentication and the lock."
	}
	return row
}

func runDoctorImages(cmd *cobra.Command, _ []string) error {
	if err := compose.LockError(); err != nil {
		return err
	}
	jsonOn, err := jsonModeOf(cmd)
	if err != nil {
		return err
	}
	if jsonOn {
		return runDoctorImagesJSON(cmd, docker.ManifestInspect)
	}
	rows := make([]doctorCheckResult, 0, len(compose.LockedImages()))
	if _, err := fmt.Fprintln(cmd.OutOrStdout(), "IMAGE                 UPSTREAM                         MIRROR                           STATUS"); err != nil {
		return err
	}
	for _, ref := range compose.LockedImages() {
		row := probeLockedImage(cmd.Context(), ref, docker.ManifestInspect)
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%-21s %-32s %-32s %s\n", row.Name, row.Upstream, row.Mirror, strings.ToUpper(row.Status)); err != nil {
			return err
		}
		if row.Remedy != "" {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", row.Remedy); err != nil {
				return err
			}
		}
		rows = append(rows, doctorCheckResult{Name: row.Name, Status: row.Status, Message: row.Remedy})
	}
	return doctorExit(buildDoctorReport(rows))
}

// runDoctorImagesJSON probes every locked image and writes one envelope
// (P7-SURF-11). The status exit code is the human mode's: nil when every image
// passed, else 1/2 in v1.4 mode and 10/12 in v1.5 (doctorExit).
func runDoctorImagesJSON(cmd *cobra.Command, inspect manifestProbe) error {
	data := DoctorImages{Images: []DoctorImage{}}
	checks := []doctorCheckResult{}
	for _, ref := range compose.LockedImages() {
		row := probeLockedImage(cmd.Context(), ref, inspect)
		data.Images = append(data.Images, DoctorImage(row))
		checks = append(checks, doctorCheckResult{Name: row.Name, Status: row.Status, Message: row.Remedy})
	}
	report := buildDoctorReport(checks)
	data.State = doctorState(report)
	if err := output.EmitData(pilotWriter(), "doctor images", data); err != nil {
		return err
	}
	return doctorExit(report)
}
