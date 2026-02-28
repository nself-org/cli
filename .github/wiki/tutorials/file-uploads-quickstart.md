# File Uploads Quick Start Tutorial

Get file uploads working in your nself app in 10 minutes.

> **Note:** As of v0.9.6, storage commands have been consolidated under `nself service storage`. Throughout this guide, `nself storage` refers to `nself service storage` in the new command structure.

## What You'll Build

A complete file upload system with:
- Drag & drop interface
- Image thumbnails
- File management dashboard
- User storage quotas
- Secure permissions

## Prerequisites

- nself project initialized (`nself init`)
- MinIO enabled in your `.env.dev`
- Frontend app (Next.js recommended)

## Step 1: Enable Storage (2 minutes)

### Enable MinIO in Environment

Edit `.env.dev`:

```bash
# Enable MinIO
MINIO_ENABLED=true
MINIO_BUCKET=uploads
STORAGE_PUBLIC_URL=http://storage.localhost

# Enable upload features
UPLOAD_ENABLE_THUMBNAILS=true
UPLOAD_ENABLE_COMPRESSION=true
UPLOAD_ENABLE_VIRUS_SCAN=false  # Enable if you have ClamAV

# Thumbnail configuration
UPLOAD_THUMBNAIL_SIZES=150x150,300x300,600x600
UPLOAD_IMAGE_FORMATS=avif,webp,jpg
```

### Rebuild and Restart

```bash
nself build && nself start
```

Wait for services to start, then verify MinIO is running:

```bash
nself status | grep minio
# Should show: minio ✓ Running
```

## Step 2: Initialize Storage (1 minute)

```bash
nself service storage init
```

Expected output:
```
✓ Initializing storage system...
✓ MinIO client installed
✓ Bucket 'uploads' created
✓ Storage system initialized

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
```

## Step 3: Test Upload (1 minute)

```bash
# Test with any image file
nself service storage upload ~/Downloads/photo.jpg --thumbnails
```

Expected output:
```
✓ Upload pipeline initialized
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

Open the URL in your browser to verify the upload worked.

## Step 4: Set Up Database (2 minutes)

### Generate GraphQL Integration

```bash
nself service storage graphql-setup
```

This creates:
```
.backend/storage/
├── migrations/20260130_create_files_table.sql
├── metadata/tables/public_files.yaml
├── graphql/files.graphql
├── types/files.ts
└── hooks/useFiles.ts
```

### Run Migration

```bash
# Get your database URL
source .env.dev
echo $DATABASE_URL

