#!/usr/bin/env bash
# graphql-integration.sh - Auto-generate GraphQL mutations and queries for file uploads
# Part of nself storage system


# Source required utilities
GRAPHQL_INTEGRATION_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

set -euo pipefail

# Check if display.sh was already sourced from parent
if [[ "${DISPLAY_SOURCED:-0}" != "1" ]]; then
  source "${GRAPHQL_INTEGRATION_DIR}/../utils/display.sh"
fi

# Compatibility aliases for output functions
output_info() { log_info "$@"; }
output_success() { log_success "$@"; }
output_error() { log_error "$@"; }
output_warning() { log_warning "$@"; }

# GraphQL schema templates
readonly GRAPHQL_FILE_TYPE='
type File {
  id: uuid!
  name: String!
  size: Int!
  mimeType: String!
  path: String!
  url: String!
  thumbnailUrl: String
  userId: uuid!
  createdAt: timestamptz!
  updatedAt: timestamptz!
  metadata: jsonb
  tags: [String!]
  user: users!
  isPublic: Boolean!
}'

readonly GRAPHQL_MUTATIONS='
# Upload a single file
mutation UploadFile($file: Upload!, $path: String, $isPublic: Boolean) {
  uploadFile(file: $file, path: $path, isPublic: $isPublic) {
    id
    name
    size
    mimeType
    path
    url
    thumbnailUrl
    createdAt
  }
}

# Upload multiple files
mutation UploadFiles($files: [Upload!]!, $path: String, $isPublic: Boolean) {
  uploadFiles(files: $files, path: $path, isPublic: $isPublic) {
    id
    name
    size
    mimeType
    path
    url
    thumbnailUrl
    createdAt
  }
}

# Delete a file
mutation DeleteFile($id: uuid!) {
  delete_files_by_pk(id: $id) {
    id
  }
}

# Update file metadata
mutation UpdateFileMetadata($id: uuid!, $metadata: jsonb, $tags: [String!]) {
  update_files_by_pk(
    pk_columns: { id: $id }
    _set: { metadata: $metadata, tags: $tags }
  ) {
    id
    metadata
    tags
  }
}'

readonly GRAPHQL_QUERIES='
# Get file by ID
query GetFile($id: uuid!) {
  files_by_pk(id: $id) {
    id
    name
    size
    mimeType
    path
    url
    thumbnailUrl
    userId
    createdAt
    updatedAt
    metadata
    tags
    isPublic
    user {
      id
      displayName
      avatarUrl
    }
  }
}

# List user files
query ListUserFiles($userId: uuid!, $limit: Int = 50, $offset: Int = 0) {
  files(
    where: { userId: { _eq: $userId } }
    order_by: { createdAt: desc }
    limit: $limit
    offset: $offset
  ) {
    id
    name
    size
    mimeType
    path
    url
    thumbnailUrl
    createdAt
    tags
  }
  files_aggregate(where: { userId: { _eq: $userId } }) {
    aggregate {
      count
      sum {
        size
      }
    }
  }
}

# Search files by name or tags
query SearchFiles($search: String!, $userId: uuid!) {
  files(
    where: {
      _and: [
        { userId: { _eq: $userId } }
        {
          _or: [
            { name: { _ilike: $search } }
            { tags: { _has_key: $search } }
          ]
        }
      ]
    }
    order_by: { createdAt: desc }
  ) {
    id
    name
    size
    mimeType
    path
    url
    thumbnailUrl
    createdAt
    tags
  }
}'

readonly GRAPHQL_SUBSCRIPTIONS='
# Subscribe to new file uploads
subscription OnFileUploaded($userId: uuid!) {
  files(
    where: { userId: { _eq: $userId } }
    order_by: { createdAt: desc }
    limit: 1
  ) {
    id
    name
    size
    mimeType
    path
    url
    thumbnailUrl
    createdAt
  }
}'

