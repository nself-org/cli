# OAuth Quick Start Guide

Get OAuth authentication working in 5 minutes.

## 1. Install OAuth Handlers

```bash
nself oauth install
```

## 2. Enable Providers

```bash
nself oauth enable --providers google,github
```

## 3. Get OAuth Credentials

### Google

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create project → APIs & Services → Credentials
3. Create OAuth client ID (Web application)
4. Add redirect URI: `http://localhost:3100/oauth/google/callback`
5. Copy Client ID and Client Secret

### GitHub

1. Go to [GitHub Settings](https://github.com/settings/developers)
2. OAuth Apps → New OAuth App
3. Callback URL: `http://localhost:3100/oauth/github/callback`
4. Copy Client ID and Client Secret

## 4. Configure Credentials

```bash
nself oauth config google \
  --client-id=YOUR_CLIENT_ID \
  --client-secret=YOUR_CLIENT_SECRET

nself oauth config github \
  --client-id=YOUR_CLIENT_ID \
  --client-secret=YOUR_CLIENT_SECRET
```

## 5. Build and Start

```bash
nself build
nself start
```

## 6. Test

Visit: `http://localhost:3100/oauth/google`

You should be redirected to Google login, then back to your frontend with a JWT token.

## 7. Integrate with Frontend

```typescript
// Login button
<button onClick={() => {
  window.location.href = 'http://localhost:3100/oauth/google';
}}>
  Sign in with Google
</button>

// Handle callback
useEffect(() => {
  const token = new URLSearchParams(window.location.search).get('token');
  if (token) {
    localStorage.setItem('authToken', token);
    window.location.href = '/dashboard';
  }
}, []);
```

## Done!

Your OAuth authentication is now working.

## Next Steps

- [Complete OAuth Setup Guide](./OAUTH-SETUP.md)
- [OAuth CLI Reference](../commands/oauth.md)
- [Security Best Practices](./OAUTH-SETUP.md#security-best-practices)

## Troubleshooting

### "Provider not enabled"
```bash
nself oauth enable --providers google
```

### "Missing credentials"
```bash
nself oauth config google --client-id=xxx --client-secret=xxx
```

### "Service not running"
```bash
nself start
docker logs oauth-handlers
```

### Check status
```bash
nself oauth status
nself oauth test google
```

---

**Need help?** See [OAuth Setup Guide](./OAUTH-SETUP.md) for detailed instructions.
