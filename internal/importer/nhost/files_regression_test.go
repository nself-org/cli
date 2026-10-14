package nhost

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/portable"
)

func TestZeroUploadedStorageConsistency(t *testing.T) {
	for _, tc := range []struct {
		name      string
		snap      storageSnapshot
		tables    []portable.Table
		wantError bool
	}{
		{"removed after dump", storageSnapshot{}, []portable.Table{{Schema: "storage", Name: "files", Rows: 1, Hash: "1"}}, true},
		{"pending rows changed", storageSnapshot{total: 2}, []portable.Table{{Schema: "storage", Name: "files", Rows: 1, Hash: "1"}}, true},
		{"listing count changed", storageSnapshot{rows: []fileRow{{id: "file"}}}, []portable.Table{{Schema: "storage", Name: "files", Hash: "1"}}, true},
		{"missing dump with pending rows", storageSnapshot{total: 1}, nil, true},
		{"stable pending rows", storageSnapshot{total: 1}, []portable.Table{{Schema: "storage", Name: "files", Rows: 1, Hash: "1"}}, false},
		{"empty table", storageSnapshot{}, []portable.Table{{Schema: "storage", Name: "files", Hash: "1"}}, false},
		{"absent table", storageSnapshot{}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := buildWith(t, importer.Input{}, tc.snap, tc.tables)
			if tc.wantError {
				if err == nil || !strings.Contains(err.Error(), "source storage changed") {
					t.Fatalf("inconsistent zero-upload snapshot accepted: %v", err)
				}
			} else if err != nil {
				t.Fatalf("consistent zero-upload snapshot requires no storage credentials: %v", err)
			}
		})
	}
}
