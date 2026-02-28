# OAuth Setup Guide

Complete guide to setting up OAuth authentication with nself.

## Overview

nself supports OAuth integration with:
- Google OAuth
- GitHub OAuth
- Apple OAuth
- Custom OAuth providers

For detailed setup instructions, see [Authentication Guide](AUTHENTICATION.md#oauth-integration).

## Google OAuth Setup

1. Create OAuth 2.0 Credentials in Google Cloud Console
2. Configure in .env:
   ```bash
   OAUTH_GOOGLE_CLIENT_ID=your-client-id
   OAUTH_GOOGLE_CLIENT_SECRET=your-secret
   ```
3. Build and restart: `nself build && nself restart auth`

## GitHub OAuth Setup

1. Create OAuth App in GitHub Settings
2. Configure in .env:
   ```bash
   OAUTH_GITHUB_CLIENT_ID=your-client-id
   OAUTH_GITHUB_CLIENT_SECRET=your-secret
   ```
3. Build and restart: `nself build && nself restart auth`

## Apple OAuth Setup

Apple OAuth requires additional configuration. See [Authentication Guide](AUTHENTICATION.md) for complete setup.

## See Also

- [Authentication Guide](AUTHENTICATION.md) - Complete auth setup
- [Deployment Guide](DEPLOYMENT-ARCHITECTURE.md) - Production OAuth setup

