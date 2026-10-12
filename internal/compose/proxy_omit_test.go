package compose

import (
	"bytes"
	"reflect"
	"testing"
)

func TestOmitNginx(t *testing.T) {
	cfg := minimalConfig()
	plain, err := NewGenerator(cfg).Generate()
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := NewGeneratorWithProfile(cfg, ProfileApp).Generate()
	if err != nil || !bytes.Equal(plain, baseline) {
		t.Fatalf("default profile changed: %v", err)
	}
	g := NewGenerator(cfg)
	if g.OmitNginx() != g {
		t.Fatal("not chainable")
	}
	got, err := g.buildDockerCompose()
	if err != nil {
		t.Fatal(err)
	}
	want, err := NewGenerator(cfg).buildDockerCompose()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Services["nginx"]; ok {
		t.Fatal("nginx remains")
	}
	if _, ok := want.Services["nginx"]; !ok {
		t.Fatal("baseline lacks nginx")
	}
	delete(want.Services, "nginx")
	want.ServiceOrder = got.ServiceOrder
	if !reflect.DeepEqual(got, want) {
		t.Fatal("other services or compose metadata changed")
	}
}
