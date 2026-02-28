# OAuth Handlers API Reference

Complete API reference for the nself OAuth handlers service.

## Base URL

**Development:** `http://localhost:3100`
**Production:** `https://your-domain.com` (or custom OAuth service URL)

---

## Endpoints

### GET /health

Health check endpoint.

**Response:**
```json
{
  "status": "healthy",
  "service": "oauth-handlers",
  "timestamp": "2026-01-30T12:00:00.000Z",
  "providers": ["google", "github"]
}
```

**Status Codes:**
- `200 OK` - Service is healthy

---

### GET /oauth/providers

List all enabled OAuth providers.

**Response:**
```json
{
  "providers": [
    {
      "name": "google",
      "displayName": "Google",
      "iconUrl": "https://www.google.com/favicon.ico",
      "color": "#4285f4",
      "authUrl": "/oauth/google"
    },
    {
      "name": "github",
      "displayName": "GitHub",
      "iconUrl": "https://github.com/favicon.ico",
      "color": "#24292e",
      "authUrl": "/oauth/github"
    }
  ]
}
```

**Status Codes:**
- `200 OK` - Success

**Usage:**
```typescript
const response = await fetch('http://localhost:3100/oauth/providers');
const { providers } = await response.json();

// Render login buttons
providers.forEach(provider => {
  console.log(provider.displayName, provider.authUrl);
});
```

---

### GET /oauth/:provider

Initiate OAuth flow for a specific provider.

**Parameters:**
- `provider` (path) - Provider name: `google`, `github`, `microsoft`, `slack`
- `redirect` (query, optional) - URL path to redirect to after authentication

**Example:**
```
GET /oauth/google
GET /oauth/google?redirect=/dashboard
GET /oauth/github?redirect=/settings
```

**Response:**
- HTTP 302 redirect to provider's authorization page

**Flow:**
1. User clicks "Sign in with Google"
2. Browser redirects to `/oauth/google`
3. Service redirects to Google's OAuth page
4. User authorizes
5. Google redirects back to service callback
6. Service processes callback and redirects to frontend with JWT

**Status Codes:**
- `302 Found` - Redirect to provider
- `404 Not Found` - Provider not enabled
- `500 Internal Server Error` - Handler initialization failed

---

### GET /oauth/:provider/callback

OAuth callback endpoint (handled automatically by provider).

**Parameters:**
- `code` (query, required) - Authorization code from provider
- `state` (query, required) - CSRF protection token
- `error` (query, optional) - OAuth error code
- `error_description` (query, optional) - Error description

**Success Response:**
- HTTP 302 redirect to frontend with JWT token

**Redirect URL:**
```
http://localhost:3000/?token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

If `redirect` query param was provided during initiation:
```
http://localhost:3000/dashboard?token=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

**Error Response:**
- HTTP 302 redirect to frontend error page

**Error Redirect URL:**
```
http://localhost:3000/auth/error?error=oauth_failed&description=Invalid+credentials
```

**Status Codes:**
- `302 Found` - Redirect to frontend (success or error)
- `400 Bad Request` - Invalid state or missing code
- `500 Internal Server Error` - Internal error

**Error Codes:**
- `invalid_state` - State validation failed (CSRF protection)
- `invalid_callback` - Missing code or state parameter
- `oauth_failed` - OAuth exchange or profile retrieval failed
- `handler_not_found` - Provider handler not initialized

---

### POST /oauth/:provider/token

Exchange authorization code for token (for mobile apps or server-to-server).

**Parameters:**
- `provider` (path) - Provider name

**Request Body:**
```json
{
  "code": "4/0AY0e-g7..."
}
```

**Success Response:**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "user@example.com",
    "displayName": "John Doe",
    "avatarUrl": "https://lh3.googleusercontent.com/..."
  }
}
```

**Error Response:**
```json
{
  "error": "Token exchange failed",
  "message": "Invalid authorization code"
}
```

**Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Missing code
- `404 Not Found` - Provider not found
- `500 Internal Server Error` - Token exchange failed

**Usage (Mobile App):**
```typescript
// After getting authorization code from OAuth flow
const response = await fetch('http://localhost:3100/oauth/google/token', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ code: authorizationCode }),
});

const { token, user } = await response.json();

// Store token
await AsyncStorage.setItem('authToken', token);
```

---

## JWT Token

### Format

The JWT token contains the following claims:

```json
{
  "sub": "550e8400-e29b-41d4-a716-446655440000",
  "email": "user@example.com",
  "displayName": "John Doe",
  "provider": "google",
  "iat": 1706616000,
  "exp": 1707220800
}
```

### Claims

- `sub` - User ID (UUID)
- `email` - User's email address
- `displayName` - User's display name
- `provider` - OAuth provider used for authentication
- `iat` - Issued at (Unix timestamp)
- `exp` - Expiration time (Unix timestamp)

### Expiration

Default: 7 days (configurable via `JWT_EXPIRES_IN` environment variable)

### Verification

```typescript
import jwt from 'jsonwebtoken';

const token = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...';
const secret = process.env.JWT_SECRET;

