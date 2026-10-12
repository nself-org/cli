package firebase

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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	testKeyOnce sync.Once
	testKey     *rsa.PrivateKey
)

func writeKey(t *testing.T, tokenURI string) (string, *rsa.PrivateKey) {
	t.Helper()
	testKeyOnce.Do(func() { testKey, _ = rsa.GenerateKey(rand.Reader, 2048) })
	key := testKey
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	body, _ := json.Marshal(map[string]string{"type": "service_account", "client_email": "svc@demo.iam.gserviceaccount.com", "private_key": pemKey, "token_uri": tokenURI})
	p := filepath.Join(t.TempDir(), "key.json")
	if err := os.WriteFile(p, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return p, key
}

func TestMintJWTVerifiesWithPublicKey(t *testing.T) {
	path, key := writeKey(t, "")
	loaded, sk, err := loadKey(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1700000000, 0)
	jwt, err := mintJWT(loaded, sk.ClientEmail, tokenURL, scopes, now)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt %q", jwt)
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err = rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, sum[:], sig); err != nil {
		t.Fatalf("signature invalid: %v", err)
	}
	var head map[string]string
	var claims map[string]any
	hb, _ := base64.RawURLEncoding.DecodeString(parts[0])
	cb, _ := base64.RawURLEncoding.DecodeString(parts[1])
	_ = json.Unmarshal(hb, &head)
	_ = json.Unmarshal(cb, &claims)
	if head["alg"] != "RS256" || claims["iss"] != sk.ClientEmail || claims["aud"] != tokenURL || claims["scope"] != scopes || claims["exp"].(float64)-claims["iat"].(float64) != 3600 {
		t.Fatalf("header %v claims %v", head, claims)
	}
}

func TestKeyTokenExchangeCachesAndHidesSecrets(t *testing.T) {
	path, _ := writeKey(t, "")
	key, sk, err := loadKey(path)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || strings.Count(r.Form.Get("assertion"), ".") != 2 {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"tok-1","expires_in":3600}`))
	}))
	defer srv.Close()
	k := &keyToken{key: key, issuer: sk.ClientEmail, endpoint: srv.URL, client: srv.Client(), now: time.Now}
	for i := 0; i < 2; i++ {
		if tok, err := k.Token(context.Background()); err != nil || tok != "tok-1" {
			t.Fatalf("%q %v", tok, err)
		}
	}
	if calls != 1 {
		t.Fatalf("token not cached: %d calls", calls)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "secret-body-text", http.StatusUnauthorized)
	}))
	defer bad.Close()
	k2 := &keyToken{key: key, issuer: sk.ClientEmail, endpoint: bad.URL, client: bad.Client(), now: time.Now}
	if _, err = k2.Token(context.Background()); err == nil || strings.Contains(err.Error(), "secret-body-text") {
		t.Fatalf("error must be fixed text: %v", err)
	}
}

func TestKeyTokenEndpointMustBeGoogleapis(t *testing.T) {
	for uri, ok := range map[string]bool{"": true, "https://oauth2.googleapis.com/token": true, "http://oauth2.googleapis.com/token": false, "https://evil.example/token": false, "https://googleapis.com.evil.example/token": false} {
		path, _ := writeKey(t, uri)
		_, err := newKeyToken(path, "")
		if (err == nil) != ok {
			t.Fatalf("token_uri %q: err=%v want ok=%v", uri, err, ok)
		}
	}
}

func TestLoadKeyRefusesBadFiles(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"notjson": "private key material", "wrongtype": `{"type":"authorized_user","client_email":"a"}`, "nopem": `{"type":"service_account","client_email":"a","private_key":"x"}`} {
		p := filepath.Join(dir, name)
		_ = os.WriteFile(p, []byte(body), 0o600)
		_, _, err := loadKey(p)
		if err == nil || strings.Contains(err.Error(), "private key material") {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, _, err := loadKey(dir); err == nil {
		t.Fatal("directory accepted")
	}
}
