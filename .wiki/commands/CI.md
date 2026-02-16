# nself dev ci - CI/CD Configuration

> **DEPRECATED COMMAND NAME**: This command was formerly `nself ci` in v0.x. It has been consolidated to `nself dev ci` in v1.0. The old command name may still work as an alias.

**Version 0.9.9** | Generate CI/CD pipeline configuration

---

## Overview

The `nself dev ci` command generates CI/CD pipeline configuration files for popular platforms. It creates ready-to-use workflows for testing, building, and deploying nself projects.

---

## Basic Usage

```bash
# Interactive setup
nself dev ci

# Generate for specific platform
nself dev ci github
nself dev ci gitlab
nself dev ci bitbucket
```

---

## Supported Platforms

| Platform | Command | Output |
|----------|---------|--------|
| GitHub Actions | `nself dev ci github` | `.github/workflows/` |
| GitLab CI | `nself dev ci gitlab` | `.gitlab-ci.yml` |
| Bitbucket Pipelines | `nself dev ci bitbucket` | `bitbucket-pipelines.yml` |
| CircleCI | `nself dev ci circleci` | `.circleci/config.yml` |

---

## Generated Workflows

### Test Workflow

```yaml
# Runs on pull requests
- Lint code
- Run unit tests
- Run integration tests
```

### Deploy Workflow

```yaml
# Runs on main branch
- Build containers
- Push to registry
- Deploy to staging
- (Manual) Deploy to production
```

---

## Options Reference

| Option | Description |
|--------|-------------|
| `--platform` | CI platform |
| `--with-deploy` | Include deployment |
| `--registry` | Container registry |
| `--output` | Output directory |

---

## Environment Variables

Set these in your CI platform:

| Variable | Description |
|----------|-------------|
| `DEPLOY_SSH_KEY` | SSH key for deployment |
| `DOCKER_USERNAME` | Registry username |
| `DOCKER_PASSWORD` | Registry password |
| `STAGING_HOST` | Staging server address |
| `PROD_HOST` | Production server address |

---

## Example: GitHub Actions

```bash
# Generate GitHub Actions workflow
nself dev ci github --with-deploy
```

Creates:
- `.github/workflows/test.yml`
- `.github/workflows/deploy.yml`

---

## See Also

- [deploy](DEPLOY.md) - Deployment
- [build](BUILD.md) - Build configuration
