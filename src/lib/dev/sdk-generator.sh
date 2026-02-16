#!/usr/bin/env bash
# sdk-generator.sh - Generate SDKs from GraphQL schema
# Part of nself v0.7.0 - Sprint 19: Developer Experience Tools


# Get Hasura GraphQL endpoint
get_hasura_endpoint() {

set -euo pipefail

  local project_name="${PROJECT_NAME:-nself}"
  local base_domain="${BASE_DOMAIN:-localhost}"

  if [[ "$base_domain" == "localhost" ]]; then
    printf "http://localhost:8080"
  else
    printf "https://api.%s" "$base_domain"
  fi
}

# Fetch GraphQL schema from Hasura
fetch_graphql_schema() {
  local endpoint="$1"
  local admin_secret="${HASURA_GRAPHQL_ADMIN_SECRET:-}"
  local output_file="${2:-schema.graphql}"

  printf "Fetching GraphQL schema from %s...\n" "$endpoint"

  local introspection_query='
  query IntrospectionQuery {
    __schema {
      queryType { name }
      mutationType { name }
      subscriptionType { name }
      types {
        ...FullType
      }
      directives {
        name
        description
        locations
        args {
          ...InputValue
        }
      }
    }
  }

  fragment FullType on __Type {
    kind
    name
    description
    fields(includeDeprecated: true) {
      name
      description
      args {
        ...InputValue
      }
      type {
        ...TypeRef
      }
      isDeprecated
      deprecationReason
    }
    inputFields {
      ...InputValue
    }
    interfaces {
      ...TypeRef
    }
    enumValues(includeDeprecated: true) {
      name
      description
      isDeprecated
      deprecationReason
    }
    possibleTypes {
      ...TypeRef
    }
  }

  fragment InputValue on __InputValue {
    name
    description
    type { ...TypeRef }
    defaultValue
  }

  fragment TypeRef on __Type {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
                ofType {
                  kind
                  name
                }
              }
            }
          }
        }
      }
    }
  }'

  local headers=(-H "Content-Type: application/json")
  if [[ -n "$admin_secret" ]]; then
    headers+=(-H "x-hasura-admin-secret: $admin_secret")
  fi

  local response
  response=$(curl -s "${headers[@]}" \
    -X POST \
    -d "$(printf '{"query":"%s"}' "$(printf '%s' "$introspection_query" | sed 's/"/\\"/g' | tr -d '\n')")" \
    "$endpoint/v1/graphql")

  if printf '%s' "$response" | grep -q "error"; then
    printf "Error fetching schema: %s\n" "$response" >&2
    return 1
  fi

  printf '%s' "$response" >"$output_file"
  printf "Schema saved to %s\n" "$output_file"
}

