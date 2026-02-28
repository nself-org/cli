# nself storage

> **⚠️ DEPRECATED in v0.9.6**: This command has been consolidated.
> Please use `nself service storage` instead.
> See [Command Consolidation Map](../architecture/COMMAND-CONSOLIDATION-MAP.md) and [v0.9.6 Release Notes](../releases/v0.9.6.md) for details.

File storage and upload management commands.

## Synopsis

```bash
nself storage <command> [options]
```

## Description

The `storage` command provides comprehensive file upload and management capabilities for your nself application. It includes:

- Multipart upload handling for large files
- Automatic thumbnail generation (AVIF/WebP/JPEG)
- Optional virus scanning with ClamAV
- Automatic compression for large files
- GraphQL integration with auto-generated types
- MinIO/S3 storage backend support

## Commands

### upload

Upload a file to storage with optional processing.

**Usage:**
```bash
nself storage upload <file> [options]
```

**Options:**
- `--dest <path>` - Destination path in storage
- `--thumbnails` - Generate image thumbnails
- `--virus-scan` - Scan file for viruses
- `--compression` - Compress large files
- `--all-features` - Enable all features

**Examples:**

```bash
# Basic upload
nself storage upload photo.jpg

# Upload with thumbnails
nself storage upload avatar.png --thumbnails

# Upload with all features
nself storage upload document.pdf --all-features

# Upload to specific path
nself storage upload file.txt --dest users/123/documents/

# Enable specific features
nself storage upload video.mp4 --compression --virus-scan
```

**Output:**
```
✓ Initializing upload pipeline...
Uploading: photo.jpg (2.3 MiB)
MIME type: image/jpeg
Destination: 2026/01/30/abc12345/photo.jpg
✓ Generating thumbnails...
✓ Upload complete!

File Details:
  URL: http://storage.localhost/uploads/2026/01/30/abc12345/photo.jpg
  Path: 2026/01/30/abc12345/photo.jpg
  Size: 2.3 MiB
  Type: image/jpeg
```

### list

List uploaded files in storage.

**Usage:**
```bash
nself storage list [prefix]
```

**Arguments:**
- `[prefix]` - Optional path prefix to filter files

**Examples:**

```bash
# List all files
nself storage list

# List files in specific folder
nself storage list users/123/

# List all user folders
nself storage list users/
```

**Output:**
```
[2026-01-30 10:30:15] 2.3 MiB photo.jpg
[2026-01-30 11:45:22] 1.1 MiB avatar.png
[2026-01-29 09:15:33] 5.2 MiB document.pdf
```

### delete

Delete an uploaded file.

**Usage:**
```bash
nself storage delete <path>
```

**Arguments:**
- `<path>` - Path to file in storage

**Examples:**

```bash
# Delete a file
nself storage delete users/123/photo.jpg

# Delete with confirmation prompt
nself storage delete 2026/01/30/abc12345/file.txt
```

**Output:**
```
Delete file: users/123/photo.jpg? [y/N] y
✓ Deleting: users/123/photo.jpg
✓ File deleted
```

### config

Show current storage configuration.

**Usage:**
```bash
nself storage config
```

**Output:**
```
Storage Configuration
====================

Backend Configuration:
  STORAGE_BACKEND        = minio
  MINIO_ENDPOINT         = http://minio:9000
  MINIO_BUCKET           = uploads
  STORAGE_PUBLIC_URL     = http://storage.localhost

Upload Features:
  UPLOAD_ENABLE_MULTIPART     = true
  UPLOAD_ENABLE_THUMBNAILS    = true
  UPLOAD_ENABLE_VIRUS_SCAN    = false
  UPLOAD_ENABLE_COMPRESSION   = true

Thumbnail Configuration:
  UPLOAD_THUMBNAIL_SIZES = 150x150,300x300,600x600
  UPLOAD_IMAGE_FORMATS   = avif,webp,jpg

To modify configuration, edit your .env file:
  vi .env.dev
```

### status

Show upload pipeline status and available tools.

