package secrets

// Purpose: change recipients of one existing age store without exporting keys.
// Inputs: file class, action, public recipient and explicit release consent.
// Outputs: recipient list and whether the ciphertext changed; one change audit.
// Constraints: preserve operator access; never create a node-only store.
// SPORT: P7-TRUST-03, D5.

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

var recipientPattern = regexp.MustCompile(`^age1[02-9ac-hj-np-z]{58}$`)

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
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, false, errs.Newf("E541", "Secret store %s not found", req.File)
		}
		return nil, false, decryptError()
	}
	key, err := readableIdentity()
	if err != nil {
		return nil, false, err
	}
	store, err := loadStore(req.ProjectRoot, req.File)
	if err != nil {
		return nil, false, decryptError()
	}
	if req.Action == "list" {
		return append([]string{}, store.Recipients...), false, nil
	}
	operator, err := GetPublicKey(key)
	if err != nil || !recipientPattern.MatchString(operator) {
		return nil, false, decryptError()
	}
	if len(store.Recipients) == 0 {
		store.Recipients = []string{operator}
	}
	changed, err := changeRecipient(store, req, operator)
	if err != nil {
		return nil, false, err
	}
	if !changed {
		return append([]string{}, store.Recipients...), false, nil
	}
	// Persist only valid recipients: store contents are untrusted input to age argv.
	for _, r := range store.Recipients {
		if !recipientPattern.MatchString(r) {
			return nil, false, decryptError()
		}
	}
	if sink == nil {
		return nil, false, errs.New("E401", "Recipient change audit is required")
	}
	if err := saveStore(req.ProjectRoot, req.File, store); err != nil {
		return nil, false, decryptError()
	}
	project, err := filepath.Abs(req.ProjectRoot)
	if err != nil {
		return nil, true, errs.New("E401", "Cannot determine project directory")
	}
	if sink("secret-recipients", map[string]string{"action": req.Action, "file": req.File,
		"recipient": req.Recipient, "project": project, "user": auditUser()}) != nil {
		return nil, true, errs.New("E401", "Recipient change audit could not be written")
	}
	return append([]string{}, store.Recipients...), true, nil
}

// changeRecipient preserves operator access and refuses removal of the last key.
func changeRecipient(store *SecretStore, req RecipientsRequest, operator string) (bool, error) {
	index := slices.Index(store.Recipients, req.Recipient)
	if req.Action == "add" {
		if index >= 0 {
			return false, nil
		}
		store.Recipients = append(store.Recipients, req.Recipient)
		return true, nil
	}
	if req.Recipient == operator {
		return false, errs.New("E401", "Cannot remove the operator recipient; use secrets rekey")
	}
	if index < 0 {
		return false, nil
	}
	store.Recipients = slices.DeleteFunc(store.Recipients, func(r string) bool { return r == req.Recipient })
	if len(store.Recipients) == 0 {
		return false, errs.New("E401", "Cannot remove the last recipient; use secrets rekey")
	}
	return true, nil
}
