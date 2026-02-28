# File Upload Security Best Practices

Comprehensive security guide for implementing file uploads in nself applications.

## Table of Contents

1. [Security Overview](#security-overview)
2. [Input Validation](#input-validation)
3. [File Type Restrictions](#file-type-restrictions)
4. [Virus Scanning](#virus-scanning)
5. [Storage Security](#storage-security)
6. [Access Control](#access-control)
7. [Rate Limiting](#rate-limiting)
8. [Content Security](#content-security)
9. [Production Checklist](#production-checklist)

## Security Overview

File uploads are a common attack vector. Follow these practices to secure your application:

### Common Attack Vectors

1. **Malicious File Upload** - Uploading executable files or malware
2. **Path Traversal** - Using `../` to escape storage directory
3. **File Type Spoofing** - Fake MIME types or file extensions
4. **Storage Exhaustion** - Uploading massive files to fill disk
5. **XML Bomb** - Specially crafted files that expand when processed
6. **SSRF (Server-Side Request Forgery)** - Uploading files that trigger internal requests

### Defense in Depth

nself implements multiple security layers:

```
Layer 1: Client-side validation (UX, not security)
    ↓
Layer 2: File type & size validation (frontend + backend)
    ↓
Layer 3: Virus scanning (ClamAV)
    ↓
Layer 4: Storage isolation (MinIO)
    ↓
Layer 5: Access control (Hasura RLS)
    ↓
Layer 6: Content Security Policy (Nginx headers)
```

## Input Validation

### 1. File Size Limits

**Always enforce on both client and server:**

```typescript
// Frontend (Next.js)
const MAX_FILE_SIZE = 10 * 1024 * 1024; // 10MB

function validateFileSize(file: File): boolean {
  if (file.size > MAX_FILE_SIZE) {
    throw new Error(`File too large. Max size: ${MAX_FILE_SIZE / 1024 / 1024}MB`);
  }
  return true;
}
```

```bash
# Backend (.env.prod)
UPLOAD_MAX_FILE_SIZE=10485760  # 10MB
```

```sql
-- Database constraint
ALTER TABLE public.files
ADD CONSTRAINT files_size_check
CHECK (size <= 10485760);
```

### 2. File Name Sanitization

**Strip dangerous characters:**

```typescript
function sanitizeFileName(fileName: string): string {
  return fileName
    .replace(/[^a-zA-Z0-9._-]/g, '_')  // Remove special chars
    .replace(/\.{2,}/g, '.')            // No multiple dots
    .replace(/^\./, '')                 // No leading dot
    .substring(0, 255);                 // Max 255 chars
}
```

**Never trust user-provided file names:**

```typescript
// Generate secure file names
const secureFileName = `${uuid()}_${sanitizeFileName(file.name)}`;
```

### 3. Path Traversal Prevention

**Never use user input in file paths:**

```typescript
// ❌ DANGEROUS
const path = `uploads/${userInput}`;

// ✅ SAFE
const path = `uploads/${userId}/${uuid()}_${sanitizedName}`;
```

**Server-side validation:**

```bash
# upload-pipeline.sh validates paths
if [[ "${dest_path}" =~ \.\. ]]; then
  output_error "Invalid path: contains .."
  return 1
fi
```

## File Type Restrictions

### 1. Allowlist (Recommended)

**Only allow specific file types:**

```typescript
const ALLOWED_TYPES = {
  images: ['image/jpeg', 'image/png', 'image/gif', 'image/webp'],
  documents: ['application/pdf', 'application/msword'],
  videos: ['video/mp4', 'video/webm'],
};

function validateFileType(file: File, category: keyof typeof ALLOWED_TYPES): boolean {
  const allowed = ALLOWED_TYPES[category];

  if (!allowed.includes(file.type)) {
    throw new Error(`Invalid file type. Allowed: ${allowed.join(', ')}`);
  }

  return true;
}
```

### 2. MIME Type Validation

**Never trust client-provided MIME type:**

```bash
# Server validates actual file content
mime_type="$(file --mime-type -b "${file_path}")"

# Check against allowlist
case "${mime_type}" in
  image/jpeg|image/png|image/gif)
    # Valid image
    ;;
  *)
    output_error "Invalid file type: ${mime_type}"
    return 1
    ;;
esac
```

### 3. Magic Number Validation

**Verify file signature (first bytes):**

```typescript
async function validateImageSignature(file: File): Promise<boolean> {
  const buffer = await file.slice(0, 4).arrayBuffer();
  const bytes = new Uint8Array(buffer);

  // JPEG: FF D8 FF
  if (bytes[0] === 0xFF && bytes[1] === 0xD8 && bytes[2] === 0xFF) {
    return true;
  }

  // PNG: 89 50 4E 47
  if (bytes[0] === 0x89 && bytes[1] === 0x50 && bytes[2] === 0x4E && bytes[3] === 0x47) {
    return true;
  }

  throw new Error('Invalid image signature');
}
```

### 4. Dangerous File Types

**Never allow these extensions:**

```typescript
const DANGEROUS_EXTENSIONS = [
  '.exe', '.dll', '.bat', '.cmd', '.sh',
  '.php', '.asp', '.jsp', '.js', '.html',
  '.svg',  // Can contain scripts
  '.jar', '.app', '.dmg',
];

function checkDangerousExtension(fileName: string): boolean {
  const ext = fileName.toLowerCase().substring(fileName.lastIndexOf('.'));

  if (DANGEROUS_EXTENSIONS.includes(ext)) {
    throw new Error(`Dangerous file type: ${ext}`);
  }

  return true;
}
```

## Virus Scanning

### 1. Enable ClamAV

```bash
# .env.prod
UPLOAD_ENABLE_VIRUS_SCAN=true

# Install ClamAV
sudo apt-get install clamav clamav-daemon

# Update virus definitions daily
sudo freshclam
```

### 2. Scan Before Processing

```bash
# upload-pipeline.sh automatically scans when enabled
if [[ "${ENABLE_VIRUS_SCAN}" == "true" ]]; then
  if ! scan_file_for_viruses "${file_path}"; then
    output_error "Virus detected! Upload aborted."
    return 1
  fi
fi
```

### 3. Quarantine Infected Files

```bash
# Move infected files to quarantine
QUARANTINE_DIR="/var/quarantine"

if clamscan "${file_path}" | grep -q "FOUND"; then
  mkdir -p "${QUARANTINE_DIR}"
  mv "${file_path}" "${QUARANTINE_DIR}/$(date +%s)_$(basename "${file_path}")"
  log_security_event "virus_detected" "${file_path}"
fi
```

### 4. Auto-Update Virus Definitions

```bash
# cron job: /etc/cron.daily/freshclam
#!/bin/bash
freshclam --quiet
systemctl reload clamav-daemon
```

## Storage Security

### 1. Separate Storage Domain

**Never serve uploads from main domain:**

```nginx
# ❌ DANGEROUS
https://yourdomain.com/uploads/user-file.jpg

# ✅ SAFE
https://cdn.yourdomain.com/uploads/user-file.jpg
```

**Prevents:**
- Cookie theft via XSS in uploaded files
- Same-origin policy bypass
- CSRF attacks

### 2. Content-Disposition Header

**Force download for dangerous types:**

```nginx
# nginx config
location /uploads/ {
  # Force download for non-images
  if ($request_uri ~ \.(pdf|doc|zip)$) {
    add_header Content-Disposition "attachment; filename=$1";
  }

  # Images can be inline
  if ($request_uri ~ \.(jpg|jpeg|png|gif|webp)$) {
    add_header Content-Disposition "inline";
  }
}
```

### 3. Disable Script Execution

```nginx
# nginx config for storage domain
location /uploads/ {
  # Disable PHP, Python, etc.
  location ~ \.(php|py|rb|pl|sh)$ {
    deny all;
  }

  # No code execution
  add_header X-Content-Type-Options "nosniff" always;
  add_header Content-Security-Policy "default-src 'none'; img-src 'self'; style-src 'none'; script-src 'none';" always;
}
```

### 4. Private Buckets

```bash
# MinIO configuration
# Bucket should NOT be public by default

# Use signed URLs for temporary access
mc policy set download nself/uploads-private  # No public access

# Generate time-limited URLs
mc share download --expire 1h nself/uploads-private/file.pdf
```

## Access Control

### 1. Row Level Security (RLS)

**Enforce ownership at database level:**

```sql
-- Users can only see their own files
CREATE POLICY files_select_own
ON public.files
FOR SELECT
USING (
  auth.uid() = user_id
  OR is_public = true
);

-- Users can only upload to their own account
CREATE POLICY files_insert_own
ON public.files
FOR INSERT
WITH CHECK (auth.uid() = user_id);
```

### 2. Hasura Permissions

**Double-layer protection:**

```yaml
# metadata/tables/public_files.yaml
select_permissions:
  - role: user
    permission:
      filter:
        _or:
          - user_id: { _eq: X-Hasura-User-Id }
          - is_public: { _eq: true }
      columns:
        - id
        - name
        - url
        # Don't expose sensitive columns to others
```

### 3. Signed URLs for Private Files

```typescript
import { signUrl } from '@/lib/storage';

// Generate time-limited URL
const signedUrl = await signUrl(file.path, {
  expiresIn: 3600,  // 1 hour
  userId: currentUser.id,
});

// URL expires after 1 hour
return { url: signedUrl };
```

### 4. IP-Based Restrictions

```nginx
# nginx config - restrict upload endpoint
location /api/upload {
  # Only allow from trusted IPs
  allow 10.0.0.0/8;      # Internal network
  allow 192.168.1.0/24;  # Office network
  deny all;

  proxy_pass http://api:3000;
}
```

## Rate Limiting

### 1. Upload Rate Limits

**Prevent abuse:**

```typescript
// Use Redis for rate limiting
import { rateLimit } from '@/lib/redis';

async function uploadHandler(req, res) {
  const userId = req.user.id;

  // Limit: 10 uploads per hour per user
  const allowed = await rateLimit(`upload:${userId}`, {
    max: 10,
    window: 3600,
  });

  if (!allowed) {
    return res.status(429).json({
      error: 'Too many uploads. Try again later.',
    });
  }

  // Process upload...
}
```

### 2. Bandwidth Throttling

```nginx
# nginx config
limit_req_zone $binary_remote_addr zone=upload:10m rate=1r/s;

location /api/upload {
  limit_req zone=upload burst=5 nodelay;
  client_max_body_size 10M;
  proxy_pass http://api:3000;
}
```

### 3. Storage Quotas

**Per-user storage limits:**

```sql
-- Add quota column
ALTER TABLE auth.users
ADD COLUMN storage_quota_bytes bigint DEFAULT 1073741824;  -- 1GB

-- Trigger to check quota before upload
CREATE OR REPLACE FUNCTION check_storage_quota()
RETURNS TRIGGER AS $$
DECLARE
  current_usage bigint;
  user_quota bigint;
BEGIN
  -- Calculate current usage
  SELECT COALESCE(SUM(size), 0)
  INTO current_usage
  FROM public.files
  WHERE user_id = NEW.user_id;

  -- Get user quota
  SELECT storage_quota_bytes
  INTO user_quota
  FROM auth.users
  WHERE id = NEW.user_id;

  -- Check if over quota
  IF (current_usage + NEW.size) > user_quota THEN
    RAISE EXCEPTION 'Storage quota exceeded. Current: %, Quota: %',
      current_usage, user_quota;
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER enforce_storage_quota
BEFORE INSERT ON public.files
FOR EACH ROW
EXECUTE FUNCTION check_storage_quota();
```

## Content Security

### 1. Content Security Policy (CSP)

```nginx
# nginx config for storage domain
add_header Content-Security-Policy "
  default-src 'none';
  img-src 'self';
  media-src 'self';
  style-src 'none';
  script-src 'none';
  object-src 'none';
  frame-ancestors 'none';
" always;
```

### 2. Image Sanitization

**Strip EXIF data (may contain GPS, camera info):**

```bash
# Use ImageMagick to strip metadata
convert uploaded.jpg -strip sanitized.jpg
```

```typescript
// Frontend: Remove EXIF before upload
import piexif from 'piexifjs';

function stripExif(file: File): Promise<File> {
  return new Promise((resolve) => {
    const reader = new FileReader();
    reader.onload = (e) => {
      const data = e.target?.result as string;
      const stripped = piexif.remove(data);
      const blob = dataURLtoBlob(stripped);
      resolve(new File([blob], file.name, { type: file.type }));
    };
    reader.readAsDataURL(file);
  });
}
```

### 3. PDF Security

**Disable JavaScript in PDFs:**

```bash
# Use pdftk to sanitize PDFs
pdftk uploaded.pdf output sanitized.pdf flatten

# Or use Ghostscript
gs -dSAFER -dNOPAUSE -dBATCH \
   -sDEVICE=pdfwrite \
   -sOutputFile=sanitized.pdf \
   uploaded.pdf
```

### 4. SVG Sanitization

**Never allow SVG uploads (contains scripts):**

```typescript
// If you MUST allow SVGs, sanitize them
import { sanitize } from 'dompurify';

function sanitizeSVG(svgContent: string): string {
  return sanitize(svgContent, {
    USE_PROFILES: { svg: true },
    ALLOWED_TAGS: ['svg', 'path', 'circle', 'rect', 'line', 'polyline', 'polygon'],
    ALLOWED_ATTR: ['width', 'height', 'viewBox', 'd', 'fill', 'stroke'],
  });
}
```

## Production Checklist

Before deploying to production:

### Required Security Measures

- [ ] **File type allowlist** implemented (frontend + backend)
- [ ] **File size limits** enforced (client + server + database)
- [ ] **Virus scanning** enabled (ClamAV with daily updates)
- [ ] **MIME type validation** on server (not client value)
- [ ] **File name sanitization** (remove special characters)
- [ ] **Path traversal prevention** (no `../` in paths)
- [ ] **Row Level Security** enabled on files table
- [ ] **Hasura permissions** configured correctly
- [ ] **Separate storage domain** (not main app domain)
- [ ] **Content-Disposition** headers set
- [ ] **Content Security Policy** headers set
- [ ] **Script execution disabled** in storage directory

### Recommended Security Measures

- [ ] **Rate limiting** on upload endpoints
- [ ] **Storage quotas** per user
- [ ] **Signed URLs** for private files
- [ ] **EXIF data stripping** for images
- [ ] **PDF sanitization** if allowing PDFs
- [ ] **IP restrictions** on admin uploads
- [ ] **CDN with WAF** (CloudFlare, AWS WAF)
- [ ] **Audit logging** for all uploads
- [ ] **Backup strategy** for uploaded files

### Monitoring

- [ ] **Alert on large uploads** (> 100MB)
- [ ] **Alert on virus detection**
- [ ] **Alert on quota exceeded**
- [ ] **Alert on unusual upload patterns**
- [ ] **Dashboard for storage usage**
- [ ] **Daily reports of file types uploaded**

### Testing

- [ ] **Upload malicious file** (should be blocked)
- [ ] **Upload with `../` in path** (should be sanitized)
- [ ] **Upload fake MIME type** (should detect real type)
- [ ] **Upload > size limit** (should be rejected)
- [ ] **Upload to other user's folder** (should fail)
- [ ] **Access other user's file** (should fail)
- [ ] **Upload with XSS in filename** (should be sanitized)

## Example: Secure Upload Flow

```typescript
// Complete secure upload implementation
async function secureFileUpload(file: File, userId: string) {
  // 1. Client-side validation
  validateFileSize(file);
  validateFileType(file, 'images');
  await validateImageSignature(file);
  checkDangerousExtension(file.name);

  // 2. Sanitize file name
  const safeName = sanitizeFileName(file.name);

  // 3. Strip EXIF data
  const strippedFile = await stripExif(file);

  // 4. Check rate limit
  const allowed = await rateLimit(`upload:${userId}`, {
    max: 10,
    window: 3600,
  });

  if (!allowed) {
    throw new Error('Rate limit exceeded');
  }

  // 5. Check storage quota
  const quota = await checkStorageQuota(userId, file.size);
  if (!quota.allowed) {
    throw new Error('Storage quota exceeded');
  }

  // 6. Upload with virus scan
  const result = await uploadFile(strippedFile, {
    path: `users/${userId}/${uuid()}_${safeName}`,
    virusScan: true,
    compression: true,
  });

  // 7. Save metadata with RLS
  const fileRecord = await db.files.insert({
    user_id: userId,
    name: safeName,
    size: file.size,
    mime_type: result.mimeType,
    path: result.path,
    url: result.url,
    is_public: false,
  });

  // 8. Audit log
  await logAuditEvent({
    action: 'file_uploaded',
    user_id: userId,
    file_id: fileRecord.id,
    file_size: file.size,
    ip: request.ip,
  });

  return fileRecord;
}
```

## Resources

- [OWASP File Upload Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/File_Upload_Cheat_Sheet.html)
- [CWE-434: Unrestricted Upload of File](https://cwe.mitre.org/data/definitions/434.html)
- [Content Security Policy Reference](https://content-security-policy.com/)
- [ClamAV Documentation](https://www.clamav.net/documents)

## Support

For security concerns:
- [Security Policy](https://github.com/nself-org/cli/security/policy)
- Email: security@nself.org