**Usage:**
```bash
nself storage status
```

**Output:**
```
Upload Pipeline Status
======================

Backend: minio
Endpoint: http://minio:9000
Bucket: uploads

Features:
  Multipart Upload: true
  Thumbnails: true
  Virus Scan: false
  Compression: true

Available Tools:
  MinIO Client: ✓
  ImageMagick: ✓
  FFmpeg: ✓
  ClamAV: ✗
```

### test

Test upload functionality with a test file.

**Usage:**
```bash
nself storage test
```

**Output:**
```
Testing upload pipeline...

✓ Pipeline initialized
Created test file: /tmp/nself_upload_test_12345.txt
Uploading: nself_upload_test_12345.txt (50 B)
✓ Upload complete!
✓ Upload test passed

All tests passed!
```

### init

Initialize storage system and verify configuration.

**Usage:**
```bash
nself storage init
```

**Output:**
```
Initializing storage system...
✓ MinIO client installed
✓ Bucket 'uploads' created
✓ Storage system initialized

Upload Pipeline Status
======================
Backend: minio
Endpoint: http://minio:9000
Bucket: uploads

Next steps:
  1. Upload a file: nself storage upload <file>
  2. View configuration: nself storage config
  3. Run tests: nself storage test
```

### graphql-setup

Generate GraphQL integration package with migrations, types, and hooks.

**Usage:**
```bash
nself storage graphql-setup [output_dir]
```

**Arguments:**
- `[output_dir]` - Output directory (default: `.backend/storage`)

**Output:**
```
Generating GraphQL integration package...

✓ Created migration: 20260130120000_create_files_table.sql
✓ Created metadata: public_files.yaml
✓ Created GraphQL operations: files.graphql
✓ Created TypeScript types: files.ts
✓ Created React hooks: useFiles.ts
✓ Created README.md

GraphQL integration package generated in: .backend/storage

Next steps:
  1. Review generated files
  2. Run migration: psql $DATABASE_URL < .backend/storage/migrations/*_create_files_table.sql
  3. Apply Hasura metadata: hasura metadata apply
  4. Copy types and hooks to your frontend
```

**Generated Files:**
```
.backend/storage/
├── migrations/
│   └── 20260130120000_create_files_table.sql
├── metadata/
│   └── tables/public_files.yaml
├── graphql/
│   └── files.graphql
├── types/
│   └── files.ts
├── hooks/
│   └── useFiles.ts
└── README.md
```

## Configuration

### Environment Variables

Configure upload pipeline in `.env.dev`:

```bash
# Storage Backend
STORAGE_BACKEND=minio              # Options: minio, s3, gcs
MINIO_ENABLED=true
MINIO_ENDPOINT=http://minio:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET=uploads
STORAGE_PUBLIC_URL=http://storage.localhost

# Upload Features
UPLOAD_ENABLE_MULTIPART=true       # Enable for files > 100MB
UPLOAD_ENABLE_THUMBNAILS=false     # Requires ImageMagick
UPLOAD_ENABLE_VIRUS_SCAN=false     # Requires ClamAV
UPLOAD_ENABLE_COMPRESSION=true     # Auto-compress large files

# Thumbnail Configuration
UPLOAD_THUMBNAIL_SIZES=150x150,300x300,600x600
UPLOAD_IMAGE_FORMATS=avif,webp,jpg

# File Limits
UPLOAD_MAX_FILE_SIZE=5368709120    # 5GB in bytes
UPLOAD_CHUNK_SIZE=5242880          # 5MB chunks
```

## Features

### Multipart Upload

Automatically enabled for files > 100MB. Files are split into chunks and uploaded in parallel for better performance.

```bash
# Uploads large file with multipart
nself storage upload large-video.mp4
```

### Thumbnail Generation

Generates responsive thumbnails in multiple formats and sizes.

**Requirements:**
- ImageMagick (`convert` command)
- FFmpeg (for video thumbnails)

