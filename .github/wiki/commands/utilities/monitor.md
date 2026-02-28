# nself monitor

**Category**: Utilities

Launch real-time monitoring dashboard for all nself services.

## Overview

Interactive terminal dashboard showing live metrics, logs, and status for all services.

**Features**:
- ✅ Real-time metrics (CPU, memory, network)
- ✅ Live log streaming
- ✅ Service health indicators
- ✅ Interactive navigation
- ✅ Multiple dashboard views

## Usage

```bash
nself monitor [OPTIONS] [VIEW]
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `-v, --view VIEW` | Dashboard view (overview/metrics/logs) | overview |
| `-r, --refresh N` | Refresh interval (seconds) | 2 |
| `--no-color` | Disable color output | false |
| `-s, --service SERVICE` | Focus on specific service | all |

## Arguments

| Argument | Description |
|----------|-------------|
| `VIEW` | Dashboard view to display (optional) |

## Examples

### Basic Monitor

```bash
nself monitor
```

**Output** (live updating):
```
╔═══════════════════════════════════════════════════════════╗
║         nself Real-Time Monitor - Press q to quit         ║
╚═══════════════════════════════════════════════════════════╝

Service Overview                          Updated: 14:30:15
──────────────────────────────────────────────────────────
Service     Status  CPU    Memory    Network     Uptime
postgres    🟢 UP    12%    234 MB   ↓1.2↑0.8    2h 15m
hasura      🟢 UP    5%     156 MB   ↓0.8↑0.5    2h 15m
auth        🟢 UP    2%     89 MB    ↓0.2↑0.1    2h 15m
nginx       🟢 UP    1%     12 MB    ↓0.5↑0.3    2h 15m
redis       🟢 UP    1%     45 MB    ↓0.1↑0.0    2h 14m
minio       🟢 UP    3%     178 MB   ↓0.3↑0.2    2h 14m

System Resources
──────────────────────────────────────────────────────────
CPU Total:  21% [████░░░░░░░░░░░░░░░░]  (12 cores)
Memory:     714 MB / 8 GB (8.9%) [██░░░░░░░░░░░░░░░░]
Disk:       45 GB / 500 GB (9%) [██░░░░░░░░░░░░░░░░]
Network:    ↓ 3.1 MB/s  ↑ 1.9 MB/s

Recent Activity
──────────────────────────────────────────────────────────
14:30:12 postgres  Query completed: SELECT * FROM users
14:30:13 hasura    GraphQL request: 45ms
14:30:14 auth      User login: user@example.com
14:30:15 nginx     HTTP 200: GET /api/graphql

[o]verview  [m]etrics  [l]ogs  [h]elp  [q]uit
```

### Metrics View

```bash
nself monitor --view metrics
```

**Output** (live charts):
```
╔═══════════════════════════════════════════════════════════╗
║              Service Metrics - PostgreSQL                 ║
╚═══════════════════════════════════════════════════════════╝

CPU Usage (last 60s)
  20% ┤                        ╭╮
  15% ┤                      ╭╯╰╮     ╭╮
  10% ┤         ╭╮          ╭╯  ╰╮   ╭╯╰╮
   5% ┤  ╭╮    ╭╯╰╮   ╭╮  ╭╯    ╰╮ ╭╯  ╰╮
   0% ┼──╯╰────╯  ╰───╯╰──╯      ╰─╯    ╰──

Memory Usage (last 60s)
 250M ┤                              ╭─
 200M ┤                          ╭───╯
 150M ┤                   ╭──────╯
 100M ┤           ╭───────╯
  50M ┤   ╭───────╯
   0M ┼───╯

Network I/O (last 60s)
 2MB ┤    ╭╮    ╭╮        ╭╮
 1MB ┤╭╮╭╮││╭╮╭╮││╭╮  ╭╮╭╮││
 0MB ┼╯╰╯╰╯╰╯╰╯╰╯╰╯╰──╯╰╯╰╯╰

Active Connections: 5
Query Throughput: 42 queries/s
Cache Hit Rate: 89%

[←→] Change service  [↑↓] Scroll  [q] Back
```

### Logs View

```bash
nself monitor --view logs
```

**Output** (live log streaming):
```
╔═══════════════════════════════════════════════════════════╗
║                 Live Logs - All Services                  ║
╚═══════════════════════════════════════════════════════════╝

[14:30:10] postgres  | LOG:  connection received
[14:30:11] hasura    | INFO: Query executed in 45ms
[14:30:12] auth      | INFO: JWT token issued
[14:30:12] nginx     | 200 GET /api/graphql
[14:30:13] postgres  | LOG:  query: SELECT * FROM users
[14:30:14] hasura    | INFO: Cache hit for query
[14:30:15] redis     | INFO: GET user:session:abc123
[14:30:15] auth      | INFO: User authenticated: user@example.com
[14:30:16] nginx     | 200 POST /v1/auth/signin
[14:30:17] postgres  | LOG:  checkpoint starting
                                                   ▼ More logs

