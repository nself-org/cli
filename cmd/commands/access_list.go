package commands

// Purpose: `nself access list` handler — renders access.List's result as a
// table (default) or JSON (--json).
// Inputs: --host, --identity, --json.
// Outputs: a table (or JSON array) of managed entries, plus a note about any
// foreign (non-nself-managed) keys sharing the file.

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/nself-org/cli/internal/access"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// accessListRow is the JSON representation of one managed entry.
type accessListRow struct {
	User        string `json:"user"`
	Fingerprint string `json:"fingerprint"`
	Sudo        bool   `json:"sudo"`
	Docker      bool   `json:"docker"`
	Expires     string `json:"expires,omitempty"`
	Expired     bool   `json:"expired"`
	Granted     string `json:"granted"`
}

func runAccessList(cmd *cobra.Command, args []string) error {
	jsonOut, _ := cmd.Flags().GetBool("json")
	targets, selected, err := resolveAccessTargets(cmd)
	if err != nil {
		return err
	}
	if selected {
		return listSelectedAccess(cmd, targets, jsonOut)
	}

	t := targets[0].Transport

	if !jsonOut {
		ui.CommandHeader("nself access list", t.Describe())
	}

	result, err := access.List(cmd.Context(), t)
	if err != nil {
		return fmt.Errorf("list access on %s: %w", t.Describe(), err)
	}

	now := time.Now()
	rows := make([]accessListRow, 0, len(result.Entries))
	for _, e := range result.Entries {
		row := accessListRow{
			User:        e.User,
			Fingerprint: e.Fingerprint(),
			Sudo:        e.Sudo,
			Docker:      e.Docker,
			Expired:     e.Expired(now),
			Granted:     e.Granted.UTC().Format(time.RFC3339),
		}
		if e.Expires != nil {
			row.Expires = e.Expires.Format("2006-01-02")
		}
		rows = append(rows, row)
	}

	if jsonOut {
		return ui.PrintJSON(rows)
	}

	if len(rows) == 0 {
		fmt.Println("No nself-managed keys found. Run 'nself access grant' to add one.")
	} else {
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "USER\tFINGERPRINT\tSUDO\tDOCKER\tEXPIRES\tSTATUS")
		for _, r := range rows {
			sudo, docker, status := "no", "no", "active"
			if r.Sudo {
				sudo = "yes"
			}
			if r.Docker {
				docker = "yes"
			}
			if r.Expired {
				status = "expired"
			}
			expires := r.Expires
			if expires == "" {
				expires = "-"
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", r.User, r.Fingerprint, sudo, docker, expires, status)
		}
		if err := tw.Flush(); err != nil {
			return fmt.Errorf("flush table: %w", err)
		}
	}

	if result.ForeignCount > 0 {
		ui.Warn(fmt.Sprintf(
			"%d key(s) in authorized_keys were not granted by nself access and are left untouched",
			result.ForeignCount))
	}
	return nil
}

// accessHostRow keeps one selector result per resolved inventory host.
type accessHostRow struct {
	Env          string          `json:"env"`
	Server       string          `json:"server"`
	Host         string          `json:"host"`
	Entries      []accessListRow `json:"entries"`
	ForeignCount int             `json:"foreign_count"`
}

func listSelectedAccess(cmd *cobra.Command, targets []accessTarget, jsonOut bool) error {
	rows := make([]accessHostRow, 0, len(targets))
	now := time.Now()
	for _, target := range targets {
		result, err := access.List(cmd.Context(), target.Transport)
		if err != nil {
			return fmt.Errorf("list access on %s/%s: %w", target.Env, target.Server, err)
		}
		row := accessHostRow{Env: target.Env, Server: target.Server, Host: target.Host, Entries: []accessListRow{}, ForeignCount: result.ForeignCount}
		for _, e := range result.Entries {
			item := accessListRow{User: e.User, Fingerprint: e.Fingerprint(), Sudo: e.Sudo, Docker: e.Docker, Expired: e.Expired(now), Granted: e.Granted.UTC().Format(time.RFC3339)}
			if e.Expires != nil {
				item.Expires = e.Expires.Format("2006-01-02")
			}
			row.Entries = append(row.Entries, item)
		}
		rows = append(rows, row)
	}
	if jsonOut {
		return ui.PrintJSON(rows)
	}
	for _, row := range rows {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s/%s %s: %d managed key(s), %d foreign key(s)\n", row.Env, row.Server, row.Host, len(row.Entries), row.ForeignCount)
	}
	return nil
}