#######################################
# Generate Hasura migration for files table
# Returns:
#   SQL migration content
#######################################
generate_files_migration() {
  cat <<'EOF'
-- Create files table
CREATE TABLE IF NOT EXISTS public.files (
  id uuid DEFAULT gen_random_uuid() NOT NULL PRIMARY KEY,
  name text NOT NULL,
  size integer NOT NULL,
  mime_type text NOT NULL,
  path text NOT NULL UNIQUE,
  url text NOT NULL,
  thumbnail_url text,
  user_id uuid NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
  created_at timestamptz DEFAULT now() NOT NULL,
  updated_at timestamptz DEFAULT now() NOT NULL,
  metadata jsonb DEFAULT '{}'::jsonb,
  tags text[] DEFAULT ARRAY[]::text[],
  is_public boolean DEFAULT false NOT NULL
);

-- Create indexes
CREATE INDEX IF NOT EXISTS files_user_id_idx ON public.files(user_id);
CREATE INDEX IF NOT EXISTS files_created_at_idx ON public.files(created_at DESC);
CREATE INDEX IF NOT EXISTS files_mime_type_idx ON public.files(mime_type);
CREATE INDEX IF NOT EXISTS files_tags_idx ON public.files USING gin(tags);
CREATE INDEX IF NOT EXISTS files_metadata_idx ON public.files USING gin(metadata);

-- Create updated_at trigger
CREATE OR REPLACE FUNCTION public.set_current_timestamp_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS set_files_updated_at ON public.files;
CREATE TRIGGER set_files_updated_at
BEFORE UPDATE ON public.files
FOR EACH ROW
EXECUTE FUNCTION public.set_current_timestamp_updated_at();

-- Set up RLS (Row Level Security)
ALTER TABLE public.files ENABLE ROW LEVEL SECURITY;

-- Users can view their own files
CREATE POLICY files_select_own
ON public.files
FOR SELECT
USING (
  auth.uid() = user_id
  OR is_public = true
);

-- Users can insert their own files
CREATE POLICY files_insert_own
ON public.files
FOR INSERT
WITH CHECK (auth.uid() = user_id);

-- Users can update their own files
CREATE POLICY files_update_own
ON public.files
FOR UPDATE
USING (auth.uid() = user_id)
WITH CHECK (auth.uid() = user_id);

-- Users can delete their own files
CREATE POLICY files_delete_own
ON public.files
FOR DELETE
USING (auth.uid() = user_id);

-- Grant permissions
GRANT SELECT, INSERT, UPDATE, DELETE ON public.files TO authenticated;
GRANT SELECT ON public.files TO anonymous;

COMMENT ON TABLE public.files IS 'Uploaded files metadata';
COMMENT ON COLUMN public.files.metadata IS 'Additional file metadata (EXIF, dimensions, etc.)';
COMMENT ON COLUMN public.files.tags IS 'User-defined tags for file organization';
COMMENT ON COLUMN public.files.is_public IS 'Whether file is publicly accessible without authentication';
EOF
}

#######################################
# Generate Hasura metadata for files table
# Returns:
#   YAML metadata content
#######################################
generate_hasura_metadata() {
  cat <<'EOF'
table:
  schema: public
  name: files
object_relationships:
  - name: user
    using:
      foreign_key_constraint_on: user_id
insert_permissions:
  - role: user
    permission:
      check:
        user_id:
          _eq: X-Hasura-User-Id
      columns:
        - name
        - size
        - mime_type
        - path
        - url
        - thumbnail_url
        - metadata
        - tags
        - is_public
select_permissions:
  - role: user
    permission:
      columns:
        - id
        - name
        - size
        - mime_type
        - path
        - url
        - thumbnail_url
        - user_id
        - created_at
        - updated_at
        - metadata
        - tags
        - is_public
      filter:
        _or:
          - user_id:
              _eq: X-Hasura-User-Id
          - is_public:
              _eq: true
      allow_aggregations: true
update_permissions:
  - role: user
    permission:
      columns:
        - name
        - metadata
        - tags
        - is_public
      filter:
        user_id:
          _eq: X-Hasura-User-Id
      check:
        user_id:
          _eq: X-Hasura-User-Id
delete_permissions:
  - role: user
    permission:
      filter:
        user_id:
          _eq: X-Hasura-User-Id
EOF
}

#######################################
# Generate TypeScript types for frontend
# Returns:
#   TypeScript type definitions
#######################################
generate_typescript_types() {
  cat <<'EOF'
// Auto-generated file upload types
// Generated by nself storage graphql-integration

export interface File {
  id: string;
  name: string;
  size: number;
  mimeType: string;
  path: string;
  url: string;
  thumbnailUrl?: string;
  userId: string;
  createdAt: string;
  updatedAt: string;
  metadata?: Record<string, any>;
  tags?: string[];
  isPublic: boolean;
  user?: {
    id: string;
    displayName?: string;
    avatarUrl?: string;
  };
}

export interface UploadFileInput {
  file: File;
  path?: string;
  isPublic?: boolean;
}

export interface UploadFilesInput {
  files: File[];
  path?: string;
  isPublic?: boolean;
}

export interface FileFilter {
  userId?: string;
  mimeType?: string;
  tags?: string[];
  search?: string;
}

export interface FileListResult {
  files: File[];
  total: number;
  totalSize: number;
}
EOF
}

