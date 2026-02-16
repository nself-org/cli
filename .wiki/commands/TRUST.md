# nself trust - Trust SSL Certificates

**Version 0.9.9** | Install root CA to system trust store

---

## Overview

The `nself trust` command installs the mkcert root CA certificate into your system's trust store. This enables browsers to trust locally-generated SSL certificates without security warnings.

---

## Basic Usage

```bash
# Install root CA
nself trust

# Check trust status
nself trust --check

# Remove from trust store
nself trust --uninstall
```

---

## How It Works

1. **mkcert** generates a root CA on first use
2. `nself trust` installs this CA to your system
3. All certificates signed by this CA are trusted
4. Browsers show green lock for `*.localhost` domains

---

## Platform Support

| Platform | Trust Store |
|----------|-------------|
| macOS | System Keychain |
| Linux | NSS/ca-certificates |
| Windows (WSL) | Certificate Manager |

---

## Verification

After running `nself trust`, verify:

```bash
# Check if CA is installed
nself trust --check

# Test with curl
curl -s https://api.local.nself.org

# Check in browser
# Visit https://local.nself.org - should show secure
```

---

## Troubleshooting

### Browser Still Shows Warning

1. Restart your browser
2. Clear browser SSL cache
3. Re-run `nself trust`

### Permission Denied

```bash
# macOS - may need sudo
sudo nself trust

# Linux - may need root
sudo nself trust
```

### mkcert Not Found

```bash
# Install mkcert first
brew install mkcert  # macOS
# or
apt install mkcert   # Ubuntu/Debian
```

---

## See Also

- [ssl](SSL.md) - SSL certificate management
- [build](BUILD.md) - Generate configuration
