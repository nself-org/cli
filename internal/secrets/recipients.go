package secrets

// Purpose: change recipients of one existing age store without exporting keys.
// Inputs: file class, action, public recipient and explicit release consent.
// Outputs: recipient list and whether the ciphertext changed; one change audit.
// Constraints: preserve operator access; never create a node-only store.
// SPORT: P7-TRUST-03, D5.

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

var recipientPattern = regexp.MustCompile(`^age1[02-9ac-hj-np-z]{58}$`)

// checkStorePath refuses symlinks and special files without following them.
// Inputs: expected directory, mapped path and missing-store error.
// Outputs: nil for a real directory and regular file, otherwise fixed errors.
// Constraints: age opens by name later; SPORT: P7-TRUST-03, G1.
func checkStorePath(dir, path string, missing error) error {
	info, err := os.Lstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return missing
		}
		return decryptError()
	}
	if !info.IsDir() {
		return decryptError()
	}
	info, err = os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return missing
		}
		return decryptError()
	}
	if !info.Mode().IsRegular() || storeHasMultipleLinks(info) {
		return decryptError()
	}
	// Residual TOCTOU: age opens this name after the check. A racing writer
	// already needs write access to .secrets (or the team store directory).
	return nil
}

// RecipientsRequest selects a store and an owner-authorized recipient operation.
type RecipientsRequest struct {
	ProjectRoot, File, Action, Recipient string
	AllowReleaseClass                    bool
}

// recipientFile validates names and identifies release/deploy classes.
func recipientFile(file string) (release bool, err error) {
	switch {
	case file == "ci":
		return false, nil
	case file == "release":
		return true, nil
	case strings.HasPrefix(file, "ci-") && validRefEnv(strings.TrimPrefix(file, "ci-")):
		return false, nil
	case validRefEnv(file):
		return true, nil
	default:
		return false, errs.New("E540", "Invalid secret store file name")
	}
}

// Recipients loads only after all guards, and sanitizes every store error.
func Recipients(req RecipientsRequest, sink AuditFunc) ([]string, bool, error) {
	release, err := recipientFile(req.File)
	if err != nil {
		return nil, false, err
	}
	if req.Action != "add" && req.Action != "remove" && req.Action != "list" {
		return nil, false, errs.New("E401", "Unknown recipient action")
	}
	if req.Action != "list" && release && !req.AllowReleaseClass {
		return nil, false, errs.New("E544", "Release-class recipient changes require --allow-release-class")
	}
	if req.Action != "list" && !recipientPattern.MatchString(req.Recipient) {
		return nil, false, errs.New("E401", "Invalid age public recipient")
	}
	path := filepath.Join(req.ProjectRoot, ".secrets", req.File+".age")
	missing := errs.Newf("E541", "Secret store %s not found", req.File)
	if err := checkStorePath(filepath.Join(req.ProjectRoot, ".secrets"), path, missing); err != nil {
		return nil, false, err
	}
	unlock, err := lockRecipientStore(path)
	if err != nil {
		return nil, false, err
	}
	defer unlock()
	key, err := readableIdentity()
	if err != nil {
		return nil, false, err
	}
	store, err := loadStore(req.ProjectRoot, req.File)
	if err != nil {
		return nil, false, decryptError()
	}
	// Decrypted metadata is untrusted even on list and unchanged paths.
	if len(store.Recipients) == 0 {
		return nil, false, decryptError()
	}
	for _, r := range store.Recipients {
		if !recipientPattern.MatchString(r) {
			return nil, false, decryptError()
		}
	}
	if req.Action == "list" {
		return append([]string{}, store.Recipients...), false, nil
	}
	operator, err := deriveRecipientKey(key)
	if err != nil || !recipientPattern.MatchString(operator) {
		return nil, false, decryptError()
	}
	owner := store.Recipients[0]
	if operator != owner {
		return nil, false, errs.New("E544", "Only the store owner may change recipients")
	}
	changed, err := changeRecipient(store, req, owner)
	if err != nil {
		return nil, false, err
	}
	if !changed {
		return append([]string{}, store.Recipients...), false, nil
	}
	if sink == nil {
		return nil, false, errs.New("E401", "Recipient change audit is required")
	}
	project, err := filepath.Abs(req.ProjectRoot)
	if err != nil {
		return nil, false, errs.New("E401", "Cannot determine project directory")
	}
	if sink("secret-recipients", map[string]string{"action": req.Action, "file": req.File,
		"recipient": req.Recipient, "project": project, "user": auditUser()}) != nil {
		return nil, false, errs.New("E401", "Recipient change audit could not be written")
	}
	if !slices.Contains(store.Recipients, owner) {
		return nil, false, errs.New("E544", "The store owner must remain a recipient")
	}
	if err := checkStorePath(filepath.Join(req.ProjectRoot, ".secrets"), path, missing); err != nil {
		return nil, false, err
	}
	if err := saveStore(req.ProjectRoot, req.File, store); err != nil {
		return nil, false, decryptError()
	}
	return append([]string{}, store.Recipients...), true, nil
}

// deriveRecipientKey uses identity material, never its untrusted public comment.
func deriveRecipientKey(key string) (string, error) {
	data, err := exec.Command("age-keygen", "-y", key).Output()
	if err != nil {
		return "", decryptError()
	}
	public := strings.TrimSpace(string(data))
	if !recipientPattern.MatchString(public) {
		return "", decryptError()
	}
	return public, nil
}

// changeRecipient preserves the store owner's access during additions and removals.
func changeRecipient(store *SecretStore, req RecipientsRequest, owner string) (bool, error) {
	index := slices.Index(store.Recipients, req.Recipient)
	if req.Action == "add" {
		if index >= 0 {
			return false, nil
		}
		store.Recipients = append(store.Recipients, req.Recipient)
		return true, nil
	}
	if req.Recipient == owner {
		return false, errs.New("E544", "Cannot remove the store owner; use secrets rekey")
	}
	if index < 0 {
		return false, nil
	}
	store.Recipients = slices.DeleteFunc(store.Recipients, func(r string) bool { return r == req.Recipient })
	if len(store.Recipients) == 0 {
		return false, errs.New("E544", "Cannot remove the last recipient; use secrets rekey")
	}
	return true, nil
}