# Run migration
psql $DATABASE_URL < .backend/storage/migrations/*_create_files_table.sql
```

Expected output:
```
CREATE TABLE
CREATE INDEX
CREATE INDEX
CREATE FUNCTION
CREATE TRIGGER
CREATE POLICY
GRANT
```

### Apply Hasura Metadata

```bash
# Make sure Hasura is running
nself status | grep hasura

# Apply metadata
hasura metadata apply
```

## Step 5: Frontend Integration (4 minutes)

### Copy Generated Files

```bash
# Copy to your Next.js frontend
cp .backend/storage/types/files.ts src/types/
cp .backend/storage/hooks/useFiles.ts src/hooks/
```

### Create Upload Component

Create `src/components/FileUpload.tsx`:

```typescript
'use client';

import { useFileUpload, useUserFiles } from '@/hooks/useFiles';
import { useState } from 'react';

export default function FileUpload({ userId }: { userId: string }) {
  const { upload, loading } = useFileUpload();
  const { files, total, totalSize, refetch } = useUserFiles(userId);
  const [error, setError] = useState<string | null>(null);

  const handleUpload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    // Validate file size (10MB max)
    if (file.size > 10 * 1024 * 1024) {
      setError('File too large. Max size: 10MB');
      return;
    }

    try {
      setError(null);
      const result = await upload(file, {
        path: `users/${userId}/`,
        isPublic: false,
      });

      console.log('Uploaded:', result.data.uploadFile);
      refetch(); // Refresh file list
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Upload failed');
    }
  };

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round(bytes / Math.pow(k, i) * 100) / 100 + ' ' + sizes[i];
  };

  return (
    <div className="max-w-4xl mx-auto p-6">
      <div className="mb-8">
        <h2 className="text-2xl font-bold mb-4">File Upload</h2>

        <div className="border-2 border-dashed border-gray-300 rounded-lg p-8 text-center">
          <input
            type="file"
            onChange={handleUpload}
            disabled={loading}
            className="block w-full text-sm text-gray-500
              file:mr-4 file:py-2 file:px-4
              file:rounded-full file:border-0
              file:text-sm file:font-semibold
              file:bg-blue-50 file:text-blue-700
              hover:file:bg-blue-100
              disabled:opacity-50 disabled:cursor-not-allowed"
          />
          {loading && (
            <p className="mt-2 text-sm text-gray-600">Uploading...</p>
          )}
          {error && (
            <p className="mt-2 text-sm text-red-600">{error}</p>
          )}
        </div>
      </div>

      <div>
        <h3 className="text-xl font-semibold mb-4">
          Your Files ({total} files, {formatBytes(totalSize)})
        </h3>

        {files.length === 0 ? (
          <p className="text-gray-500">No files uploaded yet.</p>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            {files.map((file) => (
              <div key={file.id} className="border rounded-lg p-4">
                {file.thumbnailUrl && (
                  <img
                    src={file.thumbnailUrl}
                    alt={file.name}
                    className="w-full h-48 object-cover rounded mb-2"
                  />
                )}
                <p className="font-medium truncate">{file.name}</p>
                <p className="text-sm text-gray-500">
                  {formatBytes(file.size)}
                </p>
                <a
                  href={file.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-blue-600 hover:underline text-sm"
                >
                  Download
                </a>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
```

### Use in Page

Create `src/app/files/page.tsx`:

```typescript
import FileUpload from '@/components/FileUpload';
import { auth } from '@/lib/auth'; // Your auth provider

export default async function FilesPage() {
  const session = await auth();

  if (!session?.user?.id) {
    return <p>Please log in to upload files.</p>;
  }

  return <FileUpload userId={session.user.id} />;
}
```

## Step 6: Test End-to-End

### Start Your Frontend

```bash
cd frontend
npm run dev
```

### Open in Browser

```
http://localhost:3000/files
```

### Upload a File

1. Click "Choose File"
2. Select an image
3. Wait for upload to complete
4. File appears in the grid below

### Verify in Database

```bash
psql $DATABASE_URL -c "SELECT id, name, size, url FROM files;"
```

Should show your uploaded file.

## What You Built

Congratulations! You now have:

1. **Storage Service** - MinIO running with secure bucket
2. **Upload Pipeline** - Automatic thumbnails and compression
3. **Database Schema** - Files table with RLS permissions
4. **GraphQL API** - Mutations and queries for file operations
5. **React UI** - Upload component with file management

## Next Steps

### Add Drag & Drop

Install `react-dropzone`:

```bash
npm install react-dropzone
```

Update component:

```typescript
import { useDropzone } from 'react-dropzone';

const { getRootProps, getInputProps, isDragActive } = useDropzone({
  onDrop: async (acceptedFiles) => {
    for (const file of acceptedFiles) {
      await upload(file, {
        path: `users/${userId}/`,
        isPublic: false,
      });
    }
    refetch();
  },
  maxSize: 10 * 1024 * 1024, // 10MB
  accept: {
    'image/*': ['.jpeg', '.jpg', '.png', '.gif', '.webp'],
  },
});
```

### Add Progress Indicator

```typescript
const [progress, setProgress] = useState(0);

const handleUpload = async (file: File) => {
  const xhr = new XMLHttpRequest();

  xhr.upload.addEventListener('progress', (event) => {
    if (event.lengthComputable) {
      const percentComplete = (event.loaded / event.total) * 100;
      setProgress(percentComplete);
    }
  });

  // Upload with XHR for progress tracking
};
```

### Add File Delete

```typescript
import { useFileDelete } from '@/hooks/useFiles';

const { remove } = useFileDelete();

const handleDelete = async (fileId: string) => {
  if (confirm('Delete this file?')) {
    await remove(fileId);
    refetch();
  }
};
```

### Enable Virus Scanning

```bash
# Install ClamAV
brew install clamav  # macOS
sudo apt-get install clamav  # Ubuntu

# Update virus definitions
sudo freshclam

# Enable in .env.dev
UPLOAD_ENABLE_VIRUS_SCAN=true

# Restart
nself restart
```

### Add Storage Quotas

See [Storage Quotas Guide](../guides/QUOTAS.md) for implementing per-user storage limits.

### Production Deployment

See [File Upload Security](../security/file-upload-security.md) for production best practices.

## Troubleshooting

### Upload Fails with "Connection Refused"

**Solution:**
```bash
# Check MinIO status
nself status | grep minio

# Restart if needed
nself restart minio
```

### Files Not Showing in UI

**Solution:**
```bash
# Check Hasura permissions
hasura console

# Go to Data → files → Permissions
# Verify user role has SELECT permission
```

### Thumbnails Not Generated

**Solution:**
```bash
# Install ImageMagick
brew install imagemagick  # macOS
sudo apt-get install imagemagick  # Ubuntu

# Verify
convert --version
```

### Database Migration Fails

**Solution:**
```bash
# Check database connection
psql $DATABASE_URL -c "SELECT 1"

# If fails, verify .env.dev has correct DATABASE_URL
```

## Complete Example Repository

See the [nself-chat](https://github.com/nself-org/cli-chat) repository for a complete working example with:
- File uploads
- Drag & drop
- Image previews
- Storage quotas
- Admin dashboard

## Support

Need help?
- [Documentation](https://docs.nself.org)
- [GitHub Issues](https://github.com/nself-org/cli/issues)
- [Discord Community](https://discord.gg/nself)

## What's Next?

- [File Upload Pipeline](../features/file-upload-pipeline.md)
- [File Upload Security](../security/file-upload-security.md)
- [File Upload Examples](../guides/file-upload-examples.md)
- [Production Deployment](../deployment/PRODUCTION-DEPLOYMENT.md)
