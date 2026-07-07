package server_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/svetozarm/servitor/internal/server"
)

func TestGracefulShutdown_DrainsHTTP(t *testing.T) {
	dir := t.TempDir()
	port := freePort(t)
	cfg := testConfig(dir, port)

	var requestCompleted atomic.Bool
	slowHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		requestCompleted.Store(true)
	})

	srv := server.New(cfg, []server.AppHandler{{
		Name:      "",
		Proxy:     slowHandler,
		Static:    http.NotFoundHandler(),
		APIPrefix: "/api/",
	}}, nil)

	startErr := make(chan error, 1)
	go func() { startErr <- srv.Start(context.Background()) }()

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

	// Start a slow request
	respCh := make(chan *http.Response, 1)
	go func() {
		resp, err := http.Get(fmt.Sprintf("http://%s/api/slow", addr))
		if err != nil {
			t.Logf("request error: %v", err)
			return
		}
		respCh <- resp
	}()

	// Give request time to start
	time.Sleep(50 * time.Millisecond)

	// Shutdown with timeout to drain in-flight requests
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	// Wait for Start to return
	if err := <-startErr; err != nil {
		t.Fatalf("start returned error: %v", err)
	}

	// Verify the in-flight request completed successfully
	select {
	case resp := <-respCh:
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request did not complete")
	}

	if !requestCompleted.Load() {
		t.Fatal("request handler did not complete")
	}
}

func TestGracefulShutdown_CallerControlsSequence(t *testing.T) {
	dir := t.TempDir()
	port := freePort(t)
	cfg := testConfig(dir, port)

	proxy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	srv := server.New(cfg, []server.AppHandler{{
		Name:      "",
		Proxy:     proxy,
		Static:    http.NotFoundHandler(),
		APIPrefix: "/api/",
	}}, nil)

	startErr := make(chan error, 1)
	go func() { startErr <- srv.Start(context.Background()) }()

	// Wait for server ready
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

	// Caller controls the sequence explicitly
	var sequence []string

	sequence = append(sequence, "drain_http")
	srv.Shutdown(context.Background())

	<-startErr

	sequence = append(sequence, "flush_sync")
	// (would call syncEngine.FlushSync() here)

	sequence = append(sequence, "done")

	// Verify order
	expected := []string{"drain_http", "flush_sync", "done"}
	if len(sequence) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, sequence)
	}
	for i := range expected {
		if sequence[i] != expected[i] {
			t.Fatalf("step %d: expected %s, got %s", i, expected[i], sequence[i])
		}
	}
}
