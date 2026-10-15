package sdk

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/signing"
)

const (
	// SignatureHeader is the request header carrying the base64-encoded Ed25519 signature.
	SignatureHeader = "X-Plugin-Signature"

	// TimestampHeader is the request header carrying the Unix timestamp (seconds) used
	// in the signed payload.
	TimestampHeader = "X-Plugin-Timestamp"

	// requestMaxAge is the window within which a signed request is considered valid.
	requestMaxAge = 5 * time.Minute

	pemTypePrivateKey = "PRIVATE KEY"
	pemTypePublicKey  = "PUBLIC KEY"
)

// Identity holds an Ed25519 keypair for a plugin instance. Each plugin generates
// a unique identity on first start and persists it to disk. The keypair is used
// to sign and verify inter-plugin requests.
type Identity struct {
	// PluginName is the logical name of the owning plugin (e.g. "ai", "mux").
	PluginName string
	// PublicKey is safe to share with other plugins for request verification.
	PublicKey ed25519.PublicKey
	// privateKey is kept unexported; signing is performed through Sign/SignRequest.
	privateKey ed25519.PrivateKey
}

// NewIdentity generates a fresh Ed25519 keypair for the given plugin name.
func NewIdentity(pluginName string) (*Identity, error) {
	if pluginName == "" {
		return nil, fmt.Errorf("identity: pluginName must not be empty")
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("identity: generate key: %w", err)
	}
	return &Identity{
		PluginName: pluginName,
		PublicKey:  pub,
		privateKey: priv,
	}, nil
}

// LoadOrCreate loads an existing identity from keyPath, or generates and persists
// a new one if the file does not exist. The key file is written with mode 0600.
// Intermediate directories are created if absent.
func LoadOrCreate(_ context.Context, pluginName, keyPath string) (*Identity, error) {
	if pluginName == "" {
		return nil, fmt.Errorf("identity: pluginName must not be empty")
	}
	if keyPath == "" {
		return nil, fmt.Errorf("identity: keyPath must not be empty")
	}

	data, err := os.ReadFile(keyPath)
	if err == nil {
		return parseIdentityPEM(pluginName, data)
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("identity: read %s: %w", keyPath, err)
	}

	// File does not exist — generate a fresh identity and persist it.
	id, err := NewIdentity(pluginName)
	if err != nil {
		return nil, err
	}
	pemData, err := marshalIdentityPEM(id)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return nil, fmt.Errorf("identity: mkdir %s: %w", filepath.Dir(keyPath), err)
	}
	if err := os.WriteFile(keyPath, pemData, 0600); err != nil {
		return nil, fmt.Errorf("identity: write %s: %w", keyPath, err)
	}
	return id, nil
}

// PublicKeyPEM returns the public key encoded in PEM format (PKIX DER wrapped in
// a "PUBLIC KEY" block). Share this with peer plugins so they can verify
// signatures from this identity.
func (id *Identity) PublicKeyPEM() ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(id.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("identity: marshal public key: %w", err)
	}
	block := &pem.Block{Type: pemTypePublicKey, Bytes: der}
	return pem.EncodeToMemory(block), nil
}

// Sign signs message with the identity's private key and returns a
// base64-encoded (StdEncoding) signature string.
func (id *Identity) Sign(message []byte) string {
	sig := ed25519.Sign(id.privateKey, message)
	return base64.StdEncoding.EncodeToString(sig)
}

// Verify reports whether sig is a valid Ed25519 signature of message under
// the identity's public key. sig must be raw signature bytes (not base64).
func (id *Identity) Verify(message, sig []byte) bool {
	return signing.VerifyEd25519(id.PublicKey, message, sig) == nil
}

// SignRequest signs the outgoing HTTP request and sets the X-Plugin-Signature
// and X-Plugin-Timestamp headers. The signed payload is:
//
//	"{METHOD}\n{path}\n{unix-timestamp-seconds}\n{hex-sha256-of-body}"
//
// body must be the raw request body bytes (may be nil for bodyless requests).
func (id *Identity) SignRequest(req *http.Request, body []byte) error {
	if req == nil {
		return fmt.Errorf("identity: SignRequest: req must not be nil")
	}
	now := time.Now().Unix()
	payload := buildRequestPayload(req.Method, req.URL.RequestURI(), now, body)
	sig := id.Sign([]byte(payload))
	req.Header.Set(TimestampHeader, strconv.FormatInt(now, 10))
	req.Header.Set(SignatureHeader, sig)
	return nil
}

