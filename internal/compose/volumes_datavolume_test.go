package compose

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestPostgresDataVolumeName pins POSTGRES_DATA_VOLUME (P7-ADOPT-22): set, the
// postgres_data volume carries that name; unset, the volumes render exactly as
// before the key existed.
func TestPostgresDataVolumeName(t *testing.T) {
	render := func(name string) string {
		cfg := minimalConfig()
		cfg.Postgres.DataVolume = name
		out, err := yaml.Marshal(map[string]any{"volumes": NewGenerator(cfg).buildVolumes()})
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	if got, want := render(""), "volumes:\n    postgres_data: {}\n"; got != want {
		t.Errorf("unset must render byte-identically to before:\n got %q\nwant %q", got, want)
	}
	if got, want := render("  "), "volumes:\n    postgres_data: {}\n"; got != want {
		t.Errorf("a blank value counts as unset:\n got %q\nwant %q", got, want)
	}
	if got, want := render("x"), "volumes:\n    postgres_data:\n        name: x\n"; got != want {
		t.Errorf("POSTGRES_DATA_VOLUME=x:\n got %q\nwant %q", got, want)
	}
	if got, want := render(" proj_postgres_data_pgvector_1 "), "volumes:\n    postgres_data:\n        name: proj_postgres_data_pgvector_1\n"; got != want {
		t.Errorf("the name is trimmed:\n got %q\nwant %q", got, want)
	}
	cfg := minimalConfig()
	cfg.Postgres.DataVolume = "x"
	if vols := NewGenerator(cfg).buildVolumes(); len(vols) < 1 || vols["postgres_data"].Name != "x" {
		t.Errorf("volumes = %v", vols)
	}
}
