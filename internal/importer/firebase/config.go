package firebase

// Purpose: resolve and validate every firebase source input exactly once.
// Inputs: importer flags (project, credentials-file, auth-export, bucket) and the emulator
// environment (FIRESTORE_EMULATOR_HOST, STORAGE_EMULATOR_HOST). Outputs: settings with the two
// API base URLs. Constraints: the service-account key is never read here; no value in errors.
import (
	"os"
	"regexp"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer"
)

const (
	firestoreProd = "https://firestore.googleapis.com"
	storageProd   = "https://storage.googleapis.com"
)

var (
	projectRe = regexp.MustCompile(`^[a-z][a-z0-9-]{4,60}$`)
	bucketRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,220}$`)
	hostRe    = regexp.MustCompile(`^[A-Za-z0-9.-]{1,200}:[0-9]{1,5}$`)
)

type settings struct {
	Project, CredentialsFile, AuthExport, Bucket string
	FirestoreBase, StorageBase                   string
	// FirestoreEmulator and StorageEmulator use the fixed bearer "owner" and need no key file.
	FirestoreEmulator, StorageEmulator bool
}

func env(in importer.Input, name string) string {
	if in.Env != nil {
		return in.Env(name)
	}
	return os.Getenv(name)
}

// resolve validates the flag values and picks the API endpoints. The error text is fixed and
// credential-free; it names the flag, never its value.
func resolve(in importer.Input) (settings, error) {
	s := settings{Project: in.Values["project"], CredentialsFile: in.Values["credentials-file"], AuthExport: in.Values["auth-export"], Bucket: in.Values["bucket"], FirestoreBase: firestoreProd, StorageBase: storageProd}
	if !projectRe.MatchString(s.Project) {
		return s, errs.New("E517", "firebase source requires --project with a valid project id")
	}
	if s.AuthExport == "" {
		return s, errs.New("E517", "firebase source requires --auth-export (a firebase auth:export JSON file)")
	}
	if s.Bucket != "" && !bucketRe.MatchString(s.Bucket) {
		return s, errs.New("E517", "firebase --bucket is not a valid bucket name")
	}
	if host := env(in, "FIRESTORE_EMULATOR_HOST"); host != "" {
		if !hostRe.MatchString(host) {
			return s, errs.New("E517", "FIRESTORE_EMULATOR_HOST must be host:port")
		}
		s.FirestoreBase, s.FirestoreEmulator = "http://"+host, true
	}
	if host := env(in, "STORAGE_EMULATOR_HOST"); host != "" && s.Bucket != "" {
		if !hostRe.MatchString(host) {
			return s, errs.New("E517", "STORAGE_EMULATOR_HOST must be host:port")
		}
		s.StorageBase, s.StorageEmulator = "http://"+host, true
	}
	if s.CredentialsFile != "" && (s.FirestoreEmulator || s.StorageEmulator) {
		return s, errs.New("E517", "--credentials-file and an emulator host are both set; unset one (a stale FIRESTORE_EMULATOR_HOST would read the wrong data)")
	}
	needKey := !s.FirestoreEmulator || (s.Bucket != "" && !s.StorageEmulator)
	if needKey && s.CredentialsFile == "" {
		return s, errs.New("E517", "firebase source requires --credentials-file (or FIRESTORE_EMULATOR_HOST for the emulator)")
	}
	return s, nil
}

// CheckInput validates the flag values without touching the network, so the command can show a
// precise message before the importer runs (the run layer reduces extraction errors to fixed text).
func CheckInput(in importer.Input) error {
	_, err := resolve(in)
	return err
}
