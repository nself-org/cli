package nhost

// Purpose: the fixed error texts this package may show the user. Each is constant text with no
// input in it; importer.publicError replaces any other source error with a generic one.
import "github.com/nself-org/cli/internal/importer"

const (
	msgAuthLookup      = "source auth lookup failed"
	msgNoSize          = "source storage.files row has no size, so its object cannot be checked"
	msgStorageLookup   = "source storage lookup failed"
	msgStorageFlags    = "source holds uploaded files: --storage-endpoint, --s3-access-key-env, --s3-secret-env and --bucket are required"
	msgS3Endpoint      = "source S3 endpoint must be https (or http on loopback) with no path"
	msgSnapshot        = "source storage snapshot failed"
	msgListing         = "source storage listing failed"
	msgObjectName      = "source object name unsupported"
	msgObjectMissing   = "source object unavailable for a storage.files row"
	msgObjectCopy      = "source object copy failed"
	msgObjectSizeDiff  = "source object size differs from storage.files"
	msgObjectRead      = "source object read failed: check --storage-endpoint, --s3-region, the S3 keys and --bucket"
	msgTooManyFiles    = "source holds more than 100000 files, which is more than one bundle manifest can describe"
	msgUnsupportedAuth = "source has users with MFA, OAuth provider links or security keys, which are not carried yet: remove them on the source or recreate those accounts on the target"
	msgStorageChanged  = "source storage changed while it was read: stop writes to the source and run the import again"
)

func init() {
	importer.AllowMessages(msgAuthLookup, msgStorageLookup, msgStorageFlags, msgS3Endpoint, msgSnapshot, msgListing, msgObjectName, msgObjectMissing, msgObjectCopy, msgObjectSizeDiff, msgObjectRead, msgTooManyFiles, msgUnsupportedAuth, msgNoSize, msgStorageChanged)
}
