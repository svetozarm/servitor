package proxy

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// ProxyHandler is an http.Handler that delegates requests to a BackendInvoker.
type ProxyHandler struct {
	Invoker BackendInvoker
	Logger  *slog.Logger
}

func (h *ProxyHandler) log() *slog.Logger {
	if h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.log().Debug("backend invocation start", "method", r.Method, "path", r.URL.Path)
	start := time.Now()

	resp, err := h.Invoker.Invoke(r)
	if err != nil {
		h.log().Error("backend invocation error", "path", r.URL.Path, "err", err)
		var pe *ProxyError
		if errors.As(err, &pe) {
			http.Error(w, pe.Msg, pe.Code)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)

	h.log().Debug("backend invocation end", "path", r.URL.Path, "status", resp.StatusCode, "duration", time.Since(start))
}
