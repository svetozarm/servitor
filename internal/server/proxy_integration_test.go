package server_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/svetozarm/servitor/internal/config"
	"github.com/svetozarm/servitor/internal/proxy"
	"github.com/svetozarm/servitor/internal/server"
)

func startServerWithProxy(t *testing.T, cfg *config.Config, binaryPath string, timeout time.Duration) (string, func()) {
	t.Helper()
	invoker := &proxy.CGIInvoker{
		BinaryPath: binaryPath,
		Timeout:    timeout,
	}
	handler := &proxy.ProxyHandler{Invoker: invoker}
	srv := server.New(cfg, []server.AppHandler{{
		Name:      "",
		Proxy:     handler,
		Static:    http.FileServer(http.Dir(cfg.Frontend.Path)),
		APIPrefix: cfg.Backend.APIPrefix,
	}}, nil)

	ctx := context.Background()
	done := make(chan error, 1)
	go func() { done <- srv.Start(ctx) }()

	addr := cfg.Addr()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	return "http://" + addr, func() {
		srv.Shutdown(context.Background())
		<-done
	}
}

func writeScript(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
	return p
}

func proxyCfg(t *testing.T, frontendDir, backendPath string, timeout time.Duration) *config.Config {
	t.Helper()
	return &config.Config{
		Server:   config.ServerConfig{Host: "127.0.0.1", Port: freePort(t)},
		Frontend: config.FrontendConfig{Path: frontendDir},
		Backend:  config.BackendConfig{Path: backendPath, APIPrefix: "/api/", Timeout: config.Duration{Duration: timeout}},
	}
}

func TestIntegration_FullRequestThroughProxy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash scripts not available on windows")
	}

	dir := t.TempDir()
	frontendDir := t.TempDir()
	os.WriteFile(filepath.Join(frontendDir, "index.html"), []byte("static"), 0644)

	t.Run("GET returns JSON from backend", func(t *testing.T) {
		script := writeScript(t, dir, "get-backend.sh",
			"#!/bin/sh\ncat > /dev/null\nprintf 'HTTP/1.1 200 OK\\r\\nContent-Type: application/json\\r\\n\\r\\n{\"status\":\"ok\"}'\n")
		cfg := proxyCfg(t, frontendDir, script, 5*time.Second)
		base, cleanup := startServerWithProxy(t, cfg, script, 5*time.Second)
		defer cleanup()

		resp, err := http.Get(base + "/api/health")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != `{"status":"ok"}` {
			t.Fatalf("unexpected body: %s", body)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("expected application/json, got %s", ct)
		}
	})

	t.Run("POST body piped to backend", func(t *testing.T) {
		// This backend reads stdin and echoes the body length back
		script := writeScript(t, dir, "post-backend.sh",
			"#!/bin/sh\nBODY=$(cat)\nLEN=${#BODY}\nprintf \"HTTP/1.1 200 OK\\r\\nContent-Type: text/plain\\r\\n\\r\\nreceived:%d\" \"$LEN\"\n")
		cfg := proxyCfg(t, frontendDir, script, 5*time.Second)
		base, cleanup := startServerWithProxy(t, cfg, script, 5*time.Second)
		defer cleanup()

		resp, err := http.Post(base+"/api/data", "application/json", strings.NewReader(`{"key":"value"}`))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		// The request includes headers so body length will be larger than just the JSON
		if !strings.HasPrefix(string(body), "received:") {
			t.Fatalf("unexpected body: %s", body)
		}
	})

	t.Run("backend crash returns 502", func(t *testing.T) {
		script := writeScript(t, dir, "crash-backend.sh",
			"#!/bin/sh\nexit 1\n")
		cfg := proxyCfg(t, frontendDir, script, 5*time.Second)
		base, cleanup := startServerWithProxy(t, cfg, script, 5*time.Second)
		defer cleanup()

		resp, err := http.Get(base + "/api/crash")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 502 {
			t.Fatalf("expected 502, got %d", resp.StatusCode)
		}
	})

	t.Run("backend timeout returns 504", func(t *testing.T) {
		script := writeScript(t, dir, "slow-backend.sh",
			"#!/bin/sh\nexec sleep 60\n")
		cfg := proxyCfg(t, frontendDir, script, 100*time.Millisecond)
		base, cleanup := startServerWithProxy(t, cfg, script, 100*time.Millisecond)
		defer cleanup()

		resp, err := http.Get(base + "/api/slow")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 504 {
			t.Fatalf("expected 504, got %d", resp.StatusCode)
		}
	})

	t.Run("custom response headers preserved", func(t *testing.T) {
		script := writeScript(t, dir, "headers-backend.sh",
			"#!/bin/sh\ncat > /dev/null\nprintf 'HTTP/1.1 200 OK\\r\\nX-Custom: hello\\r\\nContent-Type: text/plain\\r\\n\\r\\nok'\n")
		cfg := proxyCfg(t, frontendDir, script, 5*time.Second)
		base, cleanup := startServerWithProxy(t, cfg, script, 5*time.Second)
		defer cleanup()

		resp, err := http.Get(base + "/api/headers")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		if v := resp.Header.Get("X-Custom"); v != "hello" {
			t.Fatalf("expected X-Custom: hello, got %q", v)
		}
	})

	t.Run("static files unaffected by backend errors", func(t *testing.T) {
		script := writeScript(t, dir, "broken-backend.sh",
			"#!/bin/sh\nexit 1\n")
		cfg := proxyCfg(t, frontendDir, script, 5*time.Second)
		base, cleanup := startServerWithProxy(t, cfg, script, 5*time.Second)
		defer cleanup()

		// Backend fails
		resp, err := http.Get(base + "/api/fail")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 502 {
			t.Fatalf("expected 502, got %d", resp.StatusCode)
		}

		// Static still works
		resp, err = http.Get(base + "/index.html")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "static" {
			t.Fatalf("unexpected body: %s", body)
		}
	})
}
