# Creating GitHub Personal Access Token for Docker Push

## Steps to Create Token

1. Go to: https://github.com/settings/tokens/new
2. Set token name: `nself-docker-push`
3. Select expiration: 30 days (or your preference)
4. Select scopes:
   - ✅ `write:packages` - Upload packages to GitHub Package Registry
   - ✅ `read:packages` - Download packages from GitHub Package Registry
   - ✅ `delete:packages` - Delete packages from GitHub Package Registry (optional)
5. Click "Generate token"
6. Copy the token (starts with `ghp_`)

## Use the Token

```bash
# Set token as environment variable
export GITHUB_TOKEN=ghp_YOUR_TOKEN_HERE

# Login to Docker
echo $GITHUB_TOKEN | docker login ghcr.io -u acamarata --password-stdin

# Push images
docker push ghcr.io/nself-org/cli:0.3.9
docker push ghcr.io/nself-org/cli:latest
docker push ghcr.io/nself-org/cli:0.3
```

## Alternative: Use Docker Hub Instead

If you prefer Docker Hub:

```bash
# Login to Docker Hub
docker login -u acamarata

# Tag for Docker Hub
docker tag ghcr.io/nself-org/cli:0.3.9 acamarata/nself:0.3.9
docker tag ghcr.io/nself-org/cli:0.3.9 acamarata/nself:latest
docker tag ghcr.io/nself-org/cli:0.3.9 acamarata/nself:0.3

# Push to Docker Hub
docker push acamarata/nself:0.3.9
docker push acamarata/nself:latest
docker push acamarata/nself:0.3
```