// VerifyRequest verifies the X-Plugin-Signature and X-Plugin-Timestamp headers
// on an incoming HTTP request. It returns an error if:
//   - X-Plugin-Timestamp is missing or unparseable
//   - the timestamp is more than 5 minutes in the past or future
//   - X-Plugin-Signature is missing, malformed, or invalid for the payload
func VerifyRequest(req *http.Request, pubKey ed25519.PublicKey, body []byte) error {
	if req == nil {
		return fmt.Errorf("identity: VerifyRequest: req must not be nil")
	}

	tsStr := req.Header.Get(TimestampHeader)
	if tsStr == "" {
		return fmt.Errorf("identity: missing %s header", TimestampHeader)
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(tsStr), 10, 64)
	if err != nil {
		return fmt.Errorf("identity: invalid %s value %q: %w", TimestampHeader, tsStr, err)
	}
	age := time.Duration(math.Abs(float64(time.Now().Unix()-ts))) * time.Second
	if age > requestMaxAge {
		return fmt.Errorf("identity: request timestamp is %s old (max %s)", age.Round(time.Second), requestMaxAge)
	}

	sigStr := req.Header.Get(SignatureHeader)
	if sigStr == "" {
		return fmt.Errorf("identity: missing %s header", SignatureHeader)
	}
	sig, err := base64.StdEncoding.DecodeString(sigStr)
	if err != nil {
		return fmt.Errorf("identity: decode signature: %w", err)
	}

	payload := buildRequestPayload(req.Method, req.URL.RequestURI(), ts, body)
	if signing.VerifyEd25519(pubKey, []byte(payload), sig) != nil {
		return fmt.Errorf("identity: signature verification failed")
	}
	return nil
}

// buildRequestPayload constructs the canonical signed string for request
// authentication:
//
//	"{METHOD}\n{request-uri}\n{unix-ts}\n{hex-sha256-of-body}"
func buildRequestPayload(method, requestURI string, unixTS int64, body []byte) string {
	sum := sha256.Sum256(body)
	return fmt.Sprintf("%s\n%s\n%d\n%s",
		method,
		requestURI,
		unixTS,
		hex.EncodeToString(sum[:]),
	)
}

// marshalIdentityPEM encodes an Identity as a sequence of two PEM blocks:
// PKCS8 private key followed by PKIX public key.
func marshalIdentityPEM(id *Identity) ([]byte, error) {
	privDER, err := x509.MarshalPKCS8PrivateKey(id.privateKey)
	if err != nil {
		return nil, fmt.Errorf("identity: marshal private key: %w", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(id.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("identity: marshal public key: %w", err)
	}
	out := pem.EncodeToMemory(&pem.Block{Type: pemTypePrivateKey, Bytes: privDER})
	out = append(out, pem.EncodeToMemory(&pem.Block{Type: pemTypePublicKey, Bytes: pubDER})...)
	return out, nil
}

// PeerRegistry stores the public keys of peer plugins so that received
// inter-plugin requests can be verified without sharing private keys.
// It is safe for concurrent use.
//
// Typical usage:
//
//	registry := sdk.NewPeerRegistry()
//	registry.Register("mux", muxIdentity.PublicKey)
//	// On incoming request from mux:
//	pubKey, ok := registry.Lookup("mux")
//	if !ok { /* reject: unknown peer */ }
//	if err := sdk.VerifyRequest(req, pubKey, body); err != nil { /* reject */ }
type PeerRegistry struct {
	mu   sync.RWMutex
	keys map[string]ed25519.PublicKey
}

// NewPeerRegistry returns an empty, ready-to-use PeerRegistry.
func NewPeerRegistry() *PeerRegistry {
	return &PeerRegistry{keys: make(map[string]ed25519.PublicKey)}
}

// Register stores or replaces the public key for the named peer plugin.
// pluginName must not be empty. Calling Register again for the same name
// replaces the previous entry (key rotation).
func (r *PeerRegistry) Register(pluginName string, pubKey ed25519.PublicKey) error {
	if pluginName == "" {
		return fmt.Errorf("peer-registry: pluginName must not be empty")
	}
	if len(pubKey) != ed25519.PublicKeySize {
		return fmt.Errorf("peer-registry: invalid public key length %d (want %d)", len(pubKey), ed25519.PublicKeySize)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// Store a copy so the caller cannot mutate the stored key.
	cp := make(ed25519.PublicKey, len(pubKey))
	copy(cp, pubKey)
	r.keys[pluginName] = cp
	return nil
}

// Lookup returns the public key registered for pluginName. The second return
// value is false if the name is unknown.
func (r *PeerRegistry) Lookup(pluginName string) (ed25519.PublicKey, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	k, ok := r.keys[pluginName]
	return k, ok
}

// Remove deletes the entry for pluginName. It is a no-op if the name is not
// registered.
func (r *PeerRegistry) Remove(pluginName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.keys, pluginName)
}

// Names returns a snapshot of all registered plugin names.
func (r *PeerRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.keys))
	for n := range r.keys {
		names = append(names, n)
	}
	return names
}