# Generate TypeScript SDK
generate_typescript_sdk() {
  local schema_file="${1:-schema.graphql}"
  local output_dir="${2:-./sdk/typescript}"

  printf "Generating TypeScript SDK...\n"

  mkdir -p "$output_dir/src"

  # Generate client wrapper
  cat >"$output_dir/src/client.ts" <<'EOF'
import { GraphQLClient } from 'graphql-request';

export interface NselfClientConfig {
  endpoint: string;
  adminSecret?: string;
  headers?: Record<string, string>;
}

export class NselfClient {
  private client: GraphQLClient;

  constructor(config: NselfClientConfig) {
    const headers: Record<string, string> = {
      ...config.headers,
    };

    if (config.adminSecret) {
      headers['x-hasura-admin-secret'] = config.adminSecret;
    }

    this.client = new GraphQLClient(config.endpoint, { headers });
  }

  async query<T = any>(query: string, variables?: Record<string, any>): Promise<T> {
    return this.client.request<T>(query, variables);
  }

  async mutate<T = any>(mutation: string, variables?: Record<string, any>): Promise<T> {
    return this.client.request<T>(mutation, variables);
  }

  setHeader(key: string, value: string): void {
    this.client.setHeader(key, value);
  }

  setHeaders(headers: Record<string, string>): void {
    this.client.setHeaders(headers);
  }
}
EOF

  # Generate authentication helpers
  cat >"$output_dir/src/auth.ts" <<'EOF'
import { NselfClient } from './client';

export interface SignUpParams {
  email: string;
  password: string;
  displayName?: string;
  metadata?: Record<string, any>;
}

export interface SignInParams {
  email: string;
  password: string;
}

export interface AuthResponse {
  accessToken: string;
  refreshToken?: string;
  user: {
    id: string;
    email: string;
    displayName?: string;
  };
}

export class AuthClient {
  constructor(private client: NselfClient) {}

  async signUp(params: SignUpParams): Promise<AuthResponse> {
    const mutation = `
      mutation SignUp($email: String!, $password: String!, $displayName: String, $metadata: jsonb) {
        signUp(email: $email, password: $password, displayName: $displayName, metadata: $metadata) {
          accessToken
          refreshToken
          user {
            id
            email
            displayName
          }
        }
      }
    `;

    return this.client.mutate(mutation, params);
  }

  async signIn(params: SignInParams): Promise<AuthResponse> {
    const mutation = `
      mutation SignIn($email: String!, $password: String!) {
        signIn(email: $email, password: $password) {
          accessToken
          refreshToken
          user {
            id
            email
            displayName
          }
        }
      }
    `;

    return this.client.mutate(mutation, params);
  }

  async signOut(): Promise<void> {
    const mutation = `
      mutation SignOut {
        signOut {
          success
        }
      }
    `;

    await this.client.mutate(mutation);
  }

  async refreshToken(refreshToken: string): Promise<AuthResponse> {
    const mutation = `
      mutation RefreshToken($refreshToken: String!) {
        refreshToken(refreshToken: $refreshToken) {
          accessToken
          refreshToken
          user {
            id
            email
            displayName
          }
        }
      }
    `;

    return this.client.mutate(mutation, { refreshToken });
  }
}
EOF

  # Generate index file
  cat >"$output_dir/src/index.ts" <<'EOF'
export { NselfClient, NselfClientConfig } from './client';
export { AuthClient, SignUpParams, SignInParams, AuthResponse } from './auth';
export { StorageClient, UploadFileParams, FileMetadata } from './storage';

// Re-export commonly used types
export type { GraphQLClient } from 'graphql-request';
EOF

  # Generate storage helpers
  cat >"$output_dir/src/storage.ts" <<'EOF'
export interface UploadFileParams {
  file: File | Buffer;
  name?: string;
  bucketId?: string;
}

export interface FileMetadata {
  id: string;
  name: string;
  mimeType: string;
  size: number;
  bucketId: string;
  url: string;
  createdAt: string;
}

export class StorageClient {
  private baseUrl: string;

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl;
  }

  async upload(params: UploadFileParams): Promise<FileMetadata> {
    const formData = new FormData();

    if (params.file instanceof File) {
      formData.append('file', params.file);
    } else {
      const blob = new Blob([params.file]);
      formData.append('file', blob, params.name || 'file');
    }

    if (params.bucketId) {
      formData.append('bucketId', params.bucketId);
    }

    const response = await fetch(`${this.baseUrl}/storage/upload`, {
      method: 'POST',
      body: formData,
    });

    if (!response.ok) {
      throw new Error(`Upload failed: ${response.statusText}`);
    }

    return response.json();
  }

  async delete(fileId: string): Promise<void> {
    const response = await fetch(`${this.baseUrl}/storage/files/${fileId}`, {
      method: 'DELETE',
    });

    if (!response.ok) {
      throw new Error(`Delete failed: ${response.statusText}`);
    }
  }

  getPublicUrl(fileId: string): string {
    return `${this.baseUrl}/storage/files/${fileId}`;
  }
}
EOF

  # Generate package.json
  cat >"$output_dir/package.json" <<EOF
{
  "name": "@nself/sdk",
  "version": "1.0.0",
  "description": "TypeScript SDK for nself backend",
  "main": "dist/index.js",
  "types": "dist/index.d.ts",
  "scripts": {
    "build": "tsc",
    "prepare": "npm run build"
  },
  "dependencies": {
    "graphql": "^16.8.1",
    "graphql-request": "^6.1.0"
  },
  "devDependencies": {
    "@types/node": "^20.10.0",
    "typescript": "^5.3.0"
  },
  "keywords": [
    "nself",
    "graphql",
    "hasura",
    "backend",
    "sdk"
  ]
}
EOF

  # Generate tsconfig.json
  cat >"$output_dir/tsconfig.json" <<EOF
{
  "compilerOptions": {
    "target": "ES2020",
    "module": "commonjs",
    "lib": ["ES2020", "DOM"],
    "declaration": true,
    "outDir": "./dist",
    "rootDir": "./src",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "moduleResolution": "node",
    "resolveJsonModule": true
  },
  "include": ["src/**/*"],
  "exclude": ["node_modules", "dist"]
}
EOF

  # Generate README
  cat >"$output_dir/README.md" <<'EOF'
