package network

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"path"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	defaultTimeout         = 2 * time.Second
	defaultIdleConnTimeout = 1 * time.Second
)

// Client provides HTTP operations over a fixed transport.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewTCPClient creates an HTTP client that dials addr over TCP.
func NewTCPClient(addr string) *Client {
	dialFunc := func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: defaultTimeout}).DialContext(ctx, "tcp", addr)
	}

	return &Client{
		baseURL: "http://" + addr,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
			Transport: &http.Transport{
				DialContext:           dialFunc,
				DisableKeepAlives:     true,
				MaxIdleConnsPerHost:   1,
				IdleConnTimeout:       defaultIdleConnTimeout,
				ResponseHeaderTimeout: defaultTimeout,
			},
		},
	}
}

// Close releases idle connections held by the client.
func (c *Client) Close() error {
	if t, ok := c.httpClient.Transport.(*http.Transport); ok {
		t.CloseIdleConnections()
	}
	return nil
}

// Request is a fluent HTTP request builder.
type Request struct {
	client *Client
	method string
	path   string
}

// NewRequest returns a request builder for the given method and path.
func (c *Client) NewRequest(method, path string) *Request {
	return &Request{client: c, method: method, path: path}
}

// Do executes the request and returns the HTTP response.
func (r *Request) Do(ctx context.Context) (*http.Response, error) {
	url := r.client.baseURL + path.Clean("/"+r.path)
	req, err := http.NewRequestWithContext(ctx, r.method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := r.client.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP %s %s: %w", r.method, url, err)
	}
	return resp, nil
}

// CloseResponse drains and closes an HTTP response body safely.
func CloseResponse(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		if err := resp.Body.Close(); err != nil {
			logrus.Debugf("close response body: %v", err)
		}
	}
}
