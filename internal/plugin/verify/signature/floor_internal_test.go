package signature

import "testing"

// TestFloorFailsClosedOnBadFile: an embedded signed_since.json that does not
// decode must make every entry need a signature, never none.
func TestFloorFailsClosedOnBadFile(t *testing.T) {
	if _, err := loadFloor([]byte(`{"tiers": [`)); err == nil {
		t.Fatal("loadFloor accepted broken JSON")
	}
	// A structurally broken table (missing or null tier, no tiers key) fails closed too.
	for _, bad := range []string{`{}`, `null`, `{"tiers":{"licensed":{}}}`, `{"tiers":{"free":{},"licensed":null}}`} {
		if _, err := loadFloor([]byte(bad)); err == nil {
			t.Errorf("loadFloor accepted %s", bad)
		}
	}
	m, err := loadFloor([]byte(`{"tiers":{"free":{"ci":"1.0.0"},"licensed":{}}}`))
	if err != nil || m["free"]["ci"] != "1.0.0" {
		t.Fatalf("loadFloor = %v, %v", m, err)
	}
	oldFloor, oldErr := Floor, floorErr
	t.Cleanup(func() { Floor, floorErr = oldFloor, oldErr })
	Floor, floorErr = map[string]map[string]string{}, nil
	if FloorRequires(TierFree, "ci", "9.9.9") {
		t.Fatal("empty floor must not require a signature")
	}
	floorErr = errBadFloor
	if !FloorRequires(TierFree, "ci", "9.9.9") {
		t.Fatal("a floor that failed to load must require a signature")
	}
}

var errBadFloor = errTest("bad floor")

type errTest string

func (e errTest) Error() string { return string(e) }
