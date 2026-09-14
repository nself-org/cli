module github.com/nself-org/cli

// go 1.26.6 (full patch version, not bare "1.26"): workflows resolve the Go
// version via `go-version-file: go.mod`, and a bare "1.26" made setup-go pick a
// cached 1.26.5, after which the `toolchain` directive forced a runtime
// toolchain download that failed with tar "File exists" collisions. Pinning the
// patch here makes every workflow resolve 1.26.6 directly.
//
// 1.26.6 patches GO-2026-6218 (net/url quadratic resolvePath), GO-2026-6091
// (html/template JS regexp context), GO-2026-6090 (crypto/tls post-handshake
// message limit), GO-2026-6089 (net/http H2C ReadHeaderTimeout), GO-2026-6088
// (encoding/xml recursion depth), GO-2026-5972 (encoding/asn1 recursion depth),
// GO-2026-5856 (crypto/tls ECH privacy leak) and GO-2026-5026 (net/http idna
// Punycode), plus the earlier GO-2026-5039 / GO-2026-5037 / GO-2026-4971 /
// GO-2026-4918 stdlib fixes. Upgrade when rebuilding release artifacts.
go 1.26.6

require (
	github.com/bytecodealliance/wasmtime-go/v25 v25.0.0
	github.com/cenkalti/backoff/v5 v5.0.3
	github.com/go-chi/chi/v5 v5.3.2
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.11.0
	github.com/joho/godotenv v1.5.1
	github.com/mark3labs/mcp-go v0.58.0
	github.com/prometheus/client_golang v1.24.1
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
	go.opentelemetry.io/otel v1.46.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.46.0
	go.opentelemetry.io/otel/sdk v1.46.0
	go.opentelemetry.io/otel/trace v1.46.0
	golang.org/x/term v0.46.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dlclark/regexp2 v1.11.5 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/jsonschema-go v0.4.2 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.30.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.70.1 // indirect
	github.com/prometheus/procfs v0.21.1 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.46.0 // indirect
	go.opentelemetry.io/otel/metric v1.46.0 // indirect
	go.opentelemetry.io/proto/otlp v1.11.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260819154853-08b0e4226688 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260819154853-08b0e4226688 // indirect
	google.golang.org/grpc v1.83.1 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)
