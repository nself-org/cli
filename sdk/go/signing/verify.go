package signing

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"time"
)

// KeyLookup resolves a key id to a key. found=false with a nil error is an
// unknown key; a non-nil error is returned to the caller wrapped.
type KeyLookup func(ctx context.Context, keyID string) (Key, bool, error)

type config struct {
	scope      string
	hasScope   bool
	revokedFn  func(keyID string) bool
	clock      func() time.Time
	badOptions []string
}

// Option configures a Verifier.
type Option func(*config)

// WithScope pins the verifier to one scope; keys with another scope give
// ErrWrongScope. An empty scope is a configuration error (it would silently
// mean "any").
func WithScope(s string) Option {
	return func(c *config) {
		if s == "" {
			c.badOptions = append(c.badOptions, "empty scope")
			return
		}
		c.scope, c.hasScope = s, true
	}
}

// WithRevoked adds a revocation predicate consulted on every Verify, before
// any key lookup; true gives ErrRevoked.
func WithRevoked(fn func(keyID string) bool) Option {
	return func(c *config) { c.revokedFn = fn }
}

// WithClock replaces time.Now for key validity checks (tests).
func WithClock(fn func() time.Time) Option {
	return func(c *config) {
		if fn == nil {
			c.badOptions = append(c.badOptions, "nil clock")
			return
		}
		c.clock = fn
	}
}

// Verifier verifies signatures for one purpose and optionally one scope.
type Verifier struct {
	purpose Purpose
	cfg     config
	revoked map[string]struct{}
	keys    map[string]Key
	lookup  KeyLookup
}

func newVerifier(p Purpose, revoked []string, opts []Option) (*Verifier, error) {
	if !p.Valid() {
		return nil, fmt.Errorf("%w: purpose %q", ErrMalformed, clip(string(p)))
	}
	v := &Verifier{purpose: p, revoked: map[string]struct{}{}}
	v.cfg.clock = time.Now
	for _, o := range opts {
		if o != nil {
			o(&v.cfg)
		}
	}
	if len(v.cfg.badOptions) > 0 {
		return nil, fmt.Errorf("%w: option: %s", ErrMalformed, v.cfg.badOptions[0])
	}
	for _, id := range revoked {
		if !derivedIDShape(id) {
			return nil, fmt.Errorf("%w: revoked id %q is not a derived key id", ErrMalformed, clip(id))
		}
		v.revoked[id] = struct{}{}
	}
	return v, nil
}

// NewVerifier verifies against a fixed key set. Keys of other purposes are
// accepted into the set and refused at Verify with ErrWrongPurpose. Duplicate
// ids, bad ids, unknown purposes and wrong key lengths are errors. The keys
// are copied.
func NewVerifier(p Purpose, keys []Key, revoked []string, opts ...Option) (*Verifier, error) {
	v, err := newVerifier(p, revoked, opts)
	if err != nil {
		return nil, err
	}
	v.keys = make(map[string]Key, len(keys))
	pubs := make(map[string]string, len(keys))
	for _, k := range keys {
		if err := checkKey(k); err != nil {
			return nil, err
		}
		if _, dup := v.keys[k.ID]; dup {
			return nil, fmt.Errorf("%w: duplicate key id %q", ErrMalformed, k.ID)
		}
		if prev, dup := pubs[string(k.Public)]; dup {
			return nil, fmt.Errorf("%w: key %q repeats the public key of %q", ErrMalformed, k.ID, prev)
		}
		pubs[string(k.Public)] = k.ID
		k.Public = append(ed25519.PublicKey(nil), k.Public...)
		v.keys[k.ID] = k
	}
	return v, nil
}

// NewLookupVerifier verifies against keys resolved per call by lookup.
func NewLookupVerifier(p Purpose, lookup KeyLookup, opts ...Option) (*Verifier, error) {
	if lookup == nil {
		return nil, fmt.Errorf("%w: nil key lookup", ErrMalformed)
	}
	v, err := newVerifier(p, nil, opts)
	if err != nil {
		return nil, err
	}
	v.lookup = lookup
	return v, nil
}

// Purpose returns the purpose the verifier is pinned to.
func (v *Verifier) Purpose() Purpose {
	if v == nil {
		return ""
	}
	return v.purpose
}

// Verify checks sig over msg. It returns nil only when the key is known, not
// revoked, of the verifier's purpose and scope, currently valid, and the
// signature verifies; otherwise an error matching a sentinel. Checks run in
// this order, so the key-level error (malformed id, revoked, unknown, purpose,
// scope, validity) is reported before a malformed or bad signature.
func (v *Verifier) Verify(msg []byte, sig Signature) error {
	return v.VerifyContext(context.Background(), msg, sig)
}

// VerifyContext is Verify with a context handed to a KeyLookup.
func (v *Verifier) VerifyContext(ctx context.Context, msg []byte, sig Signature) error {
	if v == nil || (v.keys == nil && v.lookup == nil) {
		return fmt.Errorf("%w: verifier not constructed", ErrMalformed)
	}
	if !validKeyID(sig.KeyID) {
		return fmt.Errorf("%w: key id %q", ErrMalformed, clip(sig.KeyID))
	}
	if _, r := v.revoked[sig.KeyID]; r || (v.cfg.revokedFn != nil && v.cfg.revokedFn(sig.KeyID)) {
		return fmt.Errorf("%w: %q", ErrRevoked, sig.KeyID)
	}
	key, err := v.resolve(ctx, sig.KeyID)
	if err != nil {
		return err
	}
	if key.Purpose != v.purpose {
		return fmt.Errorf("%w: key %q is %q, verifier is %q", ErrWrongPurpose, key.ID, clip(string(key.Purpose)), v.purpose)
	}
	if v.cfg.hasScope && key.Scope != v.cfg.scope {
		return fmt.Errorf("%w: key %q", ErrWrongScope, key.ID)
	}
	now := v.cfg.clock()
	if !key.NotBefore.IsZero() && now.Before(key.NotBefore) {
		return fmt.Errorf("%w: %q", ErrNotYetValid, key.ID)
	}
	if !key.NotAfter.IsZero() && !now.Before(key.NotAfter) {
		return fmt.Errorf("%w: %q", ErrExpired, key.ID)
	}
	if len(sig.Sig) != ed25519.SignatureSize {
		return fmt.Errorf("%w: signature length %d (key %q)", ErrMalformed, len(sig.Sig), key.ID)
	}
	if !ed25519.Verify(key.Public, msg, sig.Sig) {
		return fmt.Errorf("%w: key %q", ErrBadSignature, key.ID)
	}
	return nil
}

func (v *Verifier) resolve(ctx context.Context, id string) (Key, error) {
	if v.lookup == nil {
		k, ok := v.keys[id]
		if !ok {
			return Key{}, fmt.Errorf("%w: %q", ErrUnknownKey, id)
		}
		return k, nil
	}
	k, ok, err := v.lookup(ctx, id)
	if err != nil {
		return Key{}, fmt.Errorf("signing: key lookup for %q: %w", id, err)
	}
	if !ok || k.ID != id {
		return Key{}, fmt.Errorf("%w: %q", ErrUnknownKey, id)
	}
	if err := checkKey(k); err != nil {
		return Key{}, err
	}
	return k, nil
}
