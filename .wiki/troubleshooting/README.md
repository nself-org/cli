# nself Troubleshooting Guide

This directory contains comprehensive troubleshooting documentation for nself.

## Quick Links

- **[Error Messages Guide](ERROR-MESSAGES.md)** - Detailed explanations of all error messages and how to fix them
- **[Common Issues](../getting-started/FAQ.md)** - Frequently asked questions and quick fixes
- **[Configuration Guide](../configuration/README.md)** - Environment variable reference and configuration best practices

## Quick Diagnostics

### 1. Run Health Check
```bash
nself doctor
```

This command checks:
- System requirements (Docker, memory, disk)
- Configuration files
- Service health
- Port availability
- SSL certificates

### 2. Auto-Fix Common Issues
```bash
nself doctor --fix
```

Automatically fixes:
- Port conflicts
- Missing configuration
- Unhealthy containers
- Database schema issues
- Docker networking

### 3. View Service Status
```bash
nself status
```

Shows:
- Running services
- Health status
- Resource usage
- Container information

## Common Error Scenarios

### Top 10 Issues (Quick Reference)

| # | Issue | Quick Fix |
|---|-------|-----------|
| 1 | Port conflict | `nself doctor --fix` |
| 2 | Container won't start | `nself logs <service>` then `nself restart <service>` |
| 3 | Missing config | `nself init` |
| 4 | Permission denied | `sudo chown -R $(whoami) ./` |
| 5 | Network error | `nself stop && docker network prune -f && nself start` |
| 6 | Docker not running | `open -a Docker` (macOS) or `sudo systemctl start docker` (Linux) |
| 7 | Low memory | Increase Docker memory or disable optional services |
| 8 | Database error | `nself restart postgres` |
| 9 | Build failure | `docker builder prune -f && nself build` |
| 10 | Health check failed | `nself logs <service>` then `nself restart <service>` |

### Detailed Solutions

See [ERROR-MESSAGES.md](ERROR-MESSAGES.md) for:
- Complete error message reference
- Step-by-step solutions
- Platform-specific fixes
- Prevention best practices

## Diagnostic Tools

### Built-in Commands

```bash
# Comprehensive health check
nself doctor

# Auto-fix issues
nself doctor --fix

# Service status
nself status
nself status --detailed

# View logs
nself logs <service>
nself logs <service> --tail 100
nself logs <service> --follow

# Service URLs
nself urls

# Container analysis
nself doctor containers
```

### Docker Commands

```bash
# List running containers
docker ps

# View all containers (including stopped)
docker ps -a

# Check resource usage
docker stats

# Inspect container
docker inspect ${PROJECT_NAME}_<service>

# View Docker logs
docker logs ${PROJECT_NAME}_<service>

# Check networks
docker network ls
docker network inspect nself_default
```

### System Commands

```bash
# Check disk space
df -h .
docker system df

# Check memory (Linux)
free -h

# Check memory (macOS)
top -l 1 | head -n 10

# Check port usage
lsof -i :5432
netstat -an | grep LISTEN

# Check processes
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
```

## Error Message Format

All nself error messages follow this consistent format:

```
✗ [Clear Problem Statement]

  Reason: [Specific cause of the error]

  Possible solutions:
  1. [Primary solution with copy-paste command]
     command to run

  2. [Alternative solution]

  3. [Additional option or automatic fix]

  Run 'nself doctor' for more diagnostics
```

Benefits:
- **Clear** - Immediately understand what went wrong
- **Actionable** - Step-by-step solutions with commands
- **Hierarchical** - Solutions ordered by likelihood of success
- **Consistent** - Same format across all errors

## Prevention Best Practices

### Before Starting

1. **Always run diagnostics first:**
   ```bash
   nself doctor
   ```

2. **Check system resources:**
   ```bash
   df -h .        # Disk space
   docker stats   # Memory/CPU
   ```

3. **Verify Docker is running:**
   ```bash
   docker info
   docker ps
   ```

### During Development

1. **Use `.env.local` for customizations:**
   - Override ports that conflict
   - Set machine-specific values
   - Never commit `.env.local`

2. **Monitor logs in real-time:**
   ```bash
   nself logs <service> --follow
   ```

3. **Regular cleanup:**
   ```bash
   nself clean
   docker system prune -a
   ```

### After Changes

1. **Validate configuration:**
   ```bash
   nself config validate
   ```

2. **Rebuild when needed:**
   ```bash
   nself build
   ```

3. **Health check after restart:**
   ```bash
   nself start && nself doctor
   ```

## Platform-Specific Notes

### macOS

- **Docker Desktop memory:** Settings → Resources → Memory (4GB minimum)
- **Port 5000 conflict:** macOS AirPlay uses port 5000 (MLflow default)
- **File permissions:** Docker Desktop handles most permissions automatically

### Linux

- **Docker group:** Add user with `sudo usermod -aG docker $USER`
- **Systemd services:** Use `systemctl` to manage Docker
- **Firewall:** May need to allow Docker networking

### Windows (WSL)

- **WSL2 required:** Ensure using WSL2, not WSL1
- **Memory allocation:** Configure in `.wslconfig`
- **File permissions:** Use Linux paths, not Windows paths

## Getting More Help

### Documentation

- **Main docs:** [docs/README.md](../README.md)
- **Configuration:** [Configuration](../configuration/README.md)
- **Deployment:** [Deployment](../deployment/README.md)
- **Development:** [Development](../development/INDEX.md)

### Command Help

```bash
nself help
nself <command> --help
```

### Community & Support

- **GitHub Issues:** [Report bugs or request features](https://github.com/nself-org/cli/issues)
- **Discussions:** [Ask questions](https://github.com/nself-org/cli/discussions)
- **Examples:** [Browse example projects](https://github.com/nself-org/cli/tree/main/examples)

### Debugging Tips

1. **Enable debug mode:**
   ```bash
   DEBUG=true nself start
   ```

2. **Verbose output:**
   ```bash
   nself doctor --verbose
   nself start --verbose
   ```

3. **Save logs for support:**
   ```bash
   nself logs > nself-debug.log
   docker ps -a >> nself-debug.log
   docker network ls >> nself-debug.log
   ```

4. **Check version:**
   ```bash
   nself version
   docker --version
   docker compose version
   ```

## Contributing

### Improving Error Messages

Found an unclear error message?

1. Open an issue describing the confusion
2. Suggest improvements
3. Submit a PR with better messaging

### Adding Troubleshooting Docs

Have a solution to share?

1. Add to [ERROR-MESSAGES.md](ERROR-MESSAGES.md)
2. Update this README with quick reference
3. Submit a PR

## Appendix

### Error Categories

- **Port Conflicts** - Service can't bind to port
- **Container Failures** - Service won't start
- **Configuration** - Missing or invalid settings
- **Permissions** - Access denied errors
- **Network** - Connectivity issues
- **Resources** - Memory/disk insufficient
- **Database** - PostgreSQL connection problems
- **Build** - Docker build failures
- **Health Checks** - Service unhealthy

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Misuse of command |
| 126 | Permission denied |
| 127 | Command not found |
| 130 | Terminated by Ctrl+C |

### Useful Links

- [Docker Documentation](https://docs.docker.com/)
- [Docker Compose Documentation](https://docs.docker.com/compose/)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)
- [Hasura Documentation](https://hasura.io/docs/)

---

**Last Updated:** January 2026
**nself Version:** v0.9.6+
