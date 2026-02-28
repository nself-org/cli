# File Upload Integration Examples

Real-world examples of file upload integration patterns for nself applications.

## Table of Contents

1. [Avatar Upload](#avatar-upload)
2. [Multi-File Upload](#multi-file-upload)
3. [Drag & Drop Interface](#drag--drop-interface)
4. [Progress Tracking](#progress-tracking)
5. [Image Cropping](#image-cropping)
6. [File Organization](#file-organization)
7. [Direct Upload to S3](#direct-upload-to-s3)
8. [Background Processing](#background-processing)

## Avatar Upload

Simple avatar upload with preview and cropping.

### Component

```typescript
// src/components/AvatarUpload.tsx
'use client';

import { useFileUpload } from '@/hooks/useFiles';
import { useState } from 'react';
import Image from 'next/image';

interface AvatarUploadProps {
  userId: string;
  currentAvatar?: string;
  onUploadComplete?: (url: string) => void;
}

export default function AvatarUpload({
  userId,
  currentAvatar,
  onUploadComplete,
}: AvatarUploadProps) {
  const { upload, loading } = useFileUpload();
  const [preview, setPreview] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const handleFileChange = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    // Validate file type
    if (!file.type.startsWith('image/')) {
      setError('Please select an image file');
      return;
    }

    // Validate file size (2MB max for avatars)
    if (file.size > 2 * 1024 * 1024) {
      setError('Image too large. Max size: 2MB');
      return;
    }

    // Show preview
    const reader = new FileReader();
    reader.onload = (e) => setPreview(e.target?.result as string);
    reader.readAsDataURL(file);

    try {
      setError(null);

      // Upload with thumbnail generation
      const result = await upload(file, {
        path: `avatars/${userId}/`,
        isPublic: true,
      });

      const uploadedFile = result.data.uploadFile;

      // Use thumbnail for avatar (150x150)
      const avatarUrl = uploadedFile.thumbnailUrl || uploadedFile.url;

      // Update user profile
      await fetch('/api/user/update-avatar', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ avatarUrl }),
      });

      onUploadComplete?.(avatarUrl);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Upload failed');
      setPreview(null);
    }
  };

  return (
    <div className="flex flex-col items-center gap-4">
      <div className="relative">
        {/* Current Avatar */}
        <div className="w-32 h-32 rounded-full overflow-hidden bg-gray-200">
          {preview || currentAvatar ? (
            <Image
              src={preview || currentAvatar || ''}
              alt="Avatar"
              width={128}
              height={128}
              className="object-cover"
            />
          ) : (
            <div className="w-full h-full flex items-center justify-center text-gray-400">
              No avatar
            </div>
          )}
        </div>

        {/* Upload Button */}
        <label
          className="absolute bottom-0 right-0 bg-blue-600 text-white p-2 rounded-full cursor-pointer hover:bg-blue-700 disabled:opacity-50"
        >
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          <input
            type="file"
            accept="image/*"
            onChange={handleFileChange}
            disabled={loading}
            className="hidden"
          />
        </label>
      </div>

      {/* Loading State */}
      {loading && (
        <p className="text-sm text-gray-600">Uploading...</p>
      )}

      {/* Error Message */}
      {error && (
        <p className="text-sm text-red-600">{error}</p>
      )}
    </div>
  );
}
```

### Usage

```typescript
// src/app/profile/page.tsx
import AvatarUpload from '@/components/AvatarUpload';
import { auth } from '@/lib/auth';

export default async function ProfilePage() {
  const session = await auth();

  return (
    <div className="max-w-2xl mx-auto p-6">
      <h1 className="text-2xl font-bold mb-6">Profile</h1>

      <AvatarUpload
        userId={session.user.id}
        currentAvatar={session.user.avatarUrl}
        onUploadComplete={(url) => {
          console.log('Avatar updated:', url);
          // Optionally refresh page or update state
        }}
      />
    </div>
  );
}
```

## Multi-File Upload

Upload multiple files at once with progress tracking.

```typescript
// src/components/MultiFileUpload.tsx
'use client';

import { useMultipleFileUpload } from '@/hooks/useFiles';
import { useState } from 'react';

interface UploadProgress {
  file: File;
  progress: number;
  status: 'pending' | 'uploading' | 'complete' | 'error';
  error?: string;
  url?: string;
}

export default function MultiFileUpload({ userId }: { userId: string }) {
  const { upload, loading } = useMultipleFileUpload();
  const [uploads, setUploads] = useState<UploadProgress[]>([]);

  const handleFilesChange = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files || []);
    if (files.length === 0) return;

    // Initialize progress tracking
    const initialProgress: UploadProgress[] = files.map(file => ({
      file,
      progress: 0,
      status: 'pending',
    }));
    setUploads(initialProgress);

    try {
      // Upload all files
      for (let i = 0; i < files.length; i++) {
        const file = files[i];

        // Update status to uploading
        setUploads(prev => prev.map((item, idx) =>
          idx === i ? { ...item, status: 'uploading' } : item
        ));

        try {
          const result = await upload([file], {
            path: `users/${userId}/documents/`,
            isPublic: false,
          });

          // Update status to complete
          setUploads(prev => prev.map((item, idx) =>
            idx === i
              ? {
                  ...item,
                  status: 'complete',
                  progress: 100,
                  url: result.data.uploadFiles[0].url,
                }
              : item
          ));
        } catch (err) {
          // Update status to error
          setUploads(prev => prev.map((item, idx) =>
            idx === i
              ? {
                  ...item,
                  status: 'error',
                  error: err instanceof Error ? err.message : 'Upload failed',
                }
              : item
          ));
        }
      }
    } catch (err) {
      console.error('Multi-file upload failed:', err);
    }
  };

  return (
    <div className="space-y-4">
      <input
        type="file"
        multiple
        onChange={handleFilesChange}
        disabled={loading}
        className="block w-full"
      />

      {uploads.length > 0 && (
        <div className="space-y-2">
          {uploads.map((upload, idx) => (
            <div key={idx} className="border rounded p-3">
              <div className="flex items-center justify-between mb-2">
                <span className="text-sm font-medium truncate">
                  {upload.file.name}
                </span>
                <span className={`text-xs ${
                  upload.status === 'complete' ? 'text-green-600' :
                  upload.status === 'error' ? 'text-red-600' :
                  'text-gray-600'
                }`}>
                  {upload.status}
                </span>
              </div>

              {upload.status === 'uploading' && (
                <div className="w-full bg-gray-200 rounded-full h-2">
                  <div
                    className="bg-blue-600 h-2 rounded-full transition-all"
                    style={{ width: `${upload.progress}%` }}
                  />
                </div>
              )}

              {upload.status === 'error' && (
                <p className="text-xs text-red-600">{upload.error}</p>
              )}

              {upload.status === 'complete' && upload.url && (
                <a
                  href={upload.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-xs text-blue-600 hover:underline"
                >
                  View file
                </a>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
```

## Drag & Drop Interface

User-friendly drag and drop with react-dropzone.

```typescript
// src/components/DropzoneUpload.tsx
'use client';

import { useCallback } from 'react';
import { useDropzone } from 'react-dropzone';
import { useMultipleFileUpload } from '@/hooks/useFiles';

export default function DropzoneUpload({ userId }: { userId: string }) {
  const { upload, loading } = useMultipleFileUpload();

  const onDrop = useCallback(async (acceptedFiles: File[]) => {
    try {
      const result = await upload(acceptedFiles, {
        path: `users/${userId}/uploads/`,
        isPublic: false,
      });

      console.log('Uploaded files:', result.data.uploadFiles);
      alert(`Successfully uploaded ${acceptedFiles.length} file(s)`);
    } catch (error) {
      console.error('Upload failed:', error);
      alert('Upload failed. Please try again.');
    }
  }, [upload, userId]);

  const { getRootProps, getInputProps, isDragActive } = useDropzone({
    onDrop,
    maxSize: 10 * 1024 * 1024, // 10MB
    accept: {
      'image/*': ['.jpeg', '.jpg', '.png', '.gif', '.webp'],
      'application/pdf': ['.pdf'],
      'application/msword': ['.doc'],
      'application/vnd.openxmlformats-officedocument.wordprocessingml.document': ['.docx'],
    },
  });

  return (
    <div
      {...getRootProps()}
      className={`
        border-2 border-dashed rounded-lg p-8 text-center cursor-pointer
        transition-colors
        ${isDragActive
          ? 'border-blue-500 bg-blue-50'
          : 'border-gray-300 hover:border-gray-400'
        }
        ${loading ? 'opacity-50 cursor-not-allowed' : ''}
      `}
    >
      <input {...getInputProps()} disabled={loading} />

      <svg
        className="mx-auto h-12 w-12 text-gray-400"
        stroke="currentColor"
        fill="none"
        viewBox="0 0 48 48"
      >
        <path
          d="M28 8H12a4 4 0 00-4 4v20m32-12v8m0 0v8a4 4 0 01-4 4H12a4 4 0 01-4-4v-4m32-4l-3.172-3.172a4 4 0 00-5.656 0L28 28M8 32l9.172-9.172a4 4 0 015.656 0L28 28m0 0l4 4m4-24h8m-4-4v8m-12 4h.02"
          strokeWidth={2}
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>

      <div className="mt-4">
        {loading ? (
          <p className="text-sm text-gray-600">Uploading...</p>
        ) : isDragActive ? (
          <p className="text-sm text-blue-600">Drop files here...</p>
        ) : (
          <>
            <p className="text-sm text-gray-600">
              Drag & drop files here, or click to select
            </p>
            <p className="text-xs text-gray-500 mt-1">
              Images, PDFs, and documents up to 10MB
            </p>
          </>
        )}
      </div>
    </div>
  );
}
```

## Progress Tracking

Advanced progress tracking with XHR upload.

```typescript
// src/hooks/useUploadWithProgress.ts
import { useState } from 'react';

interface UploadProgress {
  loaded: number;
  total: number;
  percentage: number;
}

export function useUploadWithProgress() {
  const [progress, setProgress] = useState<UploadProgress | null>(null);
  const [uploading, setUploading] = useState(false);

  const uploadFile = async (
    file: File,
    options: { path?: string; isPublic?: boolean } = {}
  ) => {
    return new Promise((resolve, reject) => {
      const xhr = new XMLHttpRequest();
      const formData = new FormData();

      formData.append('file', file);
      formData.append('path', options.path || '');
      formData.append('isPublic', String(options.isPublic ?? false));

      xhr.upload.addEventListener('progress', (event) => {
        if (event.lengthComputable) {
          setProgress({
            loaded: event.loaded,
            total: event.total,
            percentage: Math.round((event.loaded / event.total) * 100),
          });
        }
      });

      xhr.addEventListener('load', () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          resolve(JSON.parse(xhr.responseText));
        } else {
          reject(new Error(`Upload failed: ${xhr.statusText}`));
        }
        setUploading(false);
        setProgress(null);
      });

      xhr.addEventListener('error', () => {
        reject(new Error('Network error'));
        setUploading(false);
        setProgress(null);
      });

      xhr.addEventListener('abort', () => {
        reject(new Error('Upload cancelled'));
        setUploading(false);
        setProgress(null);
      });

      setUploading(true);
      xhr.open('POST', '/api/upload');
      xhr.send(formData);
    });
  };

  return { uploadFile, progress, uploading };
}
```

### Component

```typescript
// src/components/ProgressUpload.tsx
'use client';

import { useUploadWithProgress } from '@/hooks/useUploadWithProgress';

export default function ProgressUpload() {
  const { uploadFile, progress, uploading } = useUploadWithProgress();

  const handleUpload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    try {
      const result = await uploadFile(file, { isPublic: false });
      console.log('Upload complete:', result);
    } catch (error) {
      console.error('Upload failed:', error);
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
    <div className="space-y-4">
      <input
        type="file"
        onChange={handleUpload}
        disabled={uploading}
        className="block w-full"
      />

      {progress && (
        <div className="space-y-2">
          <div className="flex justify-between text-sm">
            <span>
              {formatBytes(progress.loaded)} / {formatBytes(progress.total)}
            </span>
            <span>{progress.percentage}%</span>
          </div>

          <div className="w-full bg-gray-200 rounded-full h-3">
            <div
              className="bg-blue-600 h-3 rounded-full transition-all flex items-center justify-end pr-2"
              style={{ width: `${progress.percentage}%` }}
            >
              <span className="text-xs text-white font-medium">
                {progress.percentage}%
              </span>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
```

## Image Cropping

Crop images before upload using react-image-crop.

```typescript
// src/components/ImageCropUpload.tsx
'use client';

import { useState, useRef } from 'react';
import ReactCrop, { Crop, PixelCrop } from 'react-image-crop';
import 'react-image-crop/dist/ReactCrop.css';
import { useFileUpload } from '@/hooks/useFiles';

export default function ImageCropUpload({ userId }: { userId: string }) {
  const { upload, loading } = useFileUpload();
  const [src, setSrc] = useState<string | null>(null);
  const [crop, setCrop] = useState<Crop>();
  const [completedCrop, setCompletedCrop] = useState<PixelCrop>();
  const imgRef = useRef<HTMLImageElement>(null);

  const handleFileChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    const reader = new FileReader();
    reader.onload = () => setSrc(reader.result as string);
    reader.readAsDataURL(file);
  };

  const getCroppedImage = async (): Promise<Blob> => {
    if (!completedCrop || !imgRef.current) {
      throw new Error('No crop defined');
    }

    const canvas = document.createElement('canvas');
    const ctx = canvas.getContext('2d');
    if (!ctx) throw new Error('No 2d context');

    const scaleX = imgRef.current.naturalWidth / imgRef.current.width;
    const scaleY = imgRef.current.naturalHeight / imgRef.current.height;

    canvas.width = completedCrop.width;
    canvas.height = completedCrop.height;

    ctx.drawImage(
      imgRef.current,
      completedCrop.x * scaleX,
      completedCrop.y * scaleY,
      completedCrop.width * scaleX,
      completedCrop.height * scaleY,
      0,
      0,
      completedCrop.width,
      completedCrop.height
    );

    return new Promise((resolve) => {
      canvas.toBlob((blob) => {
        if (!blob) throw new Error('Canvas is empty');
        resolve(blob);
      }, 'image/jpeg', 0.95);
    });
  };

  const handleUpload = async () => {
    try {
      const croppedBlob = await getCroppedImage();
      const croppedFile = new File([croppedBlob], 'cropped.jpg', {
        type: 'image/jpeg',
      });

      const result = await upload(croppedFile, {
        path: `avatars/${userId}/`,
        isPublic: true,
      });

      console.log('Uploaded:', result.data.uploadFile);
      setSrc(null);
    } catch (error) {
      console.error('Upload failed:', error);
    }
  };

  return (
    <div className="space-y-4">
      <input type="file" accept="image/*" onChange={handleFileChange} />

      {src && (
        <>
          <ReactCrop
            crop={crop}
            onChange={(c) => setCrop(c)}
            onComplete={(c) => setCompletedCrop(c)}
            aspect={1} // Square crop
          >
            <img ref={imgRef} src={src} alt="Crop preview" />
          </ReactCrop>

          <button
            onClick={handleUpload}
            disabled={!completedCrop || loading}
            className="px-4 py-2 bg-blue-600 text-white rounded disabled:opacity-50"
          >
            {loading ? 'Uploading...' : 'Upload Cropped Image'}
          </button>
        </>
      )}
    </div>
  );
}
```

## File Organization

Organize files in folders with breadcrumb navigation.

```typescript
// src/components/FileManager.tsx
'use client';

import { useUserFiles } from '@/hooks/useFiles';
import { useState } from 'react';

export default function FileManager({ userId }: { userId: string }) {
  const [currentPath, setCurrentPath] = useState<string[]>([]);
  const { files, loading } = useUserFiles(userId);

  // Filter files by current path
  const currentFiles = files.filter(file => {
    const filePath = file.path.split('/');
    filePath.pop(); // Remove filename

    return filePath.join('/') === currentPath.join('/');
  });

  // Get folders in current path
  const folders = [...new Set(
    files
      .filter(file => {
        const filePath = file.path.split('/');
        filePath.pop();
        return filePath.length > currentPath.length &&
               filePath.slice(0, currentPath.length).join('/') === currentPath.join('/');
      })
      .map(file => {
        const filePath = file.path.split('/');
        filePath.pop();
        return filePath[currentPath.length];
      })
  )];

  return (
    <div className="space-y-4">
      {/* Breadcrumb */}
      <nav className="flex items-center space-x-2 text-sm">
        <button
          onClick={() => setCurrentPath([])}
          className="text-blue-600 hover:underline"
        >
          Home
        </button>

        {currentPath.map((folder, idx) => (
          <span key={idx} className="flex items-center space-x-2">
            <span className="text-gray-400">/</span>
            <button
              onClick={() => setCurrentPath(currentPath.slice(0, idx + 1))}
              className="text-blue-600 hover:underline"
            >
              {folder}
            </button>
          </span>
        ))}
      </nav>

      {/* Folders */}
      {folders.length > 0 && (
        <div className="grid grid-cols-4 gap-4">
          {folders.map(folder => (
            <button
              key={folder}
              onClick={() => setCurrentPath([...currentPath, folder])}
              className="flex items-center space-x-2 p-3 border rounded hover:bg-gray-50"
            >
              <svg className="w-5 h-5 text-yellow-500" fill="currentColor" viewBox="0 0 20 20">
                <path d="M2 6a2 2 0 012-2h5l2 2h5a2 2 0 012 2v6a2 2 0 01-2 2H4a2 2 0 01-2-2V6z" />
              </svg>
              <span className="text-sm font-medium">{folder}</span>
            </button>
          ))}
        </div>
      )}

      {/* Files */}
      {loading ? (
        <p>Loading...</p>
      ) : currentFiles.length === 0 ? (
        <p className="text-gray-500">No files in this folder</p>
      ) : (
        <div className="grid grid-cols-4 gap-4">
          {currentFiles.map(file => (
            <div key={file.id} className="border rounded p-3">
              {file.thumbnailUrl && (
                <img
                  src={file.thumbnailUrl}
                  alt={file.name}
                  className="w-full h-32 object-cover rounded mb-2"
                />
              )}
              <p className="text-sm font-medium truncate">{file.name}</p>
              <a
                href={file.url}
                target="_blank"
                rel="noopener noreferrer"
                className="text-xs text-blue-600 hover:underline"
              >
                Download
              </a>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
```

## See Also

- [File Upload Pipeline Guide](file-upload-pipeline.md)
- [File Upload Security](../security/file-upload-security.md)
- [Quick Start Tutorial](../tutorials/file-uploads-quickstart.md)
- [GraphQL API Documentation](../reference/api/README.md)
