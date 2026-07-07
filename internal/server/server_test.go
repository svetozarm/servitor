package server_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/svetozarm/servitor/internal/config"
	"github.com/svetozarm/servitor/internal/server"
)

func testConfig(frontendPath string, port int) *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Host: "127.0.0.1",
			Port: port,
		},
		Frontend: config.FrontendConfig{
			Path: frontendPath,
		},
		Backend: config.BackendConfig{
			APIPrefix: "/api/",
		},
	}
}

func startServer(t *testing.T, cfg *config.Config) (string, func()) {
	t.Helper()
	placeholder := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not implemented", http.StatusNotImplemented)
	})
	srv := server.New(cfg, []server.AppHandler{{
		Name:      "",
		Proxy:     placeholder,
		Static:    http.FileServer(http.Dir(cfg.Frontend.Path)),
		APIPrefix: cfg.Backend.APIPrefix,
	}}, nil)
	ctx := context.Background()
	done := make(chan error, 1)
	go func() { done <- srv.Start(ctx) }()

	// Wait for server to be ready
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

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func TestServeStaticFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.html"), []byte("<h1>hello</h1>"), 0644)

	cfg := testConfig(dir, freePort(t))
	base, cleanup := startServer(t, cfg)
	defer cleanup()

	resp, err := http.Get(base + "/test.html")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "<h1>hello</h1>" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestServeIndexOnRoot(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("index"), 0644)

	cfg := testConfig(dir, freePort(t))
	base, cleanup := startServer(t, cfg)
	defer cleanup()

	resp, err := http.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "index" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestMissingFileReturns404(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(dir, freePort(t))
	base, cleanup := startServer(t, cfg)
	defer cleanup()

	resp, err := http.Get(base + "/nope.html")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestContentType(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "data.json"), []byte(`{"ok":true}`), 0644)

	cfg := testConfig(dir, freePort(t))
	base, cleanup := startServer(t, cfg)
	defer cleanup()

	resp, err := http.Get(base + "/data.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected application/json, got %s", ct)
	}
}

func TestNestedPaths(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "js"), 0755)
	os.WriteFile(filepath.Join(dir, "js", "app.js"), []byte("var x=1"), 0644)

	cfg := testConfig(dir, freePort(t))
	base, cleanup := startServer(t, cfg)
	defer cleanup()

	resp, err := http.Get(base + "/js/app.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "var x=1" {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestAPIPrefixRoutesToProxy(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(dir, freePort(t))
	base, cleanup := startServer(t, cfg)
	defer cleanup()

	resp, err := http.Get(base + "/api/test")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 501 {
		t.Fatalf("expected 501, got %d", resp.StatusCode)
	}
}

func TestFileServedFreshFromDisk(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "mutable.txt")
	os.WriteFile(file, []byte("v1"), 0644)

	cfg := testConfig(dir, freePort(t))
	base, cleanup := startServer(t, cfg)
	defer cleanup()

	resp, err := http.Get(base + "/mutable.txt")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "v1" {
		t.Fatalf("expected v1, got %s", body)
	}

	os.WriteFile(file, []byte("v2"), 0644)

	resp, err = http.Get(base + "/mutable.txt")
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "v2" {
		t.Fatalf("expected v2, got %s", body)
	}
}

func TestLocalhostBinding(t *testing.T) {
	dir := t.TempDir()
	cfg := testConfig(dir, freePort(t))
	_, cleanup := startServer(t, cfg)
	defer cleanup()

	conn, err := net.DialTimeout("tcp", cfg.Addr(), time.Second)
	if err != nil {
		t.Fatalf("expected connection to 127.0.0.1 to succeed: %v", err)
	}
	conn.Close()
}