# nself TypeScript SDK

Official TypeScript/JavaScript SDK for nself backends.

## Installation

```bash
npm install @nself/sdk graphql graphql-request
```

## Usage

### Basic Client

```typescript
import { NselfClient } from '@nself/sdk';

const client = new NselfClient({
  endpoint: 'https://api.yourdomain.com',
  adminSecret: 'your-admin-secret', // Optional
});

// Query data
const result = await client.query(`
  query GetUsers {
    users {
      id
      email
      displayName
    }
  }
`);

// Mutate data
await client.mutate(`
  mutation CreateUser($email: String!, $displayName: String!) {
    insert_users_one(object: {email: $email, displayName: $displayName}) {
      id
      email
    }
  }
`, {
  email: 'user@example.com',
  displayName: 'John Doe',
});
```

### Authentication

```typescript
import { NselfClient, AuthClient } from '@nself/sdk';

const client = new NselfClient({
  endpoint: 'https://api.yourdomain.com',
});

const auth = new AuthClient(client);

// Sign up
const { accessToken, user } = await auth.signUp({
  email: 'user@example.com',
  password: 'secure-password',
  displayName: 'John Doe',
});

// Sign in
const session = await auth.signIn({
  email: 'user@example.com',
  password: 'secure-password',
});

// Use access token
client.setHeader('Authorization', `Bearer ${session.accessToken}`);

// Refresh token
const newSession = await auth.refreshToken(session.refreshToken);
```

### Storage

```typescript
import { StorageClient } from '@nself/sdk';

const storage = new StorageClient('https://yourdomain.com');

// Upload file (browser)
const file = document.querySelector('input[type="file"]').files[0];
const metadata = await storage.upload({ file });

// Upload file (Node.js)
const buffer = fs.readFileSync('image.png');
const metadata = await storage.upload({
  file: buffer,
  name: 'image.png',
  bucketId: 'avatars',
});

// Get public URL
const url = storage.getPublicUrl(metadata.id);

// Delete file
await storage.delete(metadata.id);
```

## License

MIT
EOF

  printf "TypeScript SDK generated at: %s\n" "$output_dir"
  printf "Next steps:\n"
  printf "  cd %s\n" "$output_dir"
  printf "  npm install\n"
  printf "  npm run build\n"
}

