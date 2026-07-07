package server_test

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/svetozarm/servitor/internal/server"
)

func TestRequestLogging(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("ok"), 0644)

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg := testConfig(dir, freePort(t))
	proxy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	srv := server.New(cfg, []server.AppHandler{{
		Name:      "",
		Proxy:     proxy,
		Static:    http.FileServer(http.Dir(dir)),
		APIPrefix: cfg.Backend.APIPrefix,
	}}, logger)

	done := make(chan error, 1)
	go func() { done <- srv.Start(context.Background()) }()

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

	http.Get("http://" + addr + "/index.html")

	srv.Shutdown(context.Background())
	<-done

	log := buf.String()
	if !strings.Contains(log, "method=GET") {
		t.Errorf("expected method in log, got: %s", log)
	}
	if !strings.Contains(log, "path=/index.html") {
		t.Errorf("expected path in log, got: %s", log)
	}
	if !strings.Contains(log, "status=200") {
		t.Errorf("expected status in log, got: %s", log)
	}
	if !strings.Contains(log, "duration=") {
		t.Errorf("expected duration in log, got: %s", log)
	}
}