try {
  const decoded = jwt.verify(token, secret);
  console.log('User:', decoded.sub);
  console.log('Email:', decoded.email);
} catch (error) {
  console.error('Invalid token:', error.message);
}
```

---

## Frontend Integration

### Web Application (React/Next.js)

#### Login Button

```typescript
function GoogleLoginButton() {
  const handleLogin = () => {
    const redirectUrl = '/dashboard';
    window.location.href = `http://localhost:3100/oauth/google?redirect=${redirectUrl}`;
  };

  return (
    <button onClick={handleLogin}>
      Sign in with Google
    </button>
  );
}
```

#### Callback Handler

```typescript
import { useEffect } from 'react';
import { useRouter } from 'next/router';

function AuthCallback() {
  const router = useRouter();

  useEffect(() => {
    const token = router.query.token as string;
    const error = router.query.error as string;

    if (token) {
      // Store token
      localStorage.setItem('authToken', token);

      // Redirect to app
      router.push('/dashboard');
    } else if (error) {
      // Handle error
      const description = router.query.description as string;
      console.error('OAuth error:', error, description);
      router.push('/login?error=' + error);
    }
  }, [router.query]);

  return <div>Authenticating...</div>;
}
```

#### Dynamic Provider List

```typescript
import { useEffect, useState } from 'react';

function LoginPage() {
  const [providers, setProviders] = useState([]);

  useEffect(() => {
    fetch('http://localhost:3100/oauth/providers')
      .then(res => res.json())
      .then(data => setProviders(data.providers));
  }, []);

  return (
    <div>
      <h1>Sign In</h1>
      {providers.map(provider => (
        <button
          key={provider.name}
          onClick={() => {
            window.location.href = `http://localhost:3100${provider.authUrl}`;
          }}
          style={{ backgroundColor: provider.color }}
        >
          <img src={provider.iconUrl} alt={provider.displayName} />
          Sign in with {provider.displayName}
        </button>
      ))}
    </div>
  );
}
```

---

### Mobile Application (React Native)

#### Using In-App Browser

```typescript
import { useEffect } from 'react';
import { Linking } from 'react-native';
import InAppBrowser from 'react-native-inappbrowser-reborn';

async function handleGoogleLogin() {
  const authUrl = 'http://localhost:3100/oauth/google';

  if (await InAppBrowser.isAvailable()) {
    const result = await InAppBrowser.openAuth(authUrl, 'myapp://callback');

    if (result.type === 'success' && result.url) {
      // Extract token from URL
      const url = new URL(result.url);
      const token = url.searchParams.get('token');

      if (token) {
        // Store token
        await AsyncStorage.setItem('authToken', token);
        // Navigate to app
      }
    }
  }
}
```

#### Using Authorization Code Exchange

```typescript
// 1. Open browser to get authorization code
const authUrl = 'http://localhost:3100/oauth/google';
await Linking.openURL(authUrl);

// 2. Handle deep link callback
Linking.addEventListener('url', async (event) => {
  const url = new URL(event.url);
  const code = url.searchParams.get('code');

  if (code) {
    // 3. Exchange code for token
    const response = await fetch('http://localhost:3100/oauth/google/token', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ code }),
    });

    const { token, user } = await response.json();

    // 4. Store token
    await AsyncStorage.setItem('authToken', token);
  }
});
```

---

## Error Handling

### Error Response Format

```json
{
  "error": "error_code",
  "message": "Human-readable error message"
}
```

### Common Errors

| Error Code | Description | Solution |
|------------|-------------|----------|
| `provider_not_found` | OAuth provider not enabled | Enable provider with `nself oauth enable` |
| `handler_not_found` | OAuth handler initialization failed | Check provider configuration |
| `invalid_callback` | Missing code or state parameter | Retry OAuth flow |
| `invalid_state` | State validation failed (CSRF) | Retry OAuth flow |
| `oauth_failed` | Token exchange or profile retrieval failed | Check provider credentials |
| `token_exchange_failed` | Failed to exchange authorization code | Check OAuth app configuration |
| `missing_code` | Authorization code not provided | Provide code in request body |

---

## Rate Limiting

Currently no rate limiting is implemented. For production deployments, consider adding:

1. **API Gateway rate limiting** (e.g., nginx, Kong)
2. **Application-level rate limiting** (e.g., express-rate-limit)
3. **Provider-specific rate limits** (varies by OAuth provider)

---

## CORS Configuration

The OAuth handlers service allows requests from the configured frontend URL only:

```env
FRONTEND_URL=http://localhost:3000
```

For multiple frontends, configure CORS in `src/index.ts`:

```typescript
app.use(cors({
  origin: [
    'http://localhost:3000',
    'https://yourdomain.com',
  ],
  credentials: true,
}));
```

---

## Security Considerations

### CSRF Protection

All OAuth flows use state parameter for CSRF protection:

1. Generate random state on initiation
2. Store state with timestamp
3. Verify state matches on callback
4. State expires after 10 minutes
5. State is consumed (single-use)

### HTTPS Requirements

**Development:** HTTP allowed for localhost
**Production:** HTTPS required for all callbacks

### Token Security

- Tokens are signed with `JWT_SECRET`
- Tokens expire after configured duration (default 7 days)
- Tokens include user ID, email, and provider
- Tokens should be stored securely (HttpOnly cookies recommended)

---

## See Also

- [OAuth Setup Guide](../../guides/OAUTH-SETUP.md)
- [OAuth CLI Reference](../../commands/OAUTH.md)
- [Frontend Integration Guide](../../guides/OAUTH-COMPLETE-FLOWS.md)

---

**Version:** nself v0.8.0+
**Last Updated:** January 30, 2026
