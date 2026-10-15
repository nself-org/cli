package firebase

// Purpose: mint an OAuth access token from a service-account key with the standard library only.
// Inputs: key file path. Outputs: bearer tokens held in memory. Constraints: the key and every
// token live only in memory and are never logged or put in errors; the signed assertion goes
// only to a googleapis.com token endpoint over https (a key file cannot redirect it elsewhere).
import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/cli/internal/httptimeout"
)

const (
	tokenURL  = "https://oauth2.googleapis.com/token"
	tokenLife = time.Hour
	scopes    = "https://www.googleapis.com/auth/datastore https://www.googleapis.com/auth/devstorage.read_only"
	maxKey    = 64 << 10
)

type serviceKey struct {
	Type        string `json:"type"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// loadKey reads and parses a service-account JSON key.
func loadKey(path string) (*rsa.PrivateKey, serviceKey, error) {
	var sk serviceKey
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxKey {
		return nil, sk, safe("credentials file unreadable")
	}
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &sk) != nil || sk.Type != "service_account" || sk.ClientEmail == "" {
		return nil, sk, safe("credentials file is not a service-account key")
	}
	block, _ := pem.Decode([]byte(sk.PrivateKey))
	if block == nil {
		return nil, sk, safe("credentials private key invalid")
	}
	parsed, perr := x509.ParsePKCS8PrivateKey(block.Bytes)
	if perr != nil {
		rsaKey, e := x509.ParsePKCS1PrivateKey(block.Bytes)
		if e != nil {
			return nil, sk, safe("credentials private key invalid")
		}
		return rsaKey, sk, nil
	}
	rsaKey, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, sk, safe("credentials private key is not RSA")
	}
	return rsaKey, sk, nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// mintJWT builds the RS256 assertion: header.claims.signature.
func mintJWT(key *rsa.PrivateKey, issuer, audience, scope string, now time.Time) (string, error) {
	head, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{"iss": issuer, "scope": scope, "aud": audience, "iat": now.Unix(), "exp": now.Add(tokenLife).Unix()})
	input := b64(head) + "." + b64(claims)
	sum := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", safe("credentials signing failed")
	}
	return input + "." + b64(sig), nil
}

// tokenSource yields bearer tokens. The emulator source returns the fixed owner bearer.
type tokenSource interface {
	Token(ctx context.Context) (string, error)
}

type ownerToken struct{}

func (ownerToken) Token(context.Context) (string, error) { return "owner", nil }

type keyToken struct {
	key      *rsa.PrivateKey
	issuer   string
	endpoint string
	client   *http.Client
	now      func() time.Time
	mu       sync.Mutex
	token    string
	expires  time.Time
}

// newKeyToken loads the key and checks the token endpoint. endpoint "" means the key's token_uri.
func newKeyToken(path, endpoint string) (*keyToken, error) {
	key, sk, err := loadKey(path)
	if err != nil {
		return nil, err
	}
	if endpoint == "" {
		endpoint = sk.TokenURI
	}
	if endpoint == "" {
		endpoint = tokenURL
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || (u.Hostname() != "googleapis.com" && !strings.HasSuffix(u.Hostname(), ".googleapis.com")) {
		return nil, safe("credentials token endpoint is not a googleapis.com https URL")
	}
	return &keyToken{key: key, issuer: sk.ClientEmail, endpoint: endpoint, client: httptimeout.NoProxy(30 * time.Second), now: time.Now}, nil
}

func (k *keyToken) Token(ctx context.Context) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.token != "" && k.now().Add(time.Minute).Before(k.expires) {
		return k.token, nil
	}
	now := k.now()
	assertion, err := mintJWT(k.key, k.issuer, k.endpoint, scopes, now)
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, k.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", safe("token request invalid")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := k.client.Do(req)
	if err != nil {
		return "", safe("token endpoint unreachable")
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out) != nil || out.AccessToken == "" {
		return "", safe("token endpoint refused the credentials")
	}
	k.token, k.expires = out.AccessToken, now.Add(time.Duration(out.ExpiresIn)*time.Second)
	return k.token, nil
}
