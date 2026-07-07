package server

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/svetozarm/servitor/internal/config"
)

// Server serves static files and routes API requests to proxy handlers.
type Server struct {
	cfg     *config.Config
	mux     *http.ServeMux
	httpSrv *http.Server
	logger  *slog.Logger
}

// AppHandler holds the handlers for a single app.
type AppHandler struct {
	Name      string
	Proxy     http.Handler
	Static    http.Handler
	APIPrefix string
}

// New creates a Server that routes to the provided apps.
// An app with Name="" is mounted at root /. Named apps are mounted at /<name>/.
func New(cfg *config.Config, apps []AppHandler, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(nil, nil))
	}
	mux := http.NewServeMux()

	var hasRoot bool
	for _, app := range apps {
		if app.Name == "" {
			hasRoot = true
			mountRootApp(mux, app)
		} else {
			mountNamedApp(mux, app)
		}
	}

	// If no root app, show an index of named apps at /
	if !hasRoot {
		names := make([]string, 0, len(apps))
		for _, app := range apps {
			names = append(names, app.Name)
		}
		mux.HandleFunc("/.servitor/hero.png", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Cache-Control", "public, max-age=86400")
			w.Write(heroPNG)
		})
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			indexTemplate.Execute(w, names)
		})
	}

	s := &Server{
		cfg:    cfg,
		mux:    mux,
		logger: logger,
	}
	s.httpSrv = &http.Server{
		Addr:    cfg.Addr(),
		Handler: s,
	}
	return s
}

func mountRootApp(mux *http.ServeMux, app AppHandler) {
	proxy := app.Proxy
	static := app.Static
	apiPrefix := app.APIPrefix
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, apiPrefix) {
			proxy.ServeHTTP(w, r)
		} else {
			static.ServeHTTP(w, r)
		}
	})
}

func mountNamedApp(mux *http.ServeMux, app AppHandler) {
	prefix := "/" + app.Name + "/"
	apiPrefix := prefix + strings.TrimPrefix(app.APIPrefix, "/")
	proxy := app.Proxy
	static := app.Static
	mux.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/" + strings.TrimPrefix(r.URL.Path, prefix)
		r2.RequestURI = r2.URL.Path
		if r.URL.RawQuery != "" {
			r2.RequestURI += "?" + r.URL.RawQuery
		}
		if strings.HasPrefix(r.URL.Path, apiPrefix) {
			proxy.ServeHTTP(w, r2)
		} else {
			static.ServeHTTP(w, r2)
		}
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.status = code
	rr.ResponseWriter.WriteHeader(code)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rec := &responseRecorder{ResponseWriter: w, status: 200}
	s.mux.ServeHTTP(rec, r)
	s.logger.Debug("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration", time.Since(start))
}

// Start listens and serves. It blocks until the server is shut down via Shutdown.
func (s *Server) Start(_ context.Context) error {
	err := s.httpSrv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}


