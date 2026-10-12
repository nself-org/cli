package nhost

// Purpose: the fixed error texts this package may show the user. Each is constant text with no
// input in it; importer.publicError replaces any other source error with a generic one.
import "github.com/nself-org/cli/internal/importer"

const (
	msgStorageLookup  = "source storage lookup failed"
	msgStorageFlags   = "source holds uploaded files: --storage-endpoint, --s3-access-key-env, --s3-secret-env and --bucket are required"
	msgS3Endpoint     = "source S3 endpoint must be https (or http on loopback) with no path"
	msgSnapshot       = "source storage snapshot failed"
	msgListing        = "source storage listing failed"
	msgObjectName     = "source object name unsupported"
	msgObjectMissing  = "source object unavailable for a storage.files row"
	msgObjectCopy     = "source object copy failed"
	msgObjectSizeDiff = "source object size differs from storage.files"
)

func init() {
	importer.AllowMessages(msgStorageLookup, msgStorageFlags, msgS3Endpoint, msgSnapshot, msgListing, msgObjectName, msgObjectMissing, msgObjectCopy, msgObjectSizeDiff)
}
