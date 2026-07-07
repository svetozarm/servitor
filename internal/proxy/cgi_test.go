package proxy

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func writeScript(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if runtime.GOOS == "windows" {
		t.Skip("tests require unix shell scripts")
	}
	err := os.WriteFile(path, []byte("#!/bin/sh\n"+content), 0o755)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCGIInvoker_SuccessfulRoundTrip(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "echo.sh",
		`cat > /dev/null
printf "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"ok\":true}"`)

	invoker := &CGIInvoker{BinaryPath: script, Timeout: 5 * time.Second}
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)

	resp, err := invoker.Invoke(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("expected application/json, got %s", resp.Header.Get("Content-Type"))
	}
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	if buf.String() != `{"ok":true}` {
		t.Fatalf("unexpected body: %s", buf.String())
	}
}

func TestCGIInvoker_PostWithBody(t *testing.T) {
	dir := t.TempDir()
	// Backend reads all of stdin (raw HTTP request) and returns it in the response body
	// so we can verify the body was included in the serialized request.
	script := writeScript(t, dir, "echo-body.sh",
		`INPUT=$(cat)
LEN=$(printf '%s' "$INPUT" | wc -c)
printf "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s" "$LEN" "$INPUT"`)

	invoker := &CGIInvoker{BinaryPath: script, Timeout: 5 * time.Second}
	body := `{"name":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/data", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := invoker.Invoke(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	// The response body contains the full raw HTTP request; verify body is included
	if !bytes.Contains(buf.Bytes(), []byte(body)) {
		t.Fatalf("expected body to contain %q, got %q", body, buf.String())
	}
}

func TestCGIInvoker_NonZeroExit(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "crash.sh", `echo "oops" >&2; exit 1`)

	invoker := &CGIInvoker{BinaryPath: script, Timeout: 5 * time.Second}
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)

	_, err := invoker.Invoke(req)
	if err == nil {
		t.Fatal("expected error")
	}
	pe, ok := err.(*ProxyError)
	if !ok {
		t.Fatalf("expected ProxyError, got %T", err)
	}
	if pe.Code != 502 {
		t.Fatalf("expected 502, got %d", pe.Code)
	}
}

func TestCGIInvoker_Timeout(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "slow.sh", `exec sleep 60`)

	invoker := &CGIInvoker{BinaryPath: script, Timeout: 100 * time.Millisecond}
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)

	_, err := invoker.Invoke(req)
	if err == nil {
		t.Fatal("expected error")
	}
	pe, ok := err.(*ProxyError)
	if !ok {
		t.Fatalf("expected ProxyError, got %T", err)
	}
	if pe.Code != 504 {
		t.Fatalf("expected 504, got %d", pe.Code)
	}
}

func TestCGIInvoker_MissingBinary(t *testing.T) {
	invoker := &CGIInvoker{BinaryPath: "/nonexistent/binary", Timeout: 5 * time.Second}
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)

	_, err := invoker.Invoke(req)
	if err == nil {
		t.Fatal("expected error")
	}
	pe, ok := err.(*ProxyError)
	if !ok {
		t.Fatalf("expected ProxyError, got %T", err)
	}
	if pe.Code != 502 {
		t.Fatalf("expected 502, got %d", pe.Code)
	}
}
