# Service Management Guide

Manage, monitor, and troubleshoot nself services.

## Service Operations

Start, stop, and restart services:

```bash
# Start all services
nself start

# Stop all services
nself stop

# Restart specific service
nself restart postgres
```

## Monitoring Services

```bash
# Check service health
nself health

# View service logs
nself logs postgres

# Monitor in real-time
nself logs postgres -f
```

See [Service Commands](../commands/SERVICE.md) for complete reference.

