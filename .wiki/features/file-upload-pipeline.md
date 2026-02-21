# File Upload Pipeline

Enterprise-grade file upload system with multipart uploads, thumbnail generation, virus scanning, and GraphQL integration.

## Overview

nself's file upload pipeline provides a complete, production-ready solution for handling file uploads in your applications. Built on MinIO (S3-compatible storage), it offers advanced features typically requiring extensive custom development.

## Key Features

### 1. Multipart Upload

Automatic chunking for large files with parallel upload support.

- Automatic for files > 100MB
- 5MB chunks by default
- Progress tracking
- Resume capability
- Optimized bandwidth usage

### 2. Thumbnail Generation

Responsive thumbnails in modern formats.

- Multiple sizes (150x150, 300x300, 600x600)
- Multiple formats (AVIF, WebP, JPEG)
- Video frame extraction
- Automatic optimization

### 3. Virus Scanning

Optional ClamAV integration for malware detection.

- Real-time scanning before upload completes
- Daily virus definition updates
- Quarantine infected files
- Security event logging

### 4. Automatic Compression

Smart compression for large files.

- Gzip compression for text files > 10MB
- Skips already compressed formats
- Transparent decompression on download
- Saves storage space and bandwidth

### 5. GraphQL Integration

Auto-generated mutations, queries, and subscriptions.

- Complete CRUD operations
- TypeScript types
- React hooks
- Row-level security

### 6. Multiple Storage Backends

Support for various storage providers.

- MinIO (default, S3-compatible)
- Amazon S3
- Google Cloud Storage
- Azure Blob Storage

## Quick Start

### 1. Enable Storage

```bash
# .env.dev
MINIO_ENABLED=true
UPLOAD_ENABLE_THUMBNAILS=true
UPLOAD_ENABLE_COMPRESSION=true
```

### 2. Initialize

```bash
nself build && nself start
nself storage init
```

### 3. Upload Files

```bash
nself storage upload photo.jpg --thumbnails
```

### 4. Set Up Database

```bash
nself storage graphql-setup
psql $DATABASE_URL < .backend/storage/migrations/*.sql
hasura metadata apply
```

## Architecture

```
┌─────────────────┐
│   Frontend      │
│   (React/Next)  │
└────────┬────────┘
         │ GraphQL Mutation
         ↓
┌─────────────────┐
│   Hasura        │
│   GraphQL API   │
└────────┬────────┘
         │ Custom Action
         ↓
┌─────────────────┐
│  Upload Service │
│  (nself)        │
├─────────────────┤
│ • Validation    │
│ • Virus Scan    │
│ • Compression   │
│ • Thumbnails    │
└────────┬────────┘
         │ S3 API
         ↓
┌─────────────────┐
│    MinIO        │
│  Object Storage │
└─────────────────┘
```

## Configuration

### Environment Variables

```bash
# Storage Backend
STORAGE_BACKEND=minio
MINIO_ENDPOINT=http://minio:9000
MINIO_ACCESS_KEY=minioadmin
MINIO_SECRET_KEY=minioadmin
MINIO_BUCKET=uploads
STORAGE_PUBLIC_URL=http://storage.localhost

# Upload Features
UPLOAD_ENABLE_MULTIPART=true
UPLOAD_ENABLE_THUMBNAILS=true
UPLOAD_ENABLE_VIRUS_SCAN=false
UPLOAD_ENABLE_COMPRESSION=true

# Thumbnail Settings
UPLOAD_THUMBNAIL_SIZES=150x150,300x300,600x600
UPLOAD_IMAGE_FORMATS=avif,webp,jpg

# File Limits
UPLOAD_MAX_FILE_SIZE=5368709120  # 5GB
UPLOAD_CHUNK_SIZE=5242880        # 5MB
```

### Database Schema

Auto-generated with `nself storage graphql-setup`:

```sql
CREATE TABLE files (
  id uuid PRIMARY KEY,
  name text NOT NULL,
  size integer NOT NULL,
  mime_type text NOT NULL,
  path text NOT NULL UNIQUE,
  url text NOT NULL,
  thumbnail_url text,
  user_id uuid REFERENCES auth.users(id),
  created_at timestamptz DEFAULT now(),
  updated_at timestamptz DEFAULT now(),
  metadata jsonb DEFAULT '{}',
  tags text[] DEFAULT ARRAY[]::text[],
  is_public boolean DEFAULT false
);
```

## Usage Examples

### CLI Upload

```bash
# Basic upload
nself storage upload file.jpg

# With all features
nself storage upload photo.png \
  --thumbnails \
  --virus-scan \
  --compression

# To specific path
nself storage upload avatar.jpg --dest users/123/avatars/

# List files
nself storage list users/123/

# Delete file
nself storage delete users/123/old-file.jpg
```

### React Component

```typescript
import { useFileUpload } from '@/hooks/useFiles';

function FileUpload() {
  const { upload, loading } = useFileUpload();

  const handleUpload = async (file: File) => {
    const result = await upload(file, {
      path: `users/${userId}/`,
      isPublic: false,
    });

    console.log('Uploaded:', result.data.uploadFile);
  };

  return (
    <input
      type="file"
      onChange={(e) => handleUpload(e.target.files[0])}
      disabled={loading}
    />
  );
}
```

### GraphQL Mutation

```graphql
mutation UploadFile($file: Upload!) {
  uploadFile(file: $file, isPublic: false) {
    id
    name
    size
    url
    thumbnailUrl
    createdAt
  }
}
```

