# File Upload Pipeline Guide

Complete guide to using nself's file upload pipeline for production applications.

## Table of Contents

1. [Overview](#overview)
2. [Quick Start](#quick-start)
3. [Configuration](#configuration)
4. [Features](#features)
5. [CLI Usage](#cli-usage)
6. [GraphQL Integration](#graphql-integration)
7. [Frontend Integration](#frontend-integration)
8. [Security Best Practices](#security-best-practices)
9. [Performance Optimization](#performance-optimization)
10. [Troubleshooting](#troubleshooting)

## Overview

The nself file upload pipeline provides enterprise-grade file handling with:

- **Multipart uploads** for large files (automatic chunking)
- **Thumbnail generation** (AVIF, WebP, JPEG formats)
- **Virus scanning** (optional ClamAV integration)
- **Automatic compression** for large files
- **Progress tracking** for uploads
- **Multiple storage backends** (MinIO, S3, GCS)
- **GraphQL integration** with auto-generated mutations
- **TypeScript types** and React hooks

## Quick Start

### 1. Enable Storage in nself

```bash
# Add to .env.dev
MINIO_ENABLED=true
MINIO_BUCKET=uploads
STORAGE_PUBLIC_URL=http://storage.localhost

# Rebuild and restart
nself build && nself start
```

### 2. Initialize Storage

```bash
nself storage init
```

### 3. Upload a File

```bash
# Basic upload
nself storage upload photo.jpg

# Upload with thumbnails
nself storage upload avatar.png --thumbnails

# Upload with all features
nself storage upload document.pdf --all-features
```

### 4. Set Up Database

```bash
# Generate GraphQL integration
nself storage graphql-setup

# Run migration
psql $DATABASE_URL < .backend/storage/migrations/*_create_files_table.sql

# Apply Hasura metadata
hasura metadata apply
```

## Configuration

### Environment Variables

Add these to your `.env.dev` file:

```bash
# Storage Backend
STORAGE_BACKEND=minio              # Options: minio, s3, gcs
MINIO_ENABLED=true
MINIO_ENDPOINT=http://minio:9000
MINIO_ACCESS_KEY=minioadmin        # Change in production!
MINIO_SECRET_KEY=minioadmin        # Change in production!
MINIO_BUCKET=uploads
STORAGE_PUBLIC_URL=http://storage.localhost

# Upload Features
UPLOAD_ENABLE_MULTIPART=true       # Enable for files > 100MB
UPLOAD_ENABLE_THUMBNAILS=false     # Requires ImageMagick
UPLOAD_ENABLE_VIRUS_SCAN=false     # Requires ClamAV
UPLOAD_ENABLE_COMPRESSION=true     # Auto-compress large files

# Thumbnail Configuration
UPLOAD_THUMBNAIL_SIZES=150x150,300x300,600x600
UPLOAD_IMAGE_FORMATS=avif,webp,jpg # Modern formats first

# File Limits
UPLOAD_MAX_FILE_SIZE=5368709120    # 5GB in bytes
UPLOAD_CHUNK_SIZE=5242880          # 5MB chunks
```

### Production Configuration

For production, override in `.env.prod`:

```bash
# Use secure credentials
MINIO_ACCESS_KEY=${MINIO_ACCESS_KEY}      # From secrets
MINIO_SECRET_KEY=${MINIO_SECRET_KEY}      # From secrets

# Enable security features
UPLOAD_ENABLE_VIRUS_SCAN=true
UPLOAD_ENABLE_THUMBNAILS=true

# Use CDN for public URL
STORAGE_PUBLIC_URL=https://cdn.yourdomain.com

# Stricter limits
UPLOAD_MAX_FILE_SIZE=1073741824    # 1GB
```

## Features

### 1. Multipart Upload

Automatic chunking for large files:

```bash
# Automatically uses multipart for files > 100MB
nself storage upload large-video.mp4
```

The pipeline:
1. Splits file into 5MB chunks
2. Uploads chunks in parallel
3. Assembles chunks on server
4. Returns complete file URL

### 2. Thumbnail Generation

Generate responsive image thumbnails:

```bash
# Generate thumbnails in AVIF, WebP, and JPEG
nself storage upload photo.jpg --thumbnails
```

Generated files:
```
uploads/2026/01/30/abc123/photo.jpg           # Original
uploads/2026/01/30/abc123/thumbnails/150x150.avif
uploads/2026/01/30/abc123/thumbnails/150x150.webp
uploads/2026/01/30/abc123/thumbnails/150x150.jpg
uploads/2026/01/30/abc123/thumbnails/300x300.avif
... (all sizes in all formats)
```

**Requirements:**
- ImageMagick (`convert` command)
- FFmpeg (for video thumbnails)

**Install on macOS:**
```bash
brew install imagemagick ffmpeg
```

**Install on Ubuntu:**
```bash
sudo apt-get install imagemagick ffmpeg
```

### 3. Virus Scanning

Optional ClamAV integration:

```bash
# Scan file before upload
nself storage upload suspicious.zip --virus-scan
```

**Install ClamAV:**

macOS:
```bash
brew install clamav
freshclam  # Update virus definitions
```

Ubuntu:
```bash
sudo apt-get install clamav clamav-daemon
sudo freshclam
```

### 4. Automatic Compression

Compress large text files:

```bash
# Auto-compress files > 10MB (excluding images/videos)
nself storage upload large-log.txt
# → Uploaded as large-log.txt.gz
```

Skipped for:
- Already compressed formats (JPEG, PNG, GIF, WebP, MP4, ZIP)
- Files under 10MB

### 5. Progress Tracking

Real-time upload progress:

```bash
nself storage upload video.mp4 --all-features
# Output:
# Uploading: video.mp4 (1.2 GiB)
# MIME type: video/mp4
# Destination: 2026/01/30/abc123/video.mp4
# Progress: [=====>          ] 45% (540 MiB / 1.2 GiB)
```

## CLI Usage

### Upload Commands

```bash
# Basic upload
nself storage upload <file>

# Upload to specific path
nself storage upload photo.jpg --dest users/123/avatars/

# Enable features
nself storage upload photo.jpg --thumbnails
nself storage upload document.pdf --virus-scan
nself storage upload large-file.bin --compression

# Enable all features
nself storage upload photo.jpg --all-features
```

### List Files

```bash
# List all files
nself storage list

# List files in folder
nself storage list users/123/

# Filter by pattern
nself storage list --filter "*.jpg"
```

### Delete Files

```bash
# Delete single file
nself storage delete users/123/photo.jpg

# Delete folder (recursive)
nself storage delete users/123/ --recursive

# Force delete (no confirmation)
nself storage delete file.txt --force
```

### Configuration and Status

```bash
# Show current configuration
nself storage config

# Show pipeline status
nself storage status

# Test upload functionality
nself storage test
```

## GraphQL Integration

### Auto-Generate Integration

```bash
# Generate complete GraphQL package
nself storage graphql-setup

# Generated files:
# .backend/storage/
#   ├── migrations/
#   │   └── 20260130120000_create_files_table.sql
#   ├── metadata/
#   │   └── tables/public_files.yaml
#   ├── graphql/
#   │   └── files.graphql
#   ├── types/
#   │   └── files.ts
#   └── hooks/
#       └── useFiles.ts
```

### Database Setup

```bash
# Run migration
psql $DATABASE_URL < .backend/storage/migrations/*_create_files_table.sql

# Apply Hasura metadata
hasura metadata apply
```

### GraphQL Mutations

```graphql
# Upload a file
mutation UploadFile($file: Upload!, $isPublic: Boolean) {
  uploadFile(file: $file, isPublic: $isPublic) {
    id
    name
    size
    url
    thumbnailUrl
    createdAt
  }
}

# Upload multiple files
mutation UploadFiles($files: [Upload!]!) {
  uploadFiles(files: $files) {
    id
    name
    url
  }
}

# Delete a file
mutation DeleteFile($id: uuid!) {
  delete_files_by_pk(id: $id) {
    id
  }
}
```

### GraphQL Queries

```graphql
# Get file by ID
query GetFile($id: uuid!) {
  files_by_pk(id: $id) {
    id
    name
    size
    mimeType
    url
    thumbnailUrl
    metadata
    tags
    user {
      displayName
      avatarUrl
    }
  }
}

# List user files
query ListUserFiles($userId: uuid!, $limit: Int = 50) {
  files(
    where: { userId: { _eq: $userId } }
    order_by: { createdAt: desc }
    limit: $limit
  ) {
    id
    name
    size
    url
    thumbnailUrl
    createdAt
  }
  files_aggregate(where: { userId: { _eq: $userId } }) {
    aggregate {
      count
      sum { size }
    }
  }
}
```

## Frontend Integration

### React Example

```typescript
import { useFileUpload, useUserFiles } from '@/hooks/useFiles';

function FileUploadComponent() {
  const { upload, loading } = useFileUpload();
  const { files, total, totalSize } = useUserFiles(userId);

  const handleUpload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    try {
      const result = await upload(file, {
        path: `users/${userId}/`,
        isPublic: false
      });

      console.log('Uploaded:', result.data.uploadFile);
      alert('File uploaded successfully!');
    } catch (error) {
      console.error('Upload failed:', error);
    }
  };

  return (
    <div>
      <input
        type="file"
        onChange={handleUpload}
        disabled={loading}
      />
      {loading && <progress />}

      <h2>{total} files ({formatBytes(totalSize)})</h2>
      <ul>
        {files.map(file => (
          <li key={file.id}>
            {file.thumbnailUrl && (
              <img src={file.thumbnailUrl} alt={file.name} />
            )}
            <a href={file.url} download>{file.name}</a>
          </li>
        ))}
      </ul>
    </div>
  );
}
```

### Next.js API Route

```typescript
// pages/api/upload.ts
import { createClient } from '@/lib/nhost';
import { UPLOAD_FILE } from '@/graphql/mutations';

export const config = {
  api: {
    bodyParser: false, // Required for file uploads
  },
};

export default async function handler(req, res) {
  if (req.method !== 'POST') {
    return res.status(405).json({ error: 'Method not allowed' });
  }

  const client = createClient(req);

  const { data, error } = await client.mutate({
    mutation: UPLOAD_FILE,
    variables: {
      file: req.body,
      isPublic: false,
    },
  });

  if (error) {
    return res.status(500).json({ error: error.message });
  }

  res.status(200).json(data.uploadFile);
}
```

### Drag & Drop Component

```typescript
import { useCallback } from 'react';
import { useDropzone } from 'react-dropzone';
import { useMultipleFileUpload } from '@/hooks/useFiles';

function DropzoneUpload() {
  const { upload, loading } = useMultipleFileUpload();

  const onDrop = useCallback(async (acceptedFiles: File[]) => {
    try {
      const result = await upload(acceptedFiles, {
        path: `users/${userId}/uploads/`,
      });

      console.log('Uploaded:', result.data.uploadFiles);
    } catch (error) {
      console.error('Upload failed:', error);
    }
  }, [upload]);

  const { getRootProps, getInputProps, isDragActive } = useDropzone({
    onDrop,
    maxSize: 5 * 1024 * 1024 * 1024, // 5GB
  });

  return (
    <div {...getRootProps()} className="dropzone">
      <input {...getInputProps()} />
      {isDragActive ? (
        <p>Drop files here...</p>
      ) : (
        <p>Drag & drop files, or click to select</p>
      )}
      {loading && <progress />}
    </div>
  );
}
```

## Security Best Practices

### 1. File Type Validation

```typescript
// Validate on frontend
const ALLOWED_TYPES = ['image/jpeg', 'image/png', 'application/pdf'];

function validateFile(file: File): boolean {
  return ALLOWED_TYPES.includes(file.type);
}

// Validate in Hasura permissions
// metadata/tables/public_files.yaml
insert_permissions:
  - role: user
    permission:
      check:
        mime_type:
          _in: ['image/jpeg', 'image/png', 'application/pdf']
```

### 2. File Size Limits

```bash
# Set in .env.prod
UPLOAD_MAX_FILE_SIZE=10485760  # 10MB for avatars
```

```typescript
// Frontend validation
const MAX_SIZE = 10 * 1024 * 1024; // 10MB

if (file.size > MAX_SIZE) {
  throw new Error('File too large');
}
```

### 3. Virus Scanning

```bash
# Enable in production
UPLOAD_ENABLE_VIRUS_SCAN=true
```

### 4. Row Level Security (RLS)

The generated migration includes RLS policies:

```sql
-- Users can only access their own files
CREATE POLICY files_select_own
ON public.files
FOR SELECT
USING (auth.uid() = user_id OR is_public = true);
```

### 5. Content Security Policy (CSP)

```nginx
# nginx config for storage domain
add_header Content-Security-Policy "default-src 'none'; img-src 'self'; style-src 'self'; script-src 'none';" always;
add_header X-Content-Type-Options "nosniff" always;
add_header X-Frame-Options "DENY" always;
```

### 6. Signed URLs (for private files)

```typescript
// Generate time-limited URL
import { signUrl } from '@/lib/storage';

const signedUrl = signUrl(file.path, {
  expiresIn: 3600, // 1 hour
});
```

## Performance Optimization

### 1. CDN Integration

```bash
# .env.prod
STORAGE_PUBLIC_URL=https://cdn.yourdomain.com

# CloudFlare CDN
# - Cache images for 1 year
# - Use Polish (automatic image optimization)
# - Enable Mirage (lazy loading)
```

### 2. Responsive Images

```typescript
// Use thumbnails for responsive images
<picture>
  <source
    type="image/avif"
    srcSet={`${file.thumbnailUrl}/150x150.avif 150w,
             ${file.thumbnailUrl}/300x300.avif 300w`}
  />
  <source
    type="image/webp"
    srcSet={`${file.thumbnailUrl}/150x150.webp 150w,
             ${file.thumbnailUrl}/300x300.webp 300w`}
  />
  <img src={file.url} alt={file.name} loading="lazy" />
</picture>
```

### 3. Lazy Loading

```typescript
// Use Intersection Observer
import { useInView } from 'react-intersection-observer';

function LazyImage({ src, alt }) {
  const { ref, inView } = useInView({ triggerOnce: true });

  return (
    <div ref={ref}>
      {inView && <img src={src} alt={alt} />}
    </div>
  );
}
```

### 4. Compression

```bash
# Enable compression for text files
UPLOAD_ENABLE_COMPRESSION=true

# Serve compressed files
# nginx will auto-decompress .gz files
```

## Troubleshooting

### Upload Fails with "Connection Refused"

**Problem:** MinIO is not running

**Solution:**
```bash
# Check MinIO status
nself status | grep minio

# Restart MinIO
nself restart minio

# Check logs
nself logs minio
```

### Thumbnails Not Generated

**Problem:** ImageMagick not installed

**Solution:**
```bash
# Install ImageMagick
brew install imagemagick  # macOS
sudo apt-get install imagemagick  # Ubuntu

# Verify installation
convert --version
```

### Virus Scan Fails

**Problem:** ClamAV not installed or outdated definitions

**Solution:**
```bash
# Install ClamAV
brew install clamav  # macOS
sudo apt-get install clamav  # Ubuntu

# Update virus definitions
sudo freshclam

# Start ClamAV daemon (optional)
sudo clamd
```

### File Not Accessible

**Problem:** File is private but user is not authenticated

**Solution:**
```typescript
// Set file as public
await upload(file, { isPublic: true });

// Or use signed URL
const signedUrl = await getSignedUrl(fileId);
```

### Upload Progress Not Showing

**Problem:** MinIO client doesn't support progress for small files

**Solution:**
Files must be > 100MB for multipart upload progress tracking. For smaller files, progress is instant.

## Next Steps

- [Storage Service Configuration](../services/minio.md)
- [GraphQL API Reference](../reference/api/README.md)
- [Security Best Practices](../security/SECURITY-BEST-PRACTICES.md)
- [CDN Integration](../deployment/PRODUCTION-DEPLOYMENT.md)

## Support

For issues or questions:
- [GitHub Issues](https://github.com/nself-org/cli/issues)
- [Discord Community](https://discord.gg/nself)
- [Documentation](https://docs.nself.org)
