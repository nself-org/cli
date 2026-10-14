package firebase

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/importer/load"
)

func TestAssignNamesAvoidsProtectedTables(t *testing.T) {
	for _, collection := range []string{"np_orders", "NP_orders", "np-orders", "np_", "np_" + strings.Repeat("x", 80)} {
		t.Run(collection, func(t *testing.T) {
			parent := &tableAcc{pattern: collection}
			child := &tableAcc{pattern: collection + "/*/np_items", parent: parent}
			notes := assignNames([]*tableAcc{parent, child})
			if len(notes) == 0 {
				t.Error("protected collection rename must be reported")
			}
			for _, table := range []*tableAcc{parent, child} {
				if load.SystemTable(schemaName, table.name, false) {
					t.Errorf("collection %q mapped to protected table %q", table.pattern, table.name)
				}
				if len(table.name) > 63 {
					t.Errorf("mapped name exceeds Postgres limit: %q", table.name)
				}
				statement, _, err := ddl(table, nil)
				if err != nil || !strings.Contains(statement, `"firebase"."`+table.name+`"`) {
					t.Errorf("DDL does not use mapped name: %q, %v", statement, err)
				}
			}
			first := parent.name
			assignNames([]*tableAcc{parent, child})
			if parent.name != first {
				t.Error("protected collection mapping is not deterministic")
			}
		})
	}
}

func TestAssignNamesProtectedRenameCollisions(t *testing.T) {
	tables := []*tableAcc{
		{pattern: "firebase_np_orders"},
		{pattern: "np-orders"},
		{pattern: "np_orders"},
		{pattern: "orders"},
	}
	assignNames(tables)
	for i, want := range []string{"firebase_np_orders", "firebase_np_orders_2", "firebase_np_orders_3", "orders"} {
		if tables[i].name != want {
			t.Errorf("collection %q mapped to %q, want %q", tables[i].pattern, tables[i].name, want)
		}
	}
}