#######################################
# Generate React hooks for file uploads
# Returns:
#   React hooks code
#######################################
generate_react_hooks() {
  cat <<'EOF'
// Auto-generated React hooks for file uploads
// Generated by nself storage graphql-integration

import { useMutation, useQuery, useSubscription } from '@apollo/client';
import { gql } from '@apollo/client';

// Mutations
const UPLOAD_FILE = gql`
  mutation UploadFile($file: Upload!, $path: String, $isPublic: Boolean) {
    uploadFile(file: $file, path: $path, isPublic: $isPublic) {
      id
      name
      size
      mimeType
      path
      url
      thumbnailUrl
      createdAt
    }
  }
`;

const UPLOAD_FILES = gql`
  mutation UploadFiles($files: [Upload!]!, $path: String, $isPublic: Boolean) {
    uploadFiles(files: $files, path: $path, isPublic: $isPublic) {
      id
      name
      size
      mimeType
      path
      url
      thumbnailUrl
      createdAt
    }
  }
`;

const DELETE_FILE = gql`
  mutation DeleteFile($id: uuid!) {
    delete_files_by_pk(id: $id) {
      id
    }
  }
`;

// Queries
const GET_FILE = gql`
  query GetFile($id: uuid!) {
    files_by_pk(id: $id) {
      id
      name
      size
      mimeType
      path
      url
      thumbnailUrl
      userId
      createdAt
      updatedAt
      metadata
      tags
      isPublic
      user {
        id
        displayName
        avatarUrl
      }
    }
  }
`;

const LIST_USER_FILES = gql`
  query ListUserFiles($userId: uuid!, $limit: Int = 50, $offset: Int = 0) {
    files(
      where: { userId: { _eq: $userId } }
      order_by: { createdAt: desc }
      limit: $limit
      offset: $offset
    ) {
      id
      name
      size
      mimeType
      path
      url
      thumbnailUrl
      createdAt
      tags
    }
    files_aggregate(where: { userId: { _eq: $userId } }) {
      aggregate {
        count
        sum {
          size
        }
      }
    }
  }
`;

// Hooks
export function useFileUpload() {
  const [uploadFile, { data, loading, error }] = useMutation(UPLOAD_FILE);

  const upload = async (file: File, options?: { path?: string; isPublic?: boolean }) => {
    return uploadFile({
      variables: {
        file,
        path: options?.path,
        isPublic: options?.isPublic ?? false,
      },
    });
  };

  return { upload, data, loading, error };
}

export function useMultipleFileUpload() {
  const [uploadFiles, { data, loading, error }] = useMutation(UPLOAD_FILES);

  const upload = async (files: File[], options?: { path?: string; isPublic?: boolean }) => {
    return uploadFiles({
      variables: {
        files,
        path: options?.path,
        isPublic: options?.isPublic ?? false,
      },
    });
  };

  return { upload, data, loading, error };
}

export function useFileDelete() {
  const [deleteFile, { data, loading, error }] = useMutation(DELETE_FILE);

  const remove = async (fileId: string) => {
    return deleteFile({
      variables: { id: fileId },
    });
  };

  return { remove, data, loading, error };
}

export function useFile(fileId: string) {
  const { data, loading, error, refetch } = useQuery(GET_FILE, {
    variables: { id: fileId },
    skip: !fileId,
  });

  return {
    file: data?.files_by_pk,
    loading,
    error,
    refetch,
  };
}

export function useUserFiles(userId: string, options?: { limit?: number; offset?: number }) {
  const { data, loading, error, refetch, fetchMore } = useQuery(LIST_USER_FILES, {
    variables: {
      userId,
      limit: options?.limit ?? 50,
      offset: options?.offset ?? 0,
    },
    skip: !userId,
  });

  return {
    files: data?.files ?? [],
    total: data?.files_aggregate?.aggregate?.count ?? 0,
    totalSize: data?.files_aggregate?.aggregate?.sum?.size ?? 0,
    loading,
    error,
    refetch,
    fetchMore,
  };
}
EOF
}