Filters: [a]ll [e]rrors [w]arnings [s]ervice [q]uit
```

### Monitor Specific Service

```bash
nself monitor --service postgres
```

**Focuses dashboard on single service.**

### Custom Refresh Rate

```bash
nself monitor --refresh 5
```

**Updates every 5 seconds instead of default 2.**

## Dashboard Views

### Overview (Default)

Shows all services with key metrics:
- Status indicators
- CPU and memory usage
- Network activity
- Uptime
- Recent log activity

**Navigate**: Press `o` or start with default view

### Metrics

Detailed metrics with historical graphs:
- CPU usage over time
- Memory consumption trends
- Network I/O graphs
- Service-specific metrics

**Navigate**: Press `m` in overview

### Logs

Live log streaming from all services:
- Color-coded by service
- Filtering by level/service
- Search functionality
- Tail last N lines

**Navigate**: Press `l` in overview

### Health

Health check results:
- Service health status
- Response times
- Failed check details
- Dependency status

**Navigate**: Press `h` in overview

## Keyboard Shortcuts

### Navigation

| Key | Action |
|-----|--------|
| `o` | Overview view |
| `m` | Metrics view |
| `l` | Logs view |
| `h` | Health view |
| `q` | Quit monitor |
| `r` | Refresh now |
| `p` | Pause updates |

### Filtering (Logs View)

| Key | Action |
|-----|--------|
| `a` | Show all logs |
| `e` | Show errors only |
| `w` | Show warnings only |
| `s` | Select service filter |
| `/` | Search logs |

### Service Selection

| Key | Action |
|-----|--------|
| `←` `→` | Navigate services |
| `1-9` | Select service by number |
| `0` | Show all services |

## Terminal Requirements

### Minimum Requirements

- Terminal with 256 colors
- Minimum 80x24 characters
- Unicode support (for graphs)

### Recommended

- iTerm2, Alacritty, or modern terminal
- 120x40 characters or larger
- True color support

### Check Compatibility

```bash
# Check colors
echo $COLORTERM

# Check size
tput cols && tput lines

# Test unicode
echo "Test: ╭─╮│╰─╯ ░▒▓█"
```

## Monitoring Configuration

### Custom Thresholds

```bash
# In .env
MONITOR_CPU_WARNING=70     # CPU % warning threshold
MONITOR_CPU_CRITICAL=90    # CPU % critical threshold
MONITOR_MEM_WARNING=80     # Memory % warning
MONITOR_MEM_CRITICAL=95    # Memory % critical
```

### Color Indicators

```
🟢 Green  - Healthy (< warning threshold)
🟡 Yellow - Warning (warning ≤ x < critical)
🔴 Red    - Critical (≥ critical threshold)
⚪ Gray   - Stopped/Unknown
```

## Export Metrics

### Save Snapshot

While monitoring, press `e` to export current state:

```bash
# Creates timestamped snapshot
metrics-snapshot-20260213-143015.json
```

**Contents**:
```json
{
  "timestamp": "2026-02-13T14:30:15Z",
  "services": {
    "postgres": {
      "status": "running",
      "cpu_percent": 12.3,
      "memory_mb": 234,
      "network_rx_mbps": 1.2,
      "network_tx_mbps": 0.8
    }
  }
}
```

### Continuous Logging

```bash
# Log metrics to file
nself monitor --export metrics.jsonl &
```

## Remote Monitoring

### Monitor Remote Server

```bash
# Via SSH
ssh user@server 'cd /app && nself monitor'

# Or with forwarding
ssh -t user@server 'cd /app && nself monitor'
```

### Monitoring Server Setup

```bash
# Run monitor as service
nself monitor --daemon --export /var/log/nself-metrics.jsonl
```

## Integration

### With Prometheus

```bash
# Export metrics in Prometheus format
nself monitor --prometheus > metrics.prom

# Serve via HTTP
nself monitor --prometheus --http :9090
```

### With Grafana

Import dashboard template:
```bash
# Export Grafana dashboard JSON
nself monitor --export-dashboard > nself-dashboard.json

# Import in Grafana UI
```

### With ELK Stack

```bash
# Stream to Logstash
nself monitor --view logs --format json | \
  logstash -f /etc/logstash/nself.conf
```

## Troubleshooting

### Monitor Won't Start

```bash
# Check terminal compatibility
echo $TERM

# Try fallback mode
nself monitor --simple
```

### Graphics Broken

```bash
# Disable Unicode charts
nself monitor --no-unicode

# Or ASCII only
nself monitor --ascii
```

### High CPU from Monitor

```bash
# Increase refresh interval
nself monitor --refresh 10

# Disable live graphs
nself monitor --view overview
```

## Alternatives

### Basic Status Check

```bash
# Simple status (no live updates)
nself status

# Watch with native watch command
watch -n 2 nself status
```

### Web-Based Monitoring

```bash
# Open Grafana dashboard
nself admin monitoring

# Or directly
open https://grafana.localhost
```

## Related Commands

- `nself status` - One-time status check
- `nself logs` - View logs without monitoring
- `nself health` - Run health checks
- `nself admin` - Web-based admin UI

## See Also

- [nself status](status.md)
- [nself logs](logs.md)
- [Monitoring Guide](../../guides/MONITORING.md)
- [Grafana Dashboards](../../guides/GRAFANA.md)
