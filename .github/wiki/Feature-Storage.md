# Feature: Storage

ɳSelf uses MinIO as its S3-compatible object storage layer, available as an optional service.

## What's Included

| Capability | Description |
|-----------|-------------|
| S3-compatible API | Drop-in replacement for AWS S3 |
| Buckets | Create and manage storage buckets |
| Access policies | Public, private, and custom bucket policies |
| Presigned URLs | Temporary access URLs for uploads/downloads |
| Console UI | Web UI for bucket management |

## How to Enable

```bash
nself service enable minio
nself build && nself restart
```

Or set in `.env`:
```env
MINIO_ENABLED=true
```

## Key Configuration Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `MINIO_ENABLED` | Enable MinIO service | `false` |
| `MINIO_ACCESS_KEY` | Root access key | Auto-generated |
| `MINIO_SECRET_ACCESS_KEY` | Root secret key | Auto-generated |
| `MINIO_DEFAULT_BUCKET` | Default bucket name | `nself` |
| `MINIO_PORT` | Internal port | `9000` |

## Connecting to MinIO

Use the S3 endpoint URL from `nself urls`. Configure your application with:

```
Endpoint: https://storage.your-domain.com
Access Key: ${MINIO_ACCESS_KEY}
Secret Key: ${MINIO_SECRET_ACCESS_KEY}
Region: us-east-1  # MinIO ignores region but requires a value
```

## Pro Storage Plugin

For multi-provider S3 storage (AWS S3, Cloudflare R2, Backblaze B2), see the **object-storage** licensed plugin.

## See Also

- [[cmd-service]], enable/disable storage
- [[Config-Env-Vars]], full env var reference

---
← [[Home]] | [[_Sidebar]]
