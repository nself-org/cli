package firebase

// Purpose: read a `firebase auth:export` JSON file into portable auth users.
// Inputs: the export path. Outputs: users (without any password hash), the reset list, the
// hash-algorithm tally and an independent element count. Constraints: Firebase uses a modified
// scrypt that hasura-auth cannot verify, so every user that has a password hash arrives without a
// password and is listed for reset; the hash itself is never copied into the bundle. A user id is
// a UUIDv5 of the Firebase uid, and the uid stays in metadata.firebase.uid.
import (
	"encoding/json"
	"errors"
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
	CustomAttributes string          `json:"customAttributes"`
	Providers        []struct {
		ProviderID string `json:"providerId"`
	} `json:"providerUserInfo"`
}

type authResult struct {
	Users         []verify.User
	ResetRequired []string
	Algorithms    map[string]int
	Exemptions    []portable.Exemption
	// Count is the independent element count of the users array (a second, raw pass).
	Count int64
}

// openUsers positions a decoder on the first element of the `users` array.
func openUsers(path string) (*os.File, *json.Decoder, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxExport {
		return nil, nil, errors.New("auth export unreadable")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, errors.New("auth export unreadable")
	}
	dec := json.NewDecoder(f)
	if tok, e := dec.Token(); e != nil || tok != json.Delim('{') {
		_ = f.Close()
		return nil, nil, errors.New("auth export is not a JSON object")
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
	return nil, nil, errors.New("auth export has no users array")
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
			return 0, errors.New("auth export user invalid")
		}
		n++
	}
	return n, nil
}

// readAuthExport maps every export user. Users that cannot be loaded (a repeated email or phone,
// which hasura-auth keeps unique) are left out and recorded as user exemptions.
func readAuthExport(path string) (authResult, error) {
	res := authResult{Users: []verify.User{}, Algorithms: map[string]int{}}
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
	emails, phones := map[string]bool{}, map[string]bool{}
	for dec.More() {
		var e exportUser
		if err = dec.Decode(&e); err != nil || e.LocalID == "" {
			return res, errors.New("auth export user invalid")
		}
		u := mapUser(e)
		reason := ""
		if lower := strings.ToLower(e.Email); e.Email != "" && emails[lower] {
			reason = "duplicate email"
		} else if e.PhoneNumber != "" && phones[e.PhoneNumber] {
			reason = "duplicate phone number"
		}
		if reason != "" {
			res.Exemptions = append(res.Exemptions, portable.Exemption{Kind: "user", ID: u.ID, Reason: reason})
			continue
		}
		emails[strings.ToLower(e.Email)], phones[e.PhoneNumber] = e.Email != "", e.PhoneNumber != ""
		if u.HashAlgorithm == "unsupported" {
			res.ResetRequired = append(res.ResetRequired, u.ID)
		}
		res.Algorithms[u.HashAlgorithm]++
		res.Users = append(res.Users, u)
	}
	return res, nil
}

func mapUser(e exportUser) verify.User {
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
	u.CreatedAt = epochMillis(e.CreatedAt)
	meta := map[string]any{"uid": e.LocalID}
	if e.PhotoURL != "" {
		meta["photo_url"] = e.PhotoURL
	}
	if last := epochMillis(e.LastSignedInAt); e.LastSignedInAt != nil && last != "1970-01-01T00:00:00Z" {
		meta["last_signed_in_at"] = last
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
	var attrs map[string]any
	if e.CustomAttributes != "" && json.Unmarshal([]byte(e.CustomAttributes), &attrs) == nil && len(attrs) > 0 {
		meta["custom_attributes"] = attrs
	}
	u.Metadata = map[string]any{"firebase": meta}
	return u
}

// epochMillis turns Firebase's createdAt (a string or number of epoch milliseconds) into RFC 3339.
func epochMillis(raw json.RawMessage) string {
	lit := strings.Trim(string(raw), `" `)
	ms, err := strconv.ParseInt(lit, 10, 64)
	if err != nil || ms <= 0 {
		return "1970-01-01T00:00:00Z"
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
}

// writeUsers writes users as JSON lines, one per user.
func writeUsers(w io.Writer, users []verify.User) error {
	enc := json.NewEncoder(w)
	for _, u := range users {
		if err := enc.Encode(u); err != nil {
			return errors.New("auth users write failed")
		}
	}
	return nil
}
