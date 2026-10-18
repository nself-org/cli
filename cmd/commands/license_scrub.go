package commands

// Purpose: the scrubber of the account fragment's documents (P7-SURF-16).
// Text from a licence or account server, a cache file, a flag or a failure can
// carry a key, a JWT or a token. Every document a licence or account command
// emits in envelope mode, and the text of every failure, passes through it.
// Inputs: a document value (any struct, slice, map or pointer tree) or an
// error. Outputs: a copy of the same type with every string leaf scrubbed, or
// an error with scrubbed text and the same classification (code, exit status).
// Constraints: human and v1.4 output are never touched (callers gate on
// jsonEnvelopeOn); a masked key, a UUID and a date pass unchanged; nil slices
// stay nil and empty ones stay empty. SPORT: P7-SURF-16,
// contract:cli.json-envelope

import (
	"errors"
	"reflect"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license"
	"github.com/spf13/cobra"
)

var (
	// licenseKeyShape matches a licence key (product prefix, then the secret
	// part); jwtShape a JWT; tokenRun a token-like run of 24+ characters.
	licenseKeyShape = regexp.MustCompile(`nself_[a-z]+_[A-Za-z0-9_-]{8,}`)
	jwtShape        = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}(\.[A-Za-z0-9_-]*)?`)
	tokenRun        = regexp.MustCompile(`[A-Za-z0-9_.-]{24,}`)
	uuidShape       = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	dateShape       = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}[T0-9Z.+-]*$`)
	digitShape      = regexp.MustCompile(`[0-9]`)
	letterShape     = regexp.MustCompile(`[A-Za-z]`)
	upperShape      = regexp.MustCompile(`[A-Z]`)
	lowerShape      = regexp.MustCompile(`[a-z]`)
)

// licenseScrubMax caps the length of server text copied into a document row.
const licenseScrubMax = 200

// scrubber masks the keys it knows: the configured ones, the --key flag of
// `license migrate`, and any extra the caller names.
type scrubber struct{ keys []string }

func newScrubber(extra ...string) scrubber {
	keys := append(license.CollectLicenseKeys(), migrateKeyFlag)
	return scrubber{keys: append(keys, extra...)}
}

// opaque reports whether a 24+ character run looks like an access or session
// token, and is neither a UUID nor a date: letters and digits mixed, or upper
// and lower case mixed (an identifier like a unit name is single-case).
func opaque(run string) bool {
	if uuidShape.MatchString(run) || dateShape.MatchString(run) {
		return false
	}
	if digitShape.MatchString(run) && letterShape.MatchString(run) {
		return true
	}
	return upperShape.MatchString(run) && lowerShape.MatchString(run)
}

// str masks every licence key in s, drops every JWT and every opaque token.
func (sc scrubber) str(s string) string {
	for _, key := range sc.keys {
		if key != "" {
			s = strings.ReplaceAll(s, key, license.MaskKey(key))
		}
	}
	s = jwtShape.ReplaceAllString(s, "[REDACTED]")
	s = licenseKeyShape.ReplaceAllStringFunc(s, license.MaskKey)
	return tokenRun.ReplaceAllStringFunc(s, func(run string) string {
		if opaque(run) {
			return "[REDACTED]"
		}
		return run
	})
}

// licenseScrub is str, capped: server text copied into one row of a document.
func licenseScrub(s string, extra ...string) string {
	s = newScrubber(extra...).str(s)
	if r := []rune(s); len(r) > licenseScrubMax {
		s = string(r[:licenseScrubMax]) + "..."
	}
	return s
}

// licenseScrubAll scrubs every element: plugin names from a server or cache.
func licenseScrubAll(in []string) []string {
	sc := newScrubber()
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, sc.str(v))
	}
	return out
}

// value returns a copy of src with every string leaf scrubbed.
func (sc scrubber) value(src reflect.Value) reflect.Value {
	switch src.Kind() {
	case reflect.String:
		out := reflect.New(src.Type()).Elem()
		out.SetString(sc.str(src.String()))
		return out
	case reflect.Pointer:
		if src.IsNil() {
			return src
		}
		out := reflect.New(src.Type().Elem())
		out.Elem().Set(sc.value(src.Elem()))
		return out
	case reflect.Interface:
		if src.IsNil() {
			return src
		}
		out := reflect.New(src.Type()).Elem()
		out.Set(sc.value(src.Elem()))
		return out
	case reflect.Slice:
		if src.IsNil() {
			return src
		}
		out := reflect.MakeSlice(src.Type(), src.Len(), src.Len())
		for i := 0; i < src.Len(); i++ {
			out.Index(i).Set(sc.value(src.Index(i)))
		}
		return out
	case reflect.Array:
		out := reflect.New(src.Type()).Elem()
		for i := 0; i < src.Len(); i++ {
			out.Index(i).Set(sc.value(src.Index(i)))
		}
		return out
	case reflect.Map:
		if src.IsNil() {
			return src
		}
		out := reflect.MakeMapWithSize(src.Type(), src.Len())
		for it := src.MapRange(); it.Next(); {
			out.SetMapIndex(it.Key(), sc.value(it.Value()))
		}
		return out
	case reflect.Struct:
		out := reflect.New(src.Type()).Elem()
		out.Set(src)
		for i := 0; i < src.NumField(); i++ {
			if src.Type().Field(i).IsExported() {
				out.Field(i).Set(sc.value(src.Field(i)))
			}
		}
		return out
	}
	return src
}

// scrubDoc returns v with every string leaf scrubbed; the type is unchanged.
func scrubDoc(v any) any {
	if v == nil {
		return nil
	}
	return newScrubber().value(reflect.ValueOf(v)).Interface()
}

// emitDoc is emitEnv for the account fragment: in an envelope invocation the
// document is scrubbed first; v1.4 output is untouched.
func emitDoc(cmd *cobra.Command, doc any, legacyShape bool) error {
	if jsonEnvelopeOn(cmd) {
		doc = scrubDoc(doc)
	}
	return emitEnv(cmd, doc, legacyShape)
}

// scrubbedError carries scrubbed text and keeps the classification of the
// original: the structured fields are scrubbed copies, never the originals.
type scrubbedError struct {
	msg string
	err error
}

func (e *scrubbedError) Error() string { return e.msg }
func (e *scrubbedError) Unwrap() error { return e.err }

// scrubErr returns err with scrubbed text; the code and exit status are kept.
func scrubErr(err error) error {
	sc := newScrubber()
	out := &scrubbedError{msg: sc.str(err.Error()), err: err}
	var ce *errs.CLIError
	if errors.As(err, &ce) {
		c := *ce
		c.What, c.Why, c.Fix = sc.str(c.What), sc.str(c.Why), sc.str(c.Fix)
		out.err = &c
	}
	return out
}

// licenseEnv is dataEnv for the licence commands: in an envelope invocation
// the typed data and a failure's text are scrubbed before they are emitted.
func licenseEnv(run func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		dataSlots.Delete(cmd)
		on := jsonEnvelopeOn(cmd)
		if err := run(cmd, args); err != nil {
			if on {
				return scrubErr(err)
			}
			return err
		}
		if v, ok := dataSlots.Load(cmd); ok && on {
			dataSlots.Store(cmd, scrubDoc(v))
		}
		if err := emitDataEnvelope(cmd, args); err != nil {
			return scrubErr(err)
		}
		return nil
	}
}

// accountEnv scrubs the failure text of a self-emitting account command.
func accountEnv(run func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := run(cmd, args); err != nil && jsonEnvelopeOn(cmd) {
			return scrubErr(err)
		} else {
			return err
		}
	}
}
