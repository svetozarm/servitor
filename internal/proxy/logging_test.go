package proxy

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyHandler_LogsInvocationSuccess(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	handler := &ProxyHandler{
		Invoker: &mockInvoker{
			resp: &http.Response{
				StatusCode: 200,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader("ok")),
			},
		},
		Logger: logger,
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	handler.ServeHTTP(w, r)

	log := buf.String()
	if !strings.Contains(log, "backend invocation start") {
		t.Errorf("expected start log, got: %s", log)
	}
	if !strings.Contains(log, "backend invocation end") {
		t.Errorf("expected end log, got: %s", log)
	}
	if !strings.Contains(log, "path=/api/test") {
		t.Errorf("expected path in log, got: %s", log)
	}
	if !strings.Contains(log, "duration=") {
		t.Errorf("expected duration in log, got: %s", log)
	}
}

func TestProxyHandler_LogsInvocationError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	handler := &ProxyHandler{
		Invoker: &mockInvoker{err: &ProxyError{Code: 502, Msg: "backend crashed: some stderr"}},
		Logger:  logger,
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	handler.ServeHTTP(w, r)

	log := buf.String()
	if !strings.Contains(log, "backend invocation error") {
		t.Errorf("expected error log, got: %s", log)
	}
	if !strings.Contains(log, "backend crashed: some stderr") {
		t.Errorf("expected error message in log, got: %s", log)
	}
}

func TestProxyHandler_NilLoggerNosPanic(t *testing.T) {
	handler := &ProxyHandler{
		Invoker: &mockInvoker{err: errors.New("fail")},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	handler.ServeHTTP(w, r) // should not panic
	if w.Code != 500 {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
