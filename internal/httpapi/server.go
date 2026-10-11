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
	"github.com/nself-org/cli/internal/invoke"
)

type Principal struct{ Subject string }
type AuthHook interface {
	Authenticate(*http.Request) (Principal, error)
}
type authFunc func(*http.Request) (Principal, error)

func (f authFunc) Authenticate(r *http.Request) (Principal, error) { return f(r) }

type Invoker func(context.Context, *cmdregistry.Registry, string, invoke.Request, invoke.Options) (invoke.Result, error)

func BearerAuth(tok string) AuthHook {
	want := sha256.Sum256([]byte(tok))
	return authFunc(func(r *http.Request) (Principal, error) {
		value, ok := strings.CutPrefix(headerValue(r.Header, "Authorization"), "Bearer ")
		got := sha256.Sum256([]byte(value))
		if !ok || tok == "" || subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
			return Principal{}, errs.New("E426", "Bearer authentication required")
		}
		return Principal{Subject: "bearer"}, nil
	})
}

type Options struct {
	Registry                                    *cmdregistry.Registry
	Invoker                                     Invoker
	ProjectDir                                  string
	Port                                        int
	AllowedHosts                                []string
	Timeout                                     time.Duration
	MaxInFlight                                 int
	Auth                                        AuthHook
	DataSchema                                  func(string) ([]byte, bool)
	Log                                         io.Writer
	ReadHeaderTimeout, ReadTimeout, IdleTimeout time.Duration
}

type Server struct {
	o       Options
	guard   *invoke.Guard
	routes  map[string]string
	hosts   map[string]bool
	openapi []byte
	sem     chan struct{}
	mux     *http.ServeMux
	logMu   sync.Mutex
}

func New(o Options) (*Server, error) {
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
		s.hosts[strings.ToLower(fmt.Sprintf("%s:%d", host, o.Port))] = true
	}
	s.mux.HandleFunc("/", s.route)
	return s, nil
}

// Handler wraps the private mux so mounted routes share every safety check.
func (s *Server) Handler() http.Handler { return s.logged(s.authorized(s.mux)) }

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

func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	h := &http.Server{Addr: addr, Handler: s.Handler(), ReadHeaderTimeout: s.o.ReadHeaderTimeout, ReadTimeout: s.o.ReadTimeout, IdleTimeout: s.o.IdleTimeout, BaseContext: func(net.Listener) context.Context { return ctx }}
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
	err := h.ListenAndServe()
	close(done)
	<-stopped
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