# Generate Python SDK
generate_python_sdk() {
  local schema_file="${1:-schema.graphql}"
  local output_dir="${2:-./sdk/python}"

  printf "Generating Python SDK...\n"

  mkdir -p "$output_dir/nself"

  # Generate client
  cat >"$output_dir/nself/client.py" <<'EOF'
from typing import Any, Dict, Optional
import requests


class NselfClient:
    """GraphQL client for nself backend."""

    def __init__(
        self,
        endpoint: str,
        admin_secret: Optional[str] = None,
        headers: Optional[Dict[str, str]] = None
    ):
        self.endpoint = endpoint
        self.session = requests.Session()

        if headers:
            self.session.headers.update(headers)

        if admin_secret:
            self.session.headers['x-hasura-admin-secret'] = admin_secret

    def query(self, query: str, variables: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """Execute a GraphQL query."""
        response = self.session.post(
            f"{self.endpoint}/v1/graphql",
            json={
                'query': query,
                'variables': variables or {}
            }
        )
        response.raise_for_status()

        result = response.json()

        if 'errors' in result:
            raise Exception(f"GraphQL errors: {result['errors']}")

        return result.get('data', {})

    def mutate(self, mutation: str, variables: Optional[Dict[str, Any]] = None) -> Dict[str, Any]:
        """Execute a GraphQL mutation."""
        return self.query(mutation, variables)

    def set_header(self, key: str, value: str) -> None:
        """Set a custom header."""
        self.session.headers[key] = value

    def set_headers(self, headers: Dict[str, str]) -> None:
        """Set multiple headers."""
        self.session.headers.update(headers)
EOF

  # Generate auth client
  cat >"$output_dir/nself/auth.py" <<'EOF'
from typing import Any, Dict, Optional
from dataclasses import dataclass

from .client import NselfClient


@dataclass
class User:
    id: str
    email: str
    display_name: Optional[str] = None


@dataclass
class AuthResponse:
    access_token: str
    user: User
    refresh_token: Optional[str] = None


class AuthClient:
    """Authentication client for nself."""

    def __init__(self, client: NselfClient):
        self.client = client

    def sign_up(
        self,
        email: str,
        password: str,
        display_name: Optional[str] = None,
        metadata: Optional[Dict[str, Any]] = None
    ) -> AuthResponse:
        """Sign up a new user."""
        mutation = """
            mutation SignUp($email: String!, $password: String!, $displayName: String, $metadata: jsonb) {
                signUp(email: $email, password: $password, displayName: $displayName, metadata: $metadata) {
                    accessToken
                    refreshToken
                    user {
                        id
                        email
                        displayName
                    }
                }
            }
        """

        result = self.client.mutate(mutation, {
            'email': email,
            'password': password,
            'displayName': display_name,
            'metadata': metadata
        })

        data = result['signUp']
        return AuthResponse(
            access_token=data['accessToken'],
            refresh_token=data.get('refreshToken'),
            user=User(
                id=data['user']['id'],
                email=data['user']['email'],
                display_name=data['user'].get('displayName')
            )
        )

    def sign_in(self, email: str, password: str) -> AuthResponse:
        """Sign in an existing user."""
        mutation = """
            mutation SignIn($email: String!, $password: String!) {
                signIn(email: $email, password: $password) {
                    accessToken
                    refreshToken
                    user {
                        id
                        email
                        displayName
                    }
                }
            }
        """

        result = self.client.mutate(mutation, {
            'email': email,
            'password': password
        })

        data = result['signIn']
        return AuthResponse(
            access_token=data['accessToken'],
            refresh_token=data.get('refreshToken'),
            user=User(
                id=data['user']['id'],
                email=data['user']['email'],
                display_name=data['user'].get('displayName')
            )
        )

    def sign_out(self) -> None:
        """Sign out current user."""
        mutation = """
            mutation SignOut {
                signOut {
                    success
                }
            }
        """

        self.client.mutate(mutation)

    def refresh_token(self, refresh_token: str) -> AuthResponse:
        """Refresh access token."""
        mutation = """
            mutation RefreshToken($refreshToken: String!) {
                refreshToken(refreshToken: $refreshToken) {
                    accessToken
                    refreshToken
                    user {
                        id
                        email
                        displayName
                    }
                }
            }
        """

        result = self.client.mutate(mutation, {
            'refreshToken': refresh_token
        })

        data = result['refreshToken']
        return AuthResponse(
            access_token=data['accessToken'],
            refresh_token=data.get('refreshToken'),
            user=User(
                id=data['user']['id'],
                email=data['user']['email'],
                display_name=data['user'].get('displayName')
            )
        )
EOF

  # Generate storage client
  cat >"$output_dir/nself/storage.py" <<'EOF'
from typing import BinaryIO, Optional
from dataclasses import dataclass
import requests


@dataclass
class FileMetadata:
    id: str
    name: str
    mime_type: str
    size: int
    bucket_id: str
    url: str
    created_at: str


class StorageClient:
    """Storage client for file uploads."""

    def __init__(self, base_url: str):
        self.base_url = base_url
        self.session = requests.Session()

    def upload(
        self,
        file: BinaryIO,
        name: Optional[str] = None,
        bucket_id: Optional[str] = None
    ) -> FileMetadata:
        """Upload a file."""
        files = {'file': (name or 'file', file)}
        data = {}

        if bucket_id:
            data['bucketId'] = bucket_id

        response = self.session.post(
            f"{self.base_url}/storage/upload",
            files=files,
            data=data
        )
        response.raise_for_status()

        result = response.json()
        return FileMetadata(
            id=result['id'],
            name=result['name'],
            mime_type=result['mimeType'],
            size=result['size'],
            bucket_id=result['bucketId'],
            url=result['url'],
            created_at=result['createdAt']
        )

    def delete(self, file_id: str) -> None:
        """Delete a file."""
        response = self.session.delete(
            f"{self.base_url}/storage/files/{file_id}"
        )
        response.raise_for_status()

    def get_public_url(self, file_id: str) -> str:
        """Get public URL for a file."""
        return f"{self.base_url}/storage/files/{file_id}"
EOF

  # Generate __init__.py
  cat >"$output_dir/nself/__init__.py" <<'EOF'
"""nself Python SDK - Official Python client for nself backends."""

from .client import NselfClient
from .auth import AuthClient, User, AuthResponse
from .storage import StorageClient, FileMetadata

__version__ = "1.0.0"
__all__ = [
    "NselfClient",
    "AuthClient",
    "User",
    "AuthResponse",
    "StorageClient",
    "FileMetadata",
]
EOF

  # Generate setup.py
  cat >"$output_dir/setup.py" <<'EOF'
from setuptools import setup, find_packages

with open("README.md", "r", encoding="utf-8") as fh:
    long_description = fh.read()

setup(
    name="nself",
    version="1.0.0",
    author="nself",
    description="Official Python SDK for nself backends",
    long_description=long_description,
    long_description_content_type="text/markdown",
    url="https://github.com/acamarata/nself",
    packages=find_packages(),
    classifiers=[
        "Development Status :: 4 - Beta",
        "Intended Audience :: Developers",
        "License :: OSI Approved :: MIT License",
        "Programming Language :: Python :: 3",
        "Programming Language :: Python :: 3.8",
        "Programming Language :: Python :: 3.9",
        "Programming Language :: Python :: 3.10",
        "Programming Language :: Python :: 3.11",
    ],
    python_requires=">=3.8",
    install_requires=[
        "requests>=2.28.0",
    ],
)
EOF

  # Generate README
  cat >"$output_dir/README.md" <<'EOF'
# nself Python SDK

Official Python SDK for nself backends.

## Installation

```bash
pip install nself
```

## Usage

### Basic Client

```python
from nself import NselfClient

client = NselfClient(
    endpoint="https://api.yourdomain.com",
    admin_secret="your-admin-secret"  # Optional
)

# Query data
result = client.query("""
    query GetUsers {
        users {
            id
            email
            displayName
        }
    }
""")

# Mutate data
result = client.mutate("""
    mutation CreateUser($email: String!, $displayName: String!) {
        insert_users_one(object: {email: $email, displayName: $displayName}) {
            id
            email
        }
    }
""", {
    "email": "user@example.com",
    "displayName": "John Doe"
})
```

### Authentication

```python
from nself import NselfClient, AuthClient

client = NselfClient(endpoint="https://api.yourdomain.com")
auth = AuthClient(client)

# Sign up
response = auth.sign_up(
    email="user@example.com",
    password="secure-password",
    display_name="John Doe"
)

# Sign in
session = auth.sign_in(
    email="user@example.com",
    password="secure-password"
)

# Use access token
client.set_header("Authorization", f"Bearer {session.access_token}")

# Refresh token
new_session = auth.refresh_token(session.refresh_token)
```

### Storage

```python
from nself import StorageClient

storage = StorageClient("https://yourdomain.com")

# Upload file
with open("image.png", "rb") as f:
    metadata = storage.upload(
        file=f,
        name="image.png",
        bucket_id="avatars"
    )

# Get public URL
url = storage.get_public_url(metadata.id)

# Delete file
storage.delete(metadata.id)
```

## License

MIT
EOF

  printf "Python SDK generated at: %s\n" "$output_dir"
  printf "Next steps:\n"
  printf "  cd %s\n" "$output_dir"
  printf "  pip install -e .\n"
}

# Main SDK generation function
generate_sdk() {
  local language="${1:-typescript}"
  local output_dir="${2:-./sdk/$language}"

  # Source environment
  if [[ -f .env ]]; then
    set -a
    source .env
    set +a
  fi

  local endpoint
  endpoint=$(get_hasura_endpoint)

  printf "Generating %s SDK...\n" "$language"
  printf "GraphQL endpoint: %s\n" "$endpoint"

  case "$language" in
    typescript | ts)
      generate_typescript_sdk "schema.graphql" "$output_dir"
      ;;

    python | py)
      generate_python_sdk "schema.graphql" "$output_dir"
      ;;

    *)
      printf "Unsupported language: %s\n" "$language" >&2
      printf "Supported: typescript, python\n" >&2
      return 1
      ;;
  esac

  printf "\nSDK generated successfully!\n"
}

export -f get_hasura_endpoint fetch_graphql_schema
export -f generate_typescript_sdk generate_python_sdk generate_sdk
