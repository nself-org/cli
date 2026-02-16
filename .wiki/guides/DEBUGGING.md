# Debugging Guide

Debug and troubleshoot nself issues.

## Health Check

```bash
nself health
```

Shows status of all services.

## View Logs

```bash
nself logs         # All services
nself logs postgres -f  # Follow logs
nself logs --since 1h  # Last hour
```

## Run Diagnostics

```bash
nself doctor
```

Comprehensive system diagnostics.

## Getting Help

```bash
nself help
nself help <command>
```

