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
	msgObjectRead     = "source object read failed: check --storage-endpoint, --s3-region, the S3 keys and --bucket"
	msgBucket         = "source S3 bucket unreachable or it refused the keys: check --storage-endpoint, --s3-region, --bucket and the S3 keys"
	msgStorageChanged = "source storage changed while it was read: stop writes to the source and run the import again"
)

func init() {
	importer.AllowMessages(msgStorageLookup, msgStorageFlags, msgS3Endpoint, msgSnapshot, msgListing, msgObjectName, msgObjectMissing, msgObjectCopy, msgObjectSizeDiff, msgObjectRead, msgBucket, msgStorageChanged)
}
