// Package api exposes Workbench's operations as a small local HTTP API for
// agents and scripts. It is intentionally headless: the TUI and the API share
// the same operation registry and runner.
package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/solomonadzape95/workbench/internal/config"
	"github.com/solomonadzape95/workbench/internal/core"
	"github.com/solomonadzape95/workbench/internal/engines"
	"github.com/solomonadzape95/workbench/internal/history"
	"github.com/solomonadzape95/workbench/internal/pipeline"
	"github.com/solomonadzape95/workbench/internal/runner"
)

type Options struct {
	Config     config.Config
	Operations *core.Registry
	Engines    *engines.Registry
	Runner     *runner.Runner
	Pipelines  *pipeline.Store
	History    *history.Store
	Address    string
	Token      string
	Version    string
}

type Server struct {
	cfg       config.Config
	ops       *core.Registry
	engines   *engines.Registry
	runner    *runner.Runner
	pipelines *pipeline.Store
	history   *history.Store
	address   string
	token     string
	version   string
	started   time.Time
	handler   http.Handler
}

func New(opts Options) *Server {
	address := strings.TrimSpace(opts.Address)
	if address == "" {
		address = "127.0.0.1:8765"
	}
	s := &Server{
		cfg:       opts.Config,
		ops:       opts.Operations,
		engines:   opts.Engines,
		runner:    opts.Runner,
		pipelines: opts.Pipelines,
		history:   opts.History,
		address:   address,
		token:     strings.TrimSpace(opts.Token),
		version:   opts.Version,
		started:   time.Now(),
	}
	s.handler = s.routes()
	return s
}

func (s *Server) Address() string     { return s.address }
func (s *Server) TokenRequired() bool { return s.token != "" }

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /v1", s.index)
	mux.HandleFunc("GET /v1/operations", s.listOperations)
	mux.HandleFunc("GET /v1/operations/{id}", s.getOperation)
	mux.HandleFunc("GET /v1/engines", s.listEngines)
	mux.HandleFunc("GET /v1/pipelines", s.listPipelines)
	mux.HandleFunc("GET /v1/pipelines/{name}", s.getPipeline)
	mux.HandleFunc("POST /v1/jobs", s.createJob)
	mux.HandleFunc("GET /v1/jobs", s.listJobs)
	mux.HandleFunc("GET /v1/jobs/{id}", s.getJob)
	mux.HandleFunc("GET /v1/jobs/{id}/events", s.jobEvents)
	mux.HandleFunc("GET /v1/jobs/{id}/artifacts", s.listArtifacts)
	mux.HandleFunc("GET /v1/jobs/{id}/artifacts/{n}", s.getArtifact)
	mux.HandleFunc("POST /v1/jobs/{id}/cancel", s.cancelJob)
	mux.HandleFunc("POST /v1/jobs/{id}/retry", s.retryJob)
	mux.HandleFunc("POST /v1/jobs/{id}/rerun", s.rerunJob)
	return recoverMiddleware(authMiddleware(s.token, mux))
}

// ListenAndServe runs until ctx is cancelled, then shuts the server down
// gracefully. Binding errors are returned immediately.
func (s *Server) ListenAndServe(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.address,
		Handler:           s.handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			return err
		}
		return nil
	}
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"version":        s.version,
		"uptime_seconds": int(time.Since(s.started).Seconds()),
	})
}

func (s *Server) index(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":    "workbench",
		"version": s.version,
		"endpoints": []string{
			"GET /healthz",
			"GET /v1/operations",
			"GET /v1/operations/{id}",
			"GET /v1/engines",
			"GET /v1/pipelines",
			"GET /v1/pipelines/{name}",
			"POST /v1/jobs",
			"GET /v1/jobs",
			"GET /v1/jobs/{id}?wait=30",
			"GET /v1/jobs/{id}/events",
			"GET /v1/jobs/{id}/artifacts",
			"GET /v1/jobs/{id}/artifacts/{n}",
			"POST /v1/jobs/{id}/cancel",
			"POST /v1/jobs/{id}/retry",
			"POST /v1/jobs/{id}/rerun",
		},
	})
}

func authMiddleware(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		got := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(got), "bearer ") {
			got = strings.TrimSpace(got[7:])
		} else {
			got = strings.TrimSpace(r.Header.Get("X-Workbench-Token"))
		}
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="workbench"`)
			writeError(w, http.StatusUnauthorized, core.NewError(core.CodeUsage, "", "",
				"missing or invalid API token", "send Authorization: Bearer <token>"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				log.Printf("api panic: %v", p)
				writeError(w, http.StatusInternalServerError, core.NewError(core.CodeInternal, "", "",
					fmt.Sprintf("internal server error: %v", p), "check the Workbench log"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
