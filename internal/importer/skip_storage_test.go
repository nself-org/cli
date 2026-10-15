package importer

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// skipStorageSource observes the public input without requiring the new field to compile.
// Inputs: Run options. Outputs: choices seen by detection and extraction. No target writes.
type skipStorageSource struct {
	testSource
	detected, extracted bool
}

func skipStorageInput(in Input) bool {
	f := reflect.ValueOf(in).FieldByName("SkipStorage")
	return f.IsValid() && f.Kind() == reflect.Bool && f.Bool()
}

func (s *skipStorageSource) Detect(_ context.Context, in Input) (Detection, error) {
	s.detected = skipStorageInput(in)
	return Detection{}, nil
}

func (s *skipStorageSource) Extract(_ context.Context, in Input, _ string) (Extracted, error) {
	s.extracted = skipStorageInput(in)
	return Extracted{}, errors.New("stop after observing extraction input")
}

func TestRunPassesSkipStorageToSource(t *testing.T) {
	targetSeams(t)
	s := &skipStorageSource{testSource: testSource{name: "test-skip-storage-input"}}
	Register(s)
	for _, skip := range []bool{true, false} {
		_, err := Run(context.Background(), &config.Config{}, s, Input{}, RunOptions{
			ProjectDir: t.TempDir(), SkipStorage: skip,
		})
		if err == nil {
			t.Fatal("expected extraction sentinel error")
		}
		if s.detected != skip || s.extracted != skip {
			t.Fatalf("skip=%v: Detect saw %v, Extract saw %v", skip, s.detected, s.extracted)
		}
	}
}
