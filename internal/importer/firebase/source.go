// Package firebase registers the Firebase importer source (`nself db import firebase`).
// Purpose: read Firestore (REST), a `firebase auth:export` file and Cloud Storage (JSON API) into a
// portable bundle for the shared loader and verifier.
// Inputs: --project, --credentials-file or FIRESTORE_EMULATOR_HOST, --auth-export, --bucket.
// Outputs: a v1 portable bundle. Constraints: no key or token leaves memory; modified-scrypt users
// arrive without a password and are listed for reset.
package firebase

import (
	"context"
	"fmt"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer"
)

type Source struct{}

func init()                 { importer.Register(Source{}) }
func (Source) Name() string { return "firebase" }
func (Source) Flags() []importer.FlagSpec {
	return []importer.FlagSpec{
		{Name: "project", Usage: "Firebase project id"},
		{Name: "credentials-file", Usage: "Service-account key file (not needed with FIRESTORE_EMULATOR_HOST)"},
		{Name: "auth-export", Usage: "Path to the `firebase auth:export` JSON file"},
		{Name: "bucket", Usage: "Cloud Storage bucket to copy (optional)"},
	}
}

// tokens returns the token source for one API. Emulators take the fixed owner bearer; a real
// project needs the key file, shared by both APIs.
func (s settings) clients() (fs *firestore, store *gcs, err error) {
	var key *keyToken
	pick := func(emulator bool) (*client, error) {
		if emulator {
			return newClient(ownerToken{}), nil
		}
		if key == nil {
			if key, err = newKeyToken(s.CredentialsFile, ""); err != nil {
				return nil, errs.New("E517", "source connection configuration invalid")
			}
		}
		return newClient(key), nil
	}
	fc, err := pick(s.FirestoreEmulator)
	if err != nil {
		return nil, nil, err
	}
	fs = &firestore{c: fc, project: s.Project, base: s.FirestoreBase}
	if s.Bucket != "" {
		sc, err := pick(s.StorageEmulator)
		if err != nil {
			return nil, nil, err
		}
		store = &gcs{c: sc, base: s.StorageBase, bucket: s.Bucket}
	}
	return fs, store, nil
}

// Detect is a cheap probe: inputs valid, the API reachable, the auth export countable.
func (Source) Detect(ctx context.Context, in importer.Input) (importer.Detection, error) {
	d := importer.Detection{Kind: "firebase", Version: "firestore-rest-v1", Schemas: []string{schemaName}, Notes: []string{}}
	s, err := resolve(in)
	if err != nil {
		return d, err
	}
	users, err := countExportUsers(s.AuthExport)
	if err != nil {
		return d, errs.New("E517", "source auth lookup failed")
	}
	d.SourceUsers = int(users)
	fs, _, err := s.clients()
	if err != nil {
		return d, err
	}
	ids, err := fs.collectionIDs(ctx, "")
	if err != nil {
		return d, errs.New("E517", "source database unreachable")
	}
	d.Notes = append(d.Notes, fmt.Sprintf("%d top-level collection(s)", len(ids)), fmt.Sprintf("%d user(s) in the auth export", users))
	if s.Bucket != "" {
		d.Notes = append(d.Notes, "storage bucket "+s.Bucket)
	}
	return d, nil
}
