package httpapi

// Purpose: own the authenticated HTTP listener and extension points.
// Inputs: a machine registry, project directory, authentication and limits.
// Outputs: a handler and a gracefully cancellable listener.
// Constraints: one guard per server; no package initialization work.

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/hostcase"
	"github.com/nself-org/cli/internal/invoke"
)

// Principal identifies an authenticated caller; it carries no authorization grants.
type Principal struct {
	// Subject is the caller's opaque identity, not a token or a permission list.
	Subject string
}

// AuthHook authenticates every route before its body is read. An error refuses
// the request with E426; a successful principal does not bypass invocation gates.
type AuthHook interface {
	// Authenticate returns the caller identity or an error without consuming the body.
	Authenticate(*http.Request) (Principal, error)
}
type authFunc func(*http.Request) (Principal, error)

func (f authFunc) Authenticate(r *http.Request) (Principal, error) { return f(r) }

// Invoker executes a registry command with the request context and server guard.
// Implementations must honor cancellation and the shared invocation contracts.
type Invoker func(context.Context, *cmdregistry.Registry, string, invoke.Request, invoke.Options) (invoke.Result, error)

// BearerAuth checks a case-sensitive Bearer token using constant-time comparison
// of SHA-256 digests. An empty configured token rejects every request, and so
// does a request with more than one Authorization header (the winner would be
// ambiguous between this server and any proxy in front of it).
func BearerAuth(tok string) AuthHook {
	want := sha256.Sum256([]byte(tok))
	return authFunc(func(r *http.Request) (Principal, error) {
		value, ok := strings.CutPrefix(headerValue(r.Header, "Authorization"), "Bearer ")
		got := sha256.Sum256([]byte(value))
		if !ok || tok == "" || len(r.Header.Values("Authorization")) != 1 || subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			return Principal{}, errs.New("E426", "Bearer authentication required")
		}
		return Principal{Subject: "bearer"}, nil
	})
}

// Options configures one server. Registry and Auth are required; New supplies
// documented defaults for optional limits without opening a listener.
type Options struct {
	// Registry is the startup command snapshot; callers must not mutate it while serving.
	Registry *cmdregistry.Registry
	// Invoker executes commands; nil selects invoke.Invoke with the server's guard.
	Invoker Invoker
	// ProjectDir is the invocation working directory, not an HTTP-controlled value.
	ProjectDir string
	// Port is the required Host/Origin port; it must match the listener's actual port.
	Port int
	// AllowedHosts adds DNS names, IPv4 or bracketed IPv6 literals to the loopback
	// defaults. Values must omit ports, schemes, paths and whitespace.
	AllowedHosts []string
	// Timeout limits document invocations; nonpositive values use invoke.DefaultTimeout.
	// NDJSON invocations instead run until their request context ends.
	Timeout time.Duration
	// MaxInFlight caps command invocations; nonpositive values default to four.
	MaxInFlight int
	// Auth authenticates all routes before body reads and must not be nil.
	Auth AuthHook
	// DataSchema supplies committed data schemas by command path for OpenAPI.
	// Nil or a missing schema produces an open data object.
	DataSchema func(string) ([]byte, bool)
	// Log receives serialized request metadata only; nil selects os.Stderr.
	Log io.Writer
	// ReadHeaderTimeout bounds header reads; nonpositive values default to ten seconds.
	ReadHeaderTimeout time.Duration
	// ReadTimeout bounds request reads; nonpositive values default to thirty seconds.
	// The deadline is cleared after command request decoding to permit streams.
	ReadTimeout time.Duration
	// IdleTimeout bounds keep-alive idle time; nonpositive values default to sixty seconds.
	IdleTimeout time.Duration
}

// Server owns one authenticated mux, startup registry snapshot and invocation
// guard. Configure mounts before serving; concurrent mount changes are unsupported.
type Server struct {
	o                 Options
	guard             *invoke.Guard
	routes            map[string]string
	hosts             map[string]bool
	openapi           []byte
	sem               chan struct{}
	mux               *http.ServeMux
	logMu             sync.Mutex
	invokeMu          sync.Mutex
	invocations       sync.WaitGroup
	activeInvocations int
	draining          bool
}

