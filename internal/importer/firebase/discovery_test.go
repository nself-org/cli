package firebase

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestDiscoveryDoesNotDependOnCollectionIDs(t *testing.T) {
	d := fixtureDB()
	d.subs = nil // Simulate listCollectionIds omitting every collection.
	col, _ := collectFixture(t, d)
	if len(col.tables) != 3 || col.tables["users/*/orders"].listed != 5 {
		t.Fatalf("collection discovery omitted documents: %d tables", len(col.tables))
	}
	for _, request := range d.requests {
		if strings.Contains(request, ":listCollectionIds") {
			t.Fatalf("per-document collection discovery remains: %s", request)
		}
	}
}

func TestDiscoveryPinsSnapshotAcrossPages(t *testing.T) {
	d := fixtureDB()
	for i := 0; i < 85; i++ {
		d.docs["users"] = append(d.docs["users"], fakeDoc{ID: fmt.Sprintf("page-%03d", i), Fields: map[string]any{}})
	}
	col, _ := collectFixture(t, d) // The fake rejects unpinned query and collection COUNT calls.
	if col.tables["users"].listed != 88 || col.tables["users"].counted != 88 {
		t.Fatal("snapshot pagination omitted documents")
	}
	queries := 0
	for _, request := range d.requests {
		if strings.HasSuffix(request, ":runQuery") {
			queries++
		}
	}
	if queries != 3 {
		t.Fatalf("got %d query pages, want 3", queries)
	}
}

func TestDiscoveryOfEmptySnapshot(t *testing.T) {
	col, _ := collectFixture(t, &fakeDB{})
	if len(col.tables) != 0 {
		t.Fatal("empty snapshot created tables")
	}
}

func TestDiscoveryRejectsOmittedCollection(t *testing.T) {
	d := fixtureDB()
	d.dropLast = true
	fs, _, _ := fakeClient(t, d)
	col := newCollector(fs, t.TempDir())
	err := col.walk(context.Background(), "", nil)
	_ = col.close()
	if err == nil {
		t.Fatal("incomplete database query accepted")
	}
}
