package importer

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

// fixtureSafeError represents a source's explicitly reviewed, fixed-template error.
type fixtureSafeError struct{ *errs.CLIError }

func (e fixtureSafeError) SafeCLIError() *errs.CLIError { return e.CLIError }

func TestPublicErrorSourceSafeError(t *testing.T) {
	const fixed = "source identifiers collide after normalization; rename them before importing"
	for _, code := range []string{"E517", "E518", "E520", "E521"} {
		t.Run(code, func(t *testing.T) {
			source := errs.Wrap(code, fixed, errors.New("secret underlying cause"))
			source.Why, source.Fix, source.DocsPath = "secret why", "secret fix", "secret docs"
			got := publicError(fmt.Errorf("secret wrapper: %w", fixtureSafeError{source}), "E517", "generic")
			var cli *errs.CLIError
			if !errors.As(got, &cli) || cli.Code != code || cli.What != fixed {
				t.Fatalf("source fixed template must survive: %v", got)
			}
			if strings.Contains(got.Error(), "secret") || errors.Unwrap(got) != nil {
				t.Fatalf("source context must be discarded: %v", got)
			}
		})
	}
	got := publicError(errs.New("E520", fixed), "E517", "generic")
	if strings.Contains(got.Error(), fixed) {
		t.Fatalf("unmarked source message must still be redacted: %v", got)
	}
}

func TestPublicErrorRejectsInvalidSourceSafeError(t *testing.T) {
	for _, source := range []*errs.CLIError{
		nil,
		errs.New("E523", "unapproved code"),
		errs.New("E520", "terminal\x1b[31m"),
		errs.New("E520", "bidi\u202e"),
		errs.New("E520", "bad\xff"),
		errs.New("E520", strings.Repeat("x", 301)),
	} {
		got := publicError(fixtureSafeError{source}, "E517", "generic")
		var cli *errs.CLIError
		if !errors.As(got, &cli) || cli.Code != "E517" || cli.What != "generic" {
			t.Fatalf("invalid safe error must use fallback: %#v", got)
		}
	}
}

func TestNewSafeErrorIsImmutableAndStructured(t *testing.T) {
	const fixed = "source collection names are unsupported; rename them before importing"
	source := NewSafeError("E520", fixed)
	var cli *errs.CLIError
	if !errors.As(source, &cli) || cli.Code != "E520" || cli.What != fixed {
		t.Fatalf("safe source error must support structured inspection: %v", source)
	}
	cli.What = "secret mutation"
	if strings.Contains(source.Error(), "secret") {
		t.Fatalf("structured inspection must not mutate source: %v", source)
	}
	got := publicError(fmt.Errorf("secret wrapper: %w", source), "E517", "generic")
	if !errors.As(got, &cli) || cli.Code != "E520" || cli.What != fixed || errors.Unwrap(got) != nil {
		t.Fatalf("safe source message must survive without source context: %v", got)
	}
}
