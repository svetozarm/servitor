package proxy

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// ProxyError represents a proxy-layer error with an HTTP status code.
type ProxyError struct {
	Code int
	Msg  string
}

func (e *ProxyError) Error() string { return e.Msg }

// CGIInvoker spawns a backend binary per request, piping the HTTP request
// via stdin and reading the HTTP response from stdout.
type CGIInvoker struct {
	BinaryPath string
	Timeout    time.Duration
	WorkDir    string
}

func (c *CGIInvoker) Invoke(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(req.Context(), c.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.BinaryPath)
	if c.WorkDir != "" {
		cmd.Dir = c.WorkDir
	}

	var reqBuf bytes.Buffer
	if err := req.Write(&reqBuf); err != nil {
		return nil, &ProxyError{Code: 502, Msg: fmt.Sprintf("failed to serialize request: %v", err)}
	}
	cmd.Stdin = &reqBuf

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, &ProxyError{Code: 504, Msg: "backend timeout"}
		}
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return nil, &ProxyError{Code: 502, Msg: "backend binary not found"}
		}
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			return nil, &ProxyError{Code: 502, Msg: "backend binary not found"}
		}
		return nil, &ProxyError{Code: 502, Msg: fmt.Sprintf("backend crashed: %s", stderr.String())}
	}

	resp, err := http.ReadResponse(bufio.NewReader(&stdout), req)
	if err != nil {
		return nil, &ProxyError{Code: 502, Msg: fmt.Sprintf("failed to parse backend response: %v", err)}
	}
	return resp, nil
}
