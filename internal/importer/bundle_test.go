package importer

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/portable"
)

func stagedReader(t *testing.T) *portable.Reader {
	t.Helper()
	w, err := portable.NewWriter(filepath.Join(t.TempDir(), "stage"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteFile("db/data.dump", strings.NewReader("dump")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Finish(portable.Manifest{}); err != nil {
		t.Fatal(err)
	}
	r, err := portable.Open(w.Dir())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestExtractedReusesOnlyItsStagedReader(t *testing.T) {
	r := stagedReader(t)
	x := Extracted{Stage: r.Dir(), VerifiedBundle: r}
	got, err := x.openBundle(r.Dir())
	if err != nil || got != r {
		t.Fatalf("staged reader was reopened: %p != %p, %v", got, r, err)
	}
	if _, err := x.openBundle(t.TempDir()); err == nil {
		t.Fatal("accepted reader from a different directory")
	}
	got, err = (Extracted{}).openBundle(r.Dir())
	if err != nil || got == nil || got == r {
		t.Fatalf("source without a reader must be verified: %v", err)
	}
	p := filepath.Join(r.Dir(), "db", "data.dump")
	if err := os.WriteFile(p, []byte("bad!"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (Extracted{}).openBundle(r.Dir()); err == nil {
		t.Fatal("unverified corrupt stage accepted")
	}
	member, err := x.VerifiedBundle.Open("db/data.dump")
	if err == nil {
		_, err = io.ReadAll(member)
		_ = member.Close()
	}
	if err == nil {
		t.Fatal("cached reader accepted changed member")
	}
}

type handoffSource struct {
	t      *testing.T
	reader *portable.Reader
	seen   bool
}

func (*handoffSource) Name() string      { return "test-reader-handoff" }
func (*handoffSource) Flags() []FlagSpec { return nil }
func (s *handoffSource) Detect(_ context.Context, in Input) (Detection, error) {
	if in.VerifiedBundle != nil {
		s.t.Fatal("Run reused a caller's previous detection")
	}
	return Detection{VerifiedBundle: s.reader}, nil
}
func (s *handoffSource) Extract(_ context.Context, in Input, _ string) (Extracted, error) {
	if in.VerifiedBundle != s.reader {
		s.t.Fatal("Run discarded Detect's reader before extraction")
	}
	s.seen = true
	return Extracted{}, errors.New("stop before target load")
}

func TestRunHandsDetectionReaderToExtract(t *testing.T) {
	targetSeams(t)
	s := &handoffSource{t: t, reader: stagedReader(t)}
	Register(s)
	_, err := Run(context.Background(), &config.Config{}, s,
		Input{VerifiedBundle: s.reader}, RunOptions{ProjectDir: t.TempDir()})
	if err == nil || !s.seen {
		t.Fatalf("extraction handoff not exercised: %v", err)
	}
}
