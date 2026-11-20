package platform

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rluders/amaru/services/platform/internal/adapter/discovery"
	"github.com/rluders/amaru/services/platform/internal/adapter/localauth"
	"github.com/rluders/amaru/services/platform/internal/adapter/operational"
)

// HTTPConfig supplies deployment values and concrete authorized product handlers.
// An empty StateDir selects the per-user discovery location. Explicit paths
// are supplied by internal callers such as tests.
type HTTPConfig struct {
	StateDir, Version, InstanceID string
	Lifecycle                     *Lifecycle
	Logger                        *operational.Logger
	OnLogError                    func(error)
	Handler                       func(*localauth.Auth) (http.Handler, error)
}

// HTTP owns one bound listener, ephemeral Auth and private discovery publication.
// Register last, after prerequisite stages. Like the other owners, it is single-use.
type HTTP struct {
	config                        HTTPConfig
	ready, started, served        chan struct{}
	initErr                       error
	listener                      net.Listener
	server                        *http.Server
	auth                          *localauth.Auth
	publication                   *discovery.Publication
	cancelRequests, cancelStreams context.CancelFunc
	mu                            sync.Mutex
	stopping                      bool
	active                        int
	drained                       chan struct{}
	hijacked                      map[net.Conn]bool
	readyOnce, stopOnce           sync.Once
	stopped                       chan struct{}
	stopErr                       error
}

func NewHTTP(config HTTPConfig) (*HTTP, error) {
	if strings.TrimSpace(config.Version) == "" || config.InstanceID == "" || config.Lifecycle == nil || config.Logger == nil || config.OnLogError == nil || config.Handler == nil {
		return nil, errors.New("HTTP requires deployment values, lifecycle, diagnostics and product handlers")
	}
	return &HTTP{config: config, ready: make(chan struct{}), started: make(chan struct{}), served: make(chan struct{}), drained: make(chan struct{}), stopped: make(chan struct{}), hijacked: map[net.Conn]bool{}, initErr: errors.New("HTTP initialization did not complete")}, nil
}

func (*HTTP) Name() string { return "http-discovery" }