#######################################
# Generate complete GraphQL integration package
# Arguments:
#   $1 - Output directory (optional, defaults to current directory)
# Returns:
#   0 on success
#######################################
generate_graphql_package() {
  local output_dir="${1:-.}"

  output_info "Generating GraphQL integration package..."

  # Create directory structure
  mkdir -p "${output_dir}/migrations"
  mkdir -p "${output_dir}/metadata/tables"
  mkdir -p "${output_dir}/graphql"
  mkdir -p "${output_dir}/types"
  mkdir -p "${output_dir}/hooks"

  # Generate migration
  local timestamp
  timestamp="$(date +%Y%m%d%H%M%S)"
  generate_files_migration >"${output_dir}/migrations/${timestamp}_create_files_table.sql"
  output_success "Created migration: ${timestamp}_create_files_table.sql"

  # Generate Hasura metadata
  generate_hasura_metadata >"${output_dir}/metadata/tables/public_files.yaml"
  output_success "Created metadata: public_files.yaml"

  # Generate GraphQL operations
  cat >"${output_dir}/graphql/files.graphql" <<EOF
${GRAPHQL_FILE_TYPE}

# Mutations
${GRAPHQL_MUTATIONS}

# Queries
${GRAPHQL_QUERIES}

# Subscriptions
${GRAPHQL_SUBSCRIPTIONS}
EOF
  output_success "Created GraphQL operations: files.graphql"

  # Generate TypeScript types
  generate_typescript_types >"${output_dir}/types/files.ts"
  output_success "Created TypeScript types: files.ts"

  # Generate React hooks
  generate_react_hooks >"${output_dir}/hooks/useFiles.ts"
  output_success "Created React hooks: useFiles.ts"

  # Generate README
  cat >"${output_dir}/README.md" <<'EOF'
# nself File Upload Integration

Auto-generated GraphQL integration for file uploads.

## Installation

1. Run the migration:
```bash
psql $DATABASE_URL < migrations/*_create_files_table.sql
```

2. Apply Hasura metadata:
```bash
hasura metadata apply
```

3. Copy types and hooks to your frontend:
```bash
cp types/files.ts src/types/
cp hooks/useFiles.ts src/hooks/
```

## Usage

### Upload a file

```typescript
import { useFileUpload } from '@/hooks/useFiles';

function UploadButton() {
  const { upload, loading } = useFileUpload();

  const handleUpload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (file) {
      const result = await upload(file, { isPublic: false });
      console.log('Uploaded:', result.data.uploadFile);
    }
  };

  return (
    <input type="file" onChange={handleUpload} disabled={loading} />
  );
}
```

### List user files

```typescript
import { useUserFiles } from '@/hooks/useFiles';

function FileList({ userId }: { userId: string }) {
  const { files, total, totalSize, loading } = useUserFiles(userId);

  if (loading) return <div>Loading...</div>;

  return (
    <div>
      <h2>{total} files ({formatBytes(totalSize)})</h2>
      <ul>
        {files.map(file => (
          <li key={file.id}>
            {file.thumbnailUrl && <img src={file.thumbnailUrl} alt={file.name} />}
            <a href={file.url}>{file.name}</a>
          </li>
        ))}
      </ul>
    </div>
  );
}
```

## GraphQL Operations

See `graphql/files.graphql` for all available mutations, queries, and subscriptions.

## Database Schema

The `files` table includes:
- File metadata (name, size, MIME type)
- Storage paths and URLs
- Optional thumbnail URL
- User ownership (with RLS)
- Custom metadata (JSONB)
- Tags for organization
- Public/private flag

## Permissions

Row Level Security (RLS) is enabled:
- Users can only see their own files (unless public)
- Users can upload, update, and delete their own files
- Public files are viewable by anyone
EOF
  output_success "Created README.md"

  printf "\nGraphQL integration package generated in: %s\n" "${output_dir}"
  printf "\nNext steps:\n"
  printf "  1. Review generated files\n"
  printf "  2. Run migration: psql \$DATABASE_URL < migrations/*_create_files_table.sql\n"
  printf "  3. Apply Hasura metadata: hasura metadata apply\n"
  printf "  4. Copy types and hooks to your frontend\n\n"

  return 0
}

# Export functions
export -f generate_files_migration
export -f generate_hasura_metadata
export -f generate_typescript_types
export -f generate_react_hooks
export -f generate_graphql_package
