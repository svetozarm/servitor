package proxy

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type mockInvoker struct {
	resp *http.Response
	err  error
}

func (m *mockInvoker) Invoke(req *http.Request) (*http.Response, error) {
	return m.resp, m.err
}

func TestProxyHandler_Success(t *testing.T) {
	handler := &ProxyHandler{
		Invoker: &mockInvoker{
			resp: &http.Response{
				StatusCode: 200,
				Header:     http.Header{"Content-Type": {"application/json"}, "X-Custom": {"val"}},
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			},
		},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	handler.ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected application/json, got %s", w.Header().Get("Content-Type"))
	}
	if w.Header().Get("X-Custom") != "val" {
		t.Fatalf("expected X-Custom: val, got %s", w.Header().Get("X-Custom"))
	}
	if w.Body.String() != `{"ok":true}` {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestProxyHandler_ProxyError502(t *testing.T) {
	handler := &ProxyHandler{
		Invoker: &mockInvoker{err: &ProxyError{Code: 502, Msg: "backend crashed"}},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	handler.ServeHTTP(w, r)

	if w.Code != 502 {
		t.Fatalf("expected 502, got %d", w.Code)
	}
}

func TestProxyHandler_ProxyError504(t *testing.T) {
	handler := &ProxyHandler{
		Invoker: &mockInvoker{err: &ProxyError{Code: 504, Msg: "backend timeout"}},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	handler.ServeHTTP(w, r)

	if w.Code != 504 {
		t.Fatalf("expected 504, got %d", w.Code)
	}
}

func TestProxyHandler_UnknownError(t *testing.T) {
	handler := &ProxyHandler{
		Invoker: &mockInvoker{err: errors.New("something unexpected")},
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	handler.ServeHTTP(w, r)

	if w.Code != 500 {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
