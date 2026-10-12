package appwrite

// Purpose: turn Appwrite users into portable auth/users.jsonl records (EPIC D10).
// Inputs: the Users API (password + hash per user). Outputs: JSONL bytes, the manifest auth
// summary and the independent user count. Constraints: only bcrypt hashes are carried; every other
// algorithm loses its hash and lists the user as reset_required; hashes never reach errors.
import (
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // RFC 4122 name-based UUID v5, not a security use
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer/verify"
	"github.com/nself-org/cli/internal/portable"
)

// userNamespace is the fixed UUID v5 namespace for Appwrite user ids (random, never changes).
var userNamespace = [16]byte{0x6e, 0x73, 0x65, 0x6c, 0x66, 0x2d, 0x61, 0x70, 0x70, 0x77, 0x72, 0x69, 0x74, 0x65, 0x00, 0x01}

// userUUID maps an Appwrite user id (any short string) to the UUID hasura-auth requires.
func userUUID(id string) string {
	h := sha1.New() //nolint:gosec
	h.Write(userNamespace[:])
	h.Write([]byte(id))
	s := h.Sum(nil)
	s[6] = (s[6] & 0x0f) | 0x50
	s[8] = (s[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", s[0:4], s[4:6], s[6:8], s[8:10], s[10:16])
}

type apiUser struct {
	ID           string         `json:"$id"`
	CreatedAt    string         `json:"$createdAt"`
	Registration string         `json:"registration"`
	Name         string         `json:"name"`
	Email        string         `json:"email"`
	Phone        string         `json:"phone"`
	Password     string         `json:"password"`
	Hash         string         `json:"hash"`
	Status       *bool          `json:"status"`
	EmailVerify  bool           `json:"emailVerification"`
	Labels       []string       `json:"labels"`
	Prefs        map[string]any `json:"prefs"`
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// convertUser maps one API user. The result's HashAlgorithm is bcrypt, none, or the source
// algorithm name when the hash cannot be carried.
func convertUser(u apiUser) (verify.User, error) {
	created := u.Registration
	if created == "" {
		created = u.CreatedAt
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return verify.User{}, errs.New("E520", "auth snapshot extraction failed")
	}
	meta := map[string]any{"appwrite_id": u.ID}
	if len(u.Labels) > 0 {
		meta["labels"] = u.Labels
	}
	if len(u.Prefs) > 0 {
		meta["prefs"] = u.Prefs
	}
	out := verify.User{ID: userUUID(u.ID), Email: nonEmpty(strings.ToLower(u.Email)), EmailVerified: u.EmailVerify, Phone: nonEmpty(u.Phone), DisplayName: nonEmpty(u.Name), CreatedAt: t.UTC().Format(time.RFC3339Nano), Disabled: u.Status != nil && !*u.Status, Metadata: meta, Roles: []string{}, HashAlgorithm: "none"}
	switch {
	case u.Password == "":
	case strings.EqualFold(u.Hash, "bcrypt") && strings.HasPrefix(u.Password, "$2"):
		out.HashAlgorithm, out.PasswordHash = "bcrypt", &u.Password
	default:
		out.HashAlgorithm = strings.ToLower(u.Hash)
		// A hash tagged bcrypt that is not a bcrypt string, or an empty tag, cannot be carried
		// and must still be listed for a reset (never "bcrypt" or "none" without a usable hash).
		if out.HashAlgorithm == "" || out.HashAlgorithm == "bcrypt" || out.HashAlgorithm == "none" {
			out.HashAlgorithm = "unsupported"
		}
	}
	return out, nil
}

// exportUsers reads every user and returns the JSONL, the auth summary and the API total.
func (c *client) exportUsers(ctx context.Context) ([]byte, portable.Auth, int64, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	auth := portable.Auth{HashAlgorithms: map[string]int{}, ResetRequired: []string{}}
	total, err := c.each(ctx, "/users", "users", func(raw json.RawMessage) error {
		var u apiUser
		if json.Unmarshal(raw, &u) != nil {
			return errs.New("E520", "auth snapshot extraction failed")
		}
		rec, e := convertUser(u)
		if e != nil {
			return e
		}
		if rec.HashAlgorithm != "bcrypt" && rec.HashAlgorithm != "none" {
			auth.ResetRequired = append(auth.ResetRequired, rec.ID)
		}
		auth.HashAlgorithms[rec.HashAlgorithm]++
		auth.Users++
		return enc.Encode(rec)
	})
	return buf.Bytes(), auth, total, err
}