## Security Features

### Row-Level Security (RLS)

Users can only access their own files:

```sql
CREATE POLICY files_select_own
ON files FOR SELECT
USING (auth.uid() = user_id OR is_public = true);
```

### File Type Validation

Server-side MIME type detection:

```bash
# Validates actual file content, not extension
mime_type="$(file --mime-type -b "${file_path}")"
```

### Virus Scanning

ClamAV integration:

```bash
# Enable in .env.prod
UPLOAD_ENABLE_VIRUS_SCAN=true

# Install ClamAV
sudo apt-get install clamav
sudo freshclam
```

### Content Security Policy

Prevent script execution in uploaded files:

```nginx
add_header Content-Security-Policy "
  default-src 'none';
  img-src 'self';
  script-src 'none';
" always;
```

## Performance Optimization

### CDN Integration

```bash
# .env.prod
STORAGE_PUBLIC_URL=https://cdn.yourdomain.com

# CloudFlare settings
# - Cache images: 1 year
# - Polish: Lossless
# - Mirage: On
```

### Responsive Images

Use thumbnails for different screen sizes:

```html
<picture>
  <source
    type="image/avif"
    srcset="thumbnail/150x150.avif 150w,
            thumbnail/300x300.avif 300w"
  />
  <img src="original.jpg" alt="..." />
</picture>
```

### Lazy Loading

```typescript
<img src={file.url} alt={file.name} loading="lazy" />
```

## Monitoring

### Storage Usage

```sql
-- Total storage per user
SELECT
  user_id,
  COUNT(*) as file_count,
  SUM(size) as total_bytes
FROM files
GROUP BY user_id;
```

### Upload Analytics

```sql
-- Uploads by date
SELECT
  DATE(created_at) as date,
  COUNT(*) as uploads,
  SUM(size) as total_bytes
FROM files
GROUP BY DATE(created_at)
ORDER BY date DESC;
```

### Audit Logging

```sql
-- Create audit log
CREATE TABLE file_audit_log (
  id uuid PRIMARY KEY,
  action text NOT NULL,
  file_id uuid REFERENCES files(id),
  user_id uuid REFERENCES auth.users(id),
  ip_address inet,
  created_at timestamptz DEFAULT now()
);
```

## Limits and Quotas

### Default Limits

- Max file size: 5GB
- Chunk size: 5MB
- Thumbnail sizes: 150x150, 300x300, 600x600
- Supported formats: Images, videos, documents

### Storage Quotas

Implement per-user limits:

```sql
ALTER TABLE auth.users
ADD COLUMN storage_quota_bytes bigint DEFAULT 1073741824; -- 1GB

CREATE OR REPLACE FUNCTION check_storage_quota()
RETURNS TRIGGER AS $$
DECLARE
  current_usage bigint;
  user_quota bigint;
BEGIN
  SELECT COALESCE(SUM(size), 0)
  INTO current_usage
  FROM files
  WHERE user_id = NEW.user_id;

  SELECT storage_quota_bytes
  INTO user_quota
  FROM auth.users
  WHERE id = NEW.user_id;

  IF (current_usage + NEW.size) > user_quota THEN
    RAISE EXCEPTION 'Storage quota exceeded';
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER enforce_storage_quota
BEFORE INSERT ON files
FOR EACH ROW
EXECUTE FUNCTION check_storage_quota();
```

## Troubleshooting

### Common Issues

**Upload fails:**
- Check MinIO is running: `nself status | grep minio`
- Verify bucket exists: `mc ls nself/uploads`
- Check file size limits

**Thumbnails not generated:**
- Install ImageMagick: `brew install imagemagick`
- Install FFmpeg: `brew install ffmpeg`
- Enable in config: `UPLOAD_ENABLE_THUMBNAILS=true`

**Virus scan fails:**
- Install ClamAV: `sudo apt-get install clamav`
- Update definitions: `sudo freshclam`
- Check service: `systemctl status clamav-daemon`

### Debug Mode

```bash
# Enable debug logging
export LOG_LEVEL=debug

# Test upload
nself storage test

# Check logs
nself logs minio
```

## Comparison with Alternatives

| Feature | nself | Supabase | Firebase | Custom S3 |
|---------|-------|----------|----------|-----------|
| **Multipart Upload** | ✓ Auto | ✓ Manual | ✓ Manual | ✗ DIY |
| **Thumbnails** | ✓ Auto | ✗ DIY | ✗ DIY | ✗ DIY |
| **Virus Scan** | ✓ Optional | ✗ No | ✗ No | ✗ DIY |
| **Compression** | ✓ Auto | ✗ No | ✗ No | ✗ DIY |
| **GraphQL** | ✓ Auto | ✓ Yes | ✗ No | ✗ DIY |
| **Self-Hosted** | ✓ Yes | ✓ Yes | ✗ No | ✓ Yes |
| **Cost** | Free | Free tier | Free tier | AWS fees |

## Resources

- [File Upload Pipeline Guide](../guides/file-upload-pipeline.md)
- [File Upload Security](../security/file-upload-security.md)
- [Quick Start Tutorial](../tutorials/file-uploads-quickstart.md)
- [CLI Reference](../commands/storage.md)
- [Integration Examples](../guides/file-upload-examples.md)

## Support

- [GitHub Issues](https://github.com/nself-org/cli/issues)
- [Discord Community](https://discord.gg/nself)
- [Documentation](https://docs.nself.org)
