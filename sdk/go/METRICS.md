# Universal `/metrics` Endpoint

Every nSelf plugin exposes Prometheus-format metrics at `GET /metrics` when
it uses `sdk/server.New(...)`. The core counters are identical across
plugins so operators can write one dashboard, one alert set, and one Grafana
template and have it work for the whole ecosystem.

## Core metrics (every plugin)

| Metric | Type | Labels | Meaning |
| --- | --- | --- | --- |
| `nself_plugin_requests_total` | Counter | `route`, `method`, `status` | Total HTTP requests handled |
| `nself_plugin_request_duration_seconds` | Histogram | `route`, `method` | Request latency histogram (default buckets) |
| `nself_plugin_in_flight_requests` | Gauge | — | Requests currently being handled |
| `nself_plugin_errors_total` | Counter | `kind` | Errors by kind (`db`, `upstream`, `timeout`, `auth`, `not_ready`, …) |
| `nself_plugin_build_info` | Gauge | `version` | Always 1. Use for `group_left` joins to surface the running version |

Each of these carries a constant `plugin=<name>` label, stamped by
`metrics.NewRegistry(pluginName, version)` at startup.

Standard Go runtime collectors are also registered:

- `go_*` (goroutines, GC pause, memory)
- `process_*` (file descriptors, CPU, RSS, start time)

## Enabling metrics in your plugin

Use `sdk/server.New` and pass nothing — the `/metrics` endpoint is mounted
automatically:

```go
srv := sdkserver.New(sdkserver.Options{
    Plugin:  "widget",
    Version: Version,
})
```

To wrap specific routes with the shared request counter + histogram:

```go
srv := sdkserver.New(sdkserver.Options{
    Plugin:  "widget",
    Version: Version,
    Routes: func(r chi.Router, m *sdkmetrics.Registry) {
        r.With(m.Middleware("/v1/predict")).Post("/v1/predict", handler)
    },
})
```

The `route` label should be a stable template (`/v1/predict`), never a raw
URL path with variables, to prevent cardinality blowup.

## Adding plugin-specific metrics

Register custom metrics on the shared registry so they appear on the same
`/metrics` endpoint:

```go
reg := sdkmetrics.NewRegistry("widget", Version)

widgetQueueDepth := prometheus.NewGauge(prometheus.GaugeOpts{
    Name:        "nself_widget_queue_depth",
    Help:        "Number of pending widget jobs.",
    ConstLabels: prometheus.Labels{"plugin": "widget"},
})
reg.Reg.MustRegister(widgetQueueDepth)
```

## Operator contract

- Prometheus scrapes `/metrics` every 15 seconds by default.
- Plugins must keep per-label cardinality under 10,000 time series.
- Do NOT emit `user_id`, `tenant_id`, or any PII label; aggregate at the
  plugin boundary and emit counts.
- Emit `nself_plugin_errors_total{kind="..."}` on every caught error — this
  is the universal alert signal operators wire up.

## Alert defaults

The monitoring bundle ships alerts keyed on the universal metrics:

```text
nself_plugin_errors_total rate > 1/s for 5m → warning
nself_plugin_requests_total{status="5xx"} rate / total > 1% for 10m → critical
nself_plugin_in_flight_requests > 500 for 5m → warning
nself_plugin_request_duration_seconds{quantile="0.99"} > 2s for 10m → warning
```

Plugin authors can override these by shipping their own rules file under
`bundles/paid/<plugin>/monitoring/alerts.yml`.

## Verification

```bash
curl -s http://127.0.0.1:8080/metrics | grep nself_plugin_requests_total
```

Should return lines scoped to `plugin="<name>"`.
