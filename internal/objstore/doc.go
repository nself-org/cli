// Package objstore is a small stdlib S3 client: SigV4, path style,
// ListObjectsV2 with pagination, GetObject, PutObject, HeadBucket and
// CreateBucket (EPIC D19, P7-ADOPT-23).
//
// It exists because the loader, the exporter and the nhost source must move
// objects through the loopback S3 port of the project's object store (MinIO
// compatible or SeaweedFS) without a host rclone/mc dependency. Requests go
// through internal/httptimeout (NoProxy: a signed request reaches only the
// host the caller named, never follows a redirect).
//
// Secrets: errors are redacted; the secret key and any signature never appear
// in error text.
//
// Layering: L1; imports internal/httptimeout (L0) and the standard library.
package objstore
