package firebase

// Purpose: read a `firebase auth:export` JSON file into portable auth users.
// Inputs: the export path. Outputs: users (without any password hash), the reset list, the
// hash-algorithm tally and an independent element count. Constraints: Firebase uses a modified
// scrypt that hasura-auth cannot verify, so every user that has a password hash arrives without a
// password and is listed for reset; the hash itself is never copied into the bundle. A user id is
// a UUIDv5 of the Firebase uid, and the uid stays in metadata.firebase.uid.
import (
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nself-org/cli/internal/importer/verify"
	"github.com/nself-org/cli/internal/portable"
)

const maxExport = 2 << 30

// userNamespace fixes the UUIDv5 namespace so the same Firebase uid always maps to the same id.
var userNamespace = uuid.MustParse("5b0e0f3c-7d3a-4f6e-9a55-6e53656c662d")

type exportUser struct {
	LocalID          string          `json:"localId"`
	Email            string          `json:"email"`
	EmailVerified    bool            `json:"emailVerified"`
	PasswordHash     string          `json:"passwordHash"`
	DisplayName      string          `json:"displayName"`
	PhotoURL         string          `json:"photoUrl"`
	PhoneNumber      string          `json:"phoneNumber"`
	Disabled         bool            `json:"disabled"`
	CreatedAt        json.RawMessage `json:"createdAt"`
	LastSignedInAt   json.RawMessage `json:"lastSignedInAt"`
	LastLoginAt      json.RawMessage `json:"lastLoginAt"` // the key the Identity Toolkit API (and so auth:export) uses
	CustomAttributes string          `json:"customAttributes"`
	Providers        []struct {
		ProviderID string `json:"providerId"`
	} `json:"providerUserInfo"`
}

type authResult struct {
	// Written is how many users went to the output (the export minus the exemptions).
	Written       int
	ResetRequired []string
	Algorithms    map[string]int
	Exemptions    []portable.Exemption
	// Count is the independent element count of the users array (a second, raw pass).
	Count int64
	// Anonymous counts users with no email, phone or provider (Firebase anonymous auth).
	Anonymous int
}

// openUsers positions a decoder on the first element of the `users` array.
func openUsers(path string) (*os.File, *json.Decoder, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxExport {
		return nil, nil, safe("auth export unreadable")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, safe("auth export unreadable")
	}
	dec := json.NewDecoder(f)
	if tok, e := dec.Token(); e != nil || tok != json.Delim('{') {
		_ = f.Close()
		return nil, nil, safe("auth export is not a JSON object")
	}
	for dec.More() {
		tok, e := dec.Token()
		key, _ := tok.(string)
		if e != nil {
			break
		}
		if key == "users" {
			if t, e := dec.Token(); e != nil || t != json.Delim('[') {
				break
			}
			return f, dec, nil
		}
		var skip json.RawMessage
		if dec.Decode(&skip) != nil {
			break
		}
	}
	_ = f.Close()
	return nil, nil, safe("auth export has no users array")
}

