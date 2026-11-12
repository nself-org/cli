package health

import "testing"

func TestDoctorFailedService(t *testing.T) {
	if got := DoctorFailedService("ok\npostgres unhealthy\nhasura failed\n"); got != "postgres" {
		t.Errorf("got %q, want postgres", got)
	}
	if got := DoctorFailedService("all good\n"); got != "unknown" {
		t.Errorf("got %q, want unknown", got)
	}
}
