# Domain Selection Guide for nself Development

## Quick Recommendations

### Best Options for Development

1. **`local.nself.org`** (Recommended)
   - ✅ Automatic wildcard SSL certificate
   - ✅ No /etc/hosts configuration needed
   - ✅ Works immediately out of the box
   - ✅ Resolves to 127.0.0.1 globally
   - Example: `https://api.local.nself.org`, `https://admin.local.nself.org`

2. **`*.localhost`** (Modern browsers)
   - ✅ Auto-resolves in Chrome, Firefox, Edge (no /etc/hosts needed)
   - ✅ SSL certificates set up automatically during build
   - ✅ Clear indication it's local development
   - 🔑 One-time sudo password during `nself build`
   - Example: `https://api.localhost`, `https://myapp.localhost`

3. **`.test` domains** (Custom but standard)
   - ✅ Reserved TLD for testing (RFC 6761)
   - ✅ /etc/hosts configured automatically during build
   - ✅ SSL certificates set up automatically
   - ✅ Won't conflict with real domains
   - 🔑 One-time sudo password during `nself build`
   - Example: `https://myapp.test`, `https://api.myapp.test`

4. **Port-based localhost** (Simplest, no subdomains)
   - ✅ Works everywhere immediately
   - ❌ No subdomain support
   - ❌ Services on different ports
   - Example: `http://localhost:8080`, `http://localhost:3005`

## Domains to Avoid

### Never Use These

- **`.local`** - Reserved for mDNS/Bonjour, causes conflicts on macOS/Linux
- **`.dev`** - Real TLD with forced HTTPS (HSTS), won't work locally
- **`.app`** - Real TLD with forced HTTPS (HSTS), won't work locally
- **`.io`, `.com`, `.org`** - Real TLDs, could conflict with actual sites

## SSL/TLS Certificates - Fully Automated!

### 🎉 Good news: nself handles everything automatically!

During `nself build`, we automatically:
1. Install mkcert if needed (via brew)
2. Set up the local certificate authority
3. Generate wildcard SSL certificates
4. Configure nginx to use HTTPS

**The only thing you might see:** A one-time sudo password prompt to trust the certificates.

### Certificate Setup by Domain Type

| Domain Type | SSL Setup | User Action Required |
|------------|-----------|---------------------|
| `local.nself.org` | Pre-configured wildcard cert | None |
| `*.localhost` | Auto-generated during build | One-time sudo password |
| `.test` domains | Auto-generated during build | One-time sudo password |
| Custom domains | Auto-generated during build | One-time sudo password |

### What happens during `nself build`:

```bash
$ nself build
✓ Checking environment...
✓ Installing mkcert... (if needed)
✓ Setting up certificate authority... (may ask for sudo password once)
✓ Generating SSL certificates...
✓ Configuring nginx...
✓ Build complete!
```

That's it! Your services will be available via HTTPS automatically.

## Browser Support

### Automatic Resolution (No /etc/hosts needed)

| Domain | Chrome | Firefox | Safari | Edge |
|--------|--------|---------|--------|------|
| `localhost` | ✅ | ✅ | ✅ | ✅ |
| `*.localhost` | ✅ v89+ | ✅ v90+ | ✅ v15+ | ✅ |
| `local.nself.org` | ✅ | ✅ | ✅ | ✅ |
| `.test` domains | ❌ | ❌ | ❌ | ❌ |

## /etc/hosts Configuration

### 🤖 Automated Setup

nself automatically handles /etc/hosts during `nself build`:
- Detects which domains need entries
- Prompts once for sudo to add them
- Updates are non-destructive (preserves existing entries)

### What gets added (example for `.test` domains):

```bash
# nself entries (added automatically)
127.0.0.1 myapp.test
127.0.0.1 api.myapp.test
127.0.0.1 hasura.myapp.test
127.0.0.1 admin.myapp.test
127.0.0.1 storage.myapp.test
```

**Note:** For `localhost` and `local.nself.org`, no /etc/hosts changes are needed!

## Wizard Domain Flow

The nself wizard now offers these options:

1. **local.nself.org** - Zero configuration, just works
2. **localhost with subdomains** - Modern browser support, optional mkcert
3. **Custom domain** - Full control, requires setup
4. **Port-based localhost** - No subdomains, different ports

## Wizard Selection

The nself wizard keeps it simple with just three options:

1. **local.nself.org (recommended)** - Press enter and it works
2. **localhost** - Universal fallback option
3. **Custom prefix** - Add your project name

## Migration Guide

### From `.local` domains
```bash
# Old (problematic)
BASE_DOMAIN=myapp.local

# New (recommended)
BASE_DOMAIN=myapp.test        # Custom with /etc/hosts
BASE_DOMAIN=myapp.localhost   # Auto-resolves in modern browsers
BASE_DOMAIN=local.nself.org   # Zero configuration
```

### Adding SSL to existing setup

Just run `nself build` again! It will:
1. Detect your current domain configuration
2. Install mkcert if needed
3. Generate appropriate certificates
4. Update nginx configuration
5. You'll only be asked for sudo password once

```bash
$ nself build
# Automatically handles everything!
```

### Manual SSL setup (optional, for advanced users)

If you prefer to set up certificates manually:
```bash
# nself build will detect and use these
mkdir -p .certs
cd .certs
mkcert -cert-file localhost.crt -key-file localhost.key localhost *.localhost
```