```bash
# Install on macOS
brew install imagemagick ffmpeg

# Install on Ubuntu
sudo apt-get install imagemagick ffmpeg

# Upload with thumbnails
nself storage upload photo.jpg --thumbnails
```

**Generated Files:**
```
uploads/2026/01/30/abc12345/photo.jpg
uploads/2026/01/30/abc12345/thumbnails/150x150.avif
uploads/2026/01/30/abc12345/thumbnails/150x150.webp
uploads/2026/01/30/abc12345/thumbnails/150x150.jpg
uploads/2026/01/30/abc12345/thumbnails/300x300.avif
... (all sizes in all formats)
```

### Virus Scanning

Optional ClamAV integration for malware detection.

**Requirements:**
- ClamAV (`clamscan` command)

```bash
# Install on macOS
brew install clamav
freshclam  # Update virus definitions

# Install on Ubuntu
sudo apt-get install clamav clamav-daemon
sudo freshclam

# Enable in .env.dev
UPLOAD_ENABLE_VIRUS_SCAN=true

# Upload with virus scan
nself storage upload suspicious.zip --virus-scan
```

### Automatic Compression

Compresses large text files (> 10MB) automatically. Skips already compressed formats (images, videos, archives).

```bash
# Auto-compress large log file
nself storage upload large-log.txt
# → Uploaded as large-log.txt.gz
```

## Examples

### Basic Workflow

```bash
# 1. Initialize storage
nself storage init

# 2. Upload a file
nself storage upload photo.jpg --thumbnails

# 3. List uploaded files
nself storage list

# 4. Check configuration
nself storage config

# 5. Test upload system
nself storage test
```

### GraphQL Integration

```bash
# 1. Generate GraphQL package
nself storage graphql-setup

# 2. Run database migration
psql $DATABASE_URL < .backend/storage/migrations/*_create_files_table.sql

# 3. Apply Hasura metadata
hasura metadata apply

# 4. Copy to frontend
cp .backend/storage/types/files.ts src/types/
cp .backend/storage/hooks/useFiles.ts src/hooks/
```

### Production Setup

```bash
# 1. Configure production settings in .env.prod
cat >> .env.prod << EOF
MINIO_ACCESS_KEY=\${MINIO_ACCESS_KEY}
MINIO_SECRET_KEY=\${MINIO_SECRET_KEY}
UPLOAD_ENABLE_VIRUS_SCAN=true
UPLOAD_ENABLE_THUMBNAILS=true
STORAGE_PUBLIC_URL=https://cdn.yourdomain.com
EOF

# 2. Deploy
nself deploy production

# 3. Test upload
nself storage test
```

## Troubleshooting

### Upload fails with "Connection Refused"

MinIO is not running.

```bash
# Check MinIO status
nself status | grep minio

# Restart MinIO
nself restart minio
```

### Thumbnails not generated

ImageMagick not installed.

```bash
# Install ImageMagick
brew install imagemagick  # macOS
sudo apt-get install imagemagick  # Ubuntu

# Verify
convert --version
```

### Virus scan fails

ClamAV not installed or virus definitions outdated.

```bash
# Install ClamAV
brew install clamav  # macOS
sudo apt-get install clamav  # Ubuntu

# Update virus definitions
sudo freshclam
```

### MinIO client not found

Install MinIO client manually.

```bash
# macOS
brew install minio/stable/mc

# Linux
curl -sSL https://dl.min.io/client/mc/release/linux-amd64/mc -o /usr/local/bin/mc
chmod +x /usr/local/bin/mc
```

## See Also

- [File Upload Pipeline Guide](../guides/file-upload-pipeline.md)
- [File Upload Security](../security/file-upload-security.md)
- [Quick Start Tutorial](../tutorials/file-uploads-quickstart.md)
- [MinIO Service](../services/MINIO.md)
- [GraphQL API](../architecture/API.md)

## Related Commands

- `nself start` - Start all services including MinIO
- `nself status` - Check service status
- `nself logs minio` - View MinIO logs
- `nself restart minio` - Restart MinIO service