// New validates options and builds routes, OpenAPI and one shared invocation
// guard. It does not listen or execute commands; Registry and Auth are required.
func New(o Options) (*Server, error) {
	if err := ValidateAllowedHosts(o.AllowedHosts); err != nil {
		return nil, err
	}
	if o.Registry == nil {
		return nil, errors.New("HTTP API requires a registry")
	}
	if o.Auth == nil {
		return nil, errors.New("HTTP API requires an auth hook")
	}
	if o.Invoker == nil {
		o.Invoker = invoke.Invoke
	}
	if o.MaxInFlight <= 0 {
		o.MaxInFlight = 4
	}
	if o.Timeout <= 0 {
		o.Timeout = invoke.DefaultTimeout
	}
	if o.Log == nil {
		o.Log = os.Stderr
	}
	if o.ReadHeaderTimeout <= 0 {
		o.ReadHeaderTimeout = 10 * time.Second
	}
	if o.ReadTimeout <= 0 {
		o.ReadTimeout = 30 * time.Second
	}
	if o.IdleTimeout <= 0 {
		o.IdleTimeout = 60 * time.Second
	}
	doc, err := OpenAPI(o.Registry, o.DataSchema)
	if err != nil {
		return nil, err
	}
	auditPath, err := invoke.DefaultAuditPath()
	if err != nil {
		return nil, err
	}
	s := &Server{o: o, guard: invoke.NewGuard(auditPath), routes: map[string]string{}, hosts: map[string]bool{}, openapi: doc, sem: make(chan struct{}, o.MaxInFlight), mux: http.NewServeMux()}
	for _, c := range o.Registry.Commands {
		s.routes[cmdregistry.RoutePath(c.Path)] = strings.TrimPrefix(c.Path, "nself ")
	}
	for _, host := range append([]string{"localhost", "127.0.0.1", "[::1]"}, o.AllowedHosts...) {
		s.hosts[hostcase.LowerASCII(fmt.Sprintf("%s:%d", host, o.Port))] = true
	}
	s.mux.HandleFunc("/", s.route)
	return s, nil
}

// Handler returns the private mux behind Host, Origin and AuthHook checks and
// request logging. Command routes additionally enforce invocation validation.
func (s *Server) Handler() http.Handler { return s.logged(s.authorized(s.mux)) }

// Mount adds a handler behind Host, Origin and AuthHook checks before serving.
// It refuses nil handlers, conflicting patterns, the root, /v1 and /openapi.json
// prefixes, non-absolute paths and pattern, escape, whitespace or query syntax.
func (s *Server) Mount(prefix string, h http.Handler) (err error) {
	if h == nil || !strings.HasPrefix(prefix, "/") || strings.ContainsAny(prefix, "%{} \t\r\n?#") || strings.HasPrefix(prefix, "/v1") || strings.HasPrefix(prefix, "/openapi.json") || prefix == "/" {
		return errors.New("reserved or invalid mount")
	}
	defer func() {
		if recover() != nil {
			err = errors.New("conflicting mount")
		}
	}()
	s.mux.Handle(prefix, h)
	return nil
}

// ListenAndServe listens on addr without TLS or a write timeout. Cancellation
// stops accepting requests and permits five seconds to drain before closing
// remaining connections and canceling their request contexts. It then waits
// for invocation child cleanup and serialized audit writes, budgeting for every
// outstanding invocation. Context values are preserved; its deadline and cancellation do not interrupt the graceful drain.
// A completed shutdown returns nil; listener failures return their error.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.Serve(ctx, ln)
}

// Serve serves an already bound listener with the same cancellation and graceful
// shutdown behavior as ListenAndServe. It closes the listener when serving ends.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	h := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: s.o.ReadHeaderTimeout, ReadTimeout: s.o.ReadTimeout, IdleTimeout: s.o.IdleTimeout, BaseContext: func(net.Listener) context.Context { return context.WithoutCancel(ctx) }}
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if h.Shutdown(c) != nil {
				_ = h.Close()
			}
		case <-done:
		}
	}()
	err := h.Serve(ln)
	close(done)
	<-stopped
	_ = h.Close()
	s.drainInvocations(s.invocationDrainBudget())
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
