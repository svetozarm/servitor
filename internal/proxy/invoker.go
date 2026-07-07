package proxy

import "net/http"

// BackendInvoker invokes a backend process with an HTTP request.
type BackendInvoker interface {
	Invoke(req *http.Request) (*http.Response, error)
}
