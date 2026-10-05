package config

import (
	"net/url"
	"testing"
)

func TestHostDatabaseURL(t *testing.T) {
	cases := []struct {
		name string
		pg   PostgresConfig
		want string
	}{
		{"defaults", PostgresConfig{User: "postgres", Password: "pw"}, "postgresql://postgres:pw@127.0.0.1:5432/nself"},
		{"published port", PostgresConfig{User: "app", Password: "pw", Port: 15432, DB: "d1"}, "postgresql://app:pw@127.0.0.1:15432/d1"},
		{"encoded password", PostgresConfig{User: "postgres", Password: "p@ss:w/rd?#%", Port: 5433, DB: "x"},
			"postgresql://postgres:p%40ss%3Aw%2Frd%3F%23%25@127.0.0.1:5433/x"},
	}
	for _, c := range cases {
		cfg := &Config{}
		cfg.Postgres = c.pg
		got := cfg.HostDatabaseURL()
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
		u, err := url.Parse(got)
		if err != nil {
			t.Fatalf("%s: parse: %v", c.name, err)
		}
		if u.Hostname() != "127.0.0.1" {
			t.Errorf("%s: host %q, want loopback", c.name, u.Hostname())
		}
		if pw, _ := u.User.Password(); pw != c.pg.Password {
			t.Errorf("%s: password does not round-trip", c.name)
		}
	}
}

func TestHostDatabaseURLKeepsInNetworkDSN(t *testing.T) {
	cfg := &Config{}
	cfg.Postgres = PostgresConfig{Host: "postgres", User: "u", Password: "p", Port: 15432, DB: "d"}
	if got, want := cfg.DatabaseURL(), "postgresql://u:p@postgres:5432/d"; got != want {
		t.Errorf("DatabaseURL() = %q, want %q", got, want)
	}
}