// countExportUsers counts the raw elements of the users array without interpreting them.
func countExportUsers(path string) (int64, error) {
	f, dec, err := openUsers(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	var n int64
	for dec.More() {
		var raw json.RawMessage
		if err = dec.Decode(&raw); err != nil {
			return 0, safe("auth export user invalid")
		}
		n++
	}
	return n, nil
}

// readAuthExport maps every export user and writes it as one JSON line to out, so memory stays
// O(one user) plus the email/phone sets. Users that cannot be loaded (a repeated email or phone,
// which hasura-auth keeps unique) are left out and recorded as user exemptions.
func readAuthExport(path string, out io.Writer) (authResult, error) {
	res := authResult{Algorithms: map[string]int{}}
	count, err := countExportUsers(path)
	if err != nil {
		return res, err
	}
	res.Count = count
	f, dec, err := openUsers(path)
	if err != nil {
		return res, err
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(out)
	emails, phones := map[string]bool{}, map[string]bool{}
	for dec.More() {
		var e exportUser
		if err = dec.Decode(&e); err != nil || e.LocalID == "" {
			return res, safe("auth export user invalid")
		}
		u, err := mapUser(e)
		if err != nil {
			return res, err
		}
		reason := ""
		if lower := strings.ToLower(e.Email); e.Email != "" && emails[lower] {
			reason = "duplicate email; Firebase uid " + e.LocalID
		} else if e.PhoneNumber != "" && phones[e.PhoneNumber] {
			reason = "duplicate phone number; Firebase uid " + e.LocalID
		}
		if reason != "" {
			res.Exemptions = append(res.Exemptions, portable.Exemption{Kind: "user", ID: u.ID, Reason: reason})
			continue
		}
		if e.Email != "" {
			emails[strings.ToLower(e.Email)] = true
		}
		if e.PhoneNumber != "" {
			phones[e.PhoneNumber] = true
		}
		if u.HashAlgorithm == "unsupported" {
			res.ResetRequired = append(res.ResetRequired, u.ID)
		}
		if fb, ok := u.Metadata["firebase"].(map[string]any); ok && fb["anonymous"] == true {
			res.Anonymous++
		}
		if err = enc.Encode(u); err != nil {
			return res, safe("auth users write failed")
		}
		res.Algorithms[u.HashAlgorithm]++
		res.Written++
	}
	return res, nil
}

func mapUser(e exportUser) (verify.User, error) {
	u := verify.User{ID: uuid.NewSHA1(userNamespace, []byte("firebase:"+e.LocalID)).String(), EmailVerified: e.EmailVerified, Disabled: e.Disabled, HashAlgorithm: "none", Roles: []string{}}
	if e.Email != "" {
		u.Email = &e.Email
	}
	if e.PhoneNumber != "" {
		u.Phone = &e.PhoneNumber
	}
	if e.DisplayName != "" {
		u.DisplayName = &e.DisplayName
	}
	if e.PasswordHash != "" {
		u.HashAlgorithm = "unsupported"
	}
	var err error
	if u.CreatedAt, err = epochMillis(e.CreatedAt); err != nil {
		return u, err
	}
	meta := map[string]any{"uid": e.LocalID}
	if e.PhotoURL != "" {
		meta["photo_url"] = e.PhotoURL
	}
	lastRaw := e.LastSignedInAt
	if lastRaw == nil {
		lastRaw = e.LastLoginAt
	}
	if lastRaw != nil {
		last, err := epochMillis(lastRaw)
		if err != nil {
			return u, err
		}
		if last != epochZero {
			meta["last_signed_in_at"] = last
		}
	}
	var providers []string
	for _, p := range e.Providers {
		if p.ProviderID != "" {
			providers = append(providers, p.ProviderID)
		}
	}
	if len(providers) > 0 {
		meta["providers"] = providers
	}
	if e.Email == "" && e.PhoneNumber == "" && len(providers) == 0 {
		meta["anonymous"] = true // no credential of any kind: Firebase anonymous auth
	}
	if e.CustomAttributes != "" {
		var attrs map[string]any
		if json.Unmarshal([]byte(e.CustomAttributes), &attrs) != nil {
			return u, safe("auth export customAttributes is not a JSON object")
		}
		if len(attrs) > 0 {
			meta["custom_attributes"] = attrs
		}
	}
	u.Metadata = map[string]any{"firebase": meta}
	return u, nil
}

const epochZero = "1970-01-01T00:00:00Z"

// epochMillis turns Firebase's createdAt (a string or number of epoch milliseconds) into RFC 3339.
// An absent or null value is the epoch; a present value that is not a number is an error, never a
// silent default.
func epochMillis(raw json.RawMessage) (string, error) {
	lit := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if lit == "" || lit == "null" {
		return epochZero, nil
	}
	ms, err := strconv.ParseInt(lit, 10, 64)
	if err != nil || ms < 0 {
		return "", safe("auth export timestamp is not epoch milliseconds")
	}
	if ms == 0 {
		return epochZero, nil
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339Nano), nil
}