func (h *HTTP) Start(ctx context.Context) error {
	defer close(h.started)
	defer h.publish(errors.New("HTTP initialization did not complete"))
	if err := ctx.Err(); err != nil {
		h.initErr = err
		return err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		h.initErr = err
		return err
	}
	h.listener = listener
	h.auth, err = localauth.New(listener.Addr())
	if err != nil {
		h.initErr = err
		return err
	}
	product, err := h.config.Handler(h.auth)
	if err != nil {
		h.initErr = err
		return err
	}
	if product == nil {
		return errors.New("HTTP product handler is missing")
	}
	started := time.Now().UTC()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "protocolVersion": 1, "platformVersion": h.config.Version, "startedAt": started, "uptimeSec": int64(time.Since(started).Seconds())})
	})
	mux.Handle("/", product)
	requests, cancelRequests := context.WithCancel(context.Background())
	streams, cancelStreams := context.WithCancel(context.Background())
	h.cancelRequests, h.cancelStreams = cancelRequests, cancelStreams
	handler := h.config.Logger.HTTP(h.config.InstanceID, h.auth.Handler(mux), h.config.OnLogError)
	unavailable := h.config.Logger.HTTP(h.config.InstanceID, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Amaru-Protocol", "1")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":{"code":"internal","message":"Instance is not ready"}}`)
	}), h.config.OnLogError)
	h.server = &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h.mu.Lock()
			if h.stopping || !h.config.Lifecycle.Ready() {
				h.mu.Unlock()
				unavailable.ServeHTTP(w, r)
				return
			}
			h.active++
			h.mu.Unlock()
			defer func() {
				h.mu.Lock()
				defer h.mu.Unlock()
				h.active--
				if h.stopping && h.active == 0 {
					close(h.drained)
				}
			}()
			if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
				streamCtx, cancel := context.WithCancel(r.Context())
				stop := context.AfterFunc(streams, cancel)
				defer stop()
				defer cancel()
				r = r.WithContext(streamCtx)
			}
			handler.ServeHTTP(w, r)
		}),
		BaseContext:       func(net.Listener) context.Context { return requests },
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second,
		// net/http's raw panic/TLS diagnostics must not bypass the safe middleware.
		ErrorLog: log.New(io.Discard, "", 0),
		ConnState: func(conn net.Conn, state http.ConnState) {
			h.mu.Lock()
			switch state {
			case http.StateHijacked:
				if !h.stopping {
					h.hijacked[conn] = true
				}
			case http.StateClosed:
				delete(h.hijacked, conn)
			}
			closing := state == http.StateHijacked && h.stopping
			h.mu.Unlock()
			if closing {
				_ = conn.Close()
			}
		},
	}
	entered := make(chan struct{})
	go func() {
		defer close(h.served)
		defer func() {
			_ = recover() // Start classifies every serving exit as failure.
		}()
		_ = h.server.Serve(&httpListener{Listener: listener, entered: entered, owner: h})
	}()
	select {
	case <-entered:
	case <-h.served:
		return errors.New("HTTP serving exited during initialization")
	case <-ctx.Done():
		h.initErr = ctx.Err()
		return h.initErr
	}
	if err := ctx.Err(); err != nil {
		h.initErr = err
		return err
	}
	dir := h.config.StateDir
	if dir == "" {
		dir, err = discovery.PrepareStateDirectory()
		if err != nil {
			h.initErr = err
			return err
		}
	}
	h.publication, err = discovery.Publish(dir, listener, h.auth.AdministratorSecret(), h.config.Version, started)
	if err != nil {
		h.initErr = err
		return err
	}
	if err := ctx.Err(); err != nil {
		h.initErr = err
		return err
	}
	h.initErr = nil
	// Readiness precedes the serving lifetime; do not wait for Start to return.
	h.publish(nil)
	select {
	case <-ctx.Done():
		h.cancelStreams()
		return nil
	case <-h.served:
		return errors.New("HTTP serving exited unexpectedly")
	}
}

// httpListener signals that Serve has initialized and reached Accept before
// discovery can be published. Aggregate readiness still controls admission.
type httpListener struct {
	net.Listener
	entered chan struct{}
	once    sync.Once
	owner   *HTTP
}

func (l *httpListener) Accept() (net.Conn, error) {
	l.once.Do(func() { close(l.entered) })
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &httpConn{Conn: conn, owner: l.owner}, nil
}

// A hijacked connection leaves net/http ownership; remove it when its transport closes.
type httpConn struct {
	net.Conn
	owner *HTTP
}

func (c *httpConn) Close() error {
	err := c.Conn.Close()
	c.owner.mu.Lock()
	delete(c.owner.hijacked, c)
	c.owner.mu.Unlock()
	return err
}

func (h *HTTP) publish(err error) {
	h.readyOnce.Do(func() { h.initErr = err; close(h.ready) })
}

func (h *HTTP) WaitReady(ctx context.Context) error {
	select {
	case <-h.ready:
		if err := ctx.Err(); err != nil {
			return err
		}
		return h.initErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *HTTP) Stop(ctx context.Context) error {
	h.stopOnce.Do(func() {
		go func() {
			defer close(h.stopped)
			defer func() {
				if recover() != nil {
					h.stopErr = errors.New("HTTP cleanup panicked")
				}
			}()
			h.stopErr = h.stop(ctx)
		}()
	})
	select {
	case <-h.stopped:
		return h.stopErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *HTTP) stop(ctx context.Context) error {
	// Start retains resources until Stop, including on partial initialization.
	select {
	case <-h.started:
	case <-ctx.Done():
		return ctx.Err()
	}
	h.mu.Lock()
	h.stopping = true
	if h.active == 0 {
		close(h.drained)
	}
	connections := make([]net.Conn, 0, len(h.hijacked))
	for conn := range h.hijacked {
		connections = append(connections, conn)
	}
	h.mu.Unlock()
	for _, conn := range connections {
		_ = conn.Close()
	}
	if h.cancelStreams != nil {
		h.cancelStreams()
	}
	var err error
	if h.server != nil {
		drain, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = h.server.Shutdown(drain)
		cancel()
		if err != nil {
			h.cancelRequests()
			_ = h.server.Close()
		}
		select {
		case <-h.served:
		case <-ctx.Done():
			return errors.Join(err, ctx.Err())
		}
		select {
		case <-h.drained:
		case <-ctx.Done():
			return errors.Join(err, ctx.Err())
		}
	}
	if h.listener != nil {
		_ = h.listener.Close()
	}
	if h.cancelRequests != nil {
		h.cancelRequests()
	}
	if h.auth != nil {
		h.auth.Close()
	}
	if h.publication != nil {
		err = errors.Join(err, h.publication.Close())
	}
	return err
}
