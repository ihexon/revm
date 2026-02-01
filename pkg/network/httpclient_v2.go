package network

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
)

// Default configuration values
const (
	defaultTimeout         = 2 * time.Second
	defaultIdleConnTimeout = 1 * time.Second
)

// clientConfig holds internal configuration
type clientConfig struct {
	timeout             time.Duration
	idleConnTimeout     time.Duration
	disableKeepAlives   bool
	maxIdleConnsPerHost int
}

// ClientOption configures a Client
type ClientOption func(*clientConfig)

// WithTimeout sets the request timeout
func WithTimeout(d time.Duration) ClientOption {
	return func(c *clientConfig) {
		c.timeout = d
	}
}

// WithIdleConnTimeout sets the idle connection timeout
func WithIdleConnTimeout(d time.Duration) ClientOption {
	return func(c *clientConfig) {
		c.idleConnTimeout = d
	}
}

// WithKeepAlive enables or disables HTTP keep-alive
func WithKeepAlive(enabled bool) ClientOption {
	return func(c *clientConfig) {
		c.disableKeepAlives = !enabled
	}
}

// WithMaxIdleConns sets the maximum idle connections per host
func WithMaxIdleConns(n int) ClientOption {
	return func(c *clientConfig) {
		c.maxIdleConnsPerHost = n
	}
}

// Client provides HTTP operations over various transports.
// It is immutable after creation and safe for concurrent use.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

func defaultConfig() *clientConfig {
	return &clientConfig{
		timeout:             defaultTimeout,
		idleConnTimeout:     defaultIdleConnTimeout,
		disableKeepAlives:   true,
		maxIdleConnsPerHost: 1,
	}
}

func applyOptions(cfg *clientConfig, opts []ClientOption) {
	for _, opt := range opts {
		opt(cfg)
	}
}

func newClient(baseURL string, dialFunc func(ctx context.Context, network, addr string) (net.Conn, error), cfg *clientConfig) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: cfg.timeout,
			Transport: &http.Transport{
				DialContext:           dialFunc,
				DisableKeepAlives:     cfg.disableKeepAlives,
				MaxIdleConnsPerHost:   cfg.maxIdleConnsPerHost,
				IdleConnTimeout:       cfg.idleConnTimeout,
				ResponseHeaderTimeout: cfg.timeout,
			},
		},
	}
}

// NewUnixClient creates a new HTTP client for Unix socket communication.
func NewUnixClient(socketPath string, opts ...ClientOption) *Client {
	cfg := defaultConfig()
	applyOptions(cfg, opts)

	dialFunc := func(ctx context.Context, _, _ string) (net.Conn, error) {
		dialer := &net.Dialer{Timeout: cfg.timeout}
		return dialer.DialContext(ctx, "unix", socketPath)
	}

	return newClient("http://unix", dialFunc, cfg)
}

// NewTCPClient creates a new HTTP client for TCP communication.
func NewTCPClient(addr string, opts ...ClientOption) *Client {
	cfg := defaultConfig()
	applyOptions(cfg, opts)

	dialFunc := func(ctx context.Context, _, _ string) (net.Conn, error) {
		dialer := &net.Dialer{Timeout: cfg.timeout}
		return dialer.DialContext(ctx, "tcp", addr)
	}

	baseURL := "http://" + addr
	return newClient(baseURL, dialFunc, cfg)
}

// Close closes the HTTP client and cleans up resources
func (c *Client) Close() error {
	if transport, ok := c.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
	return nil
}

// Request represents an HTTP request being built.
// Methods return the request for chaining.
type Request struct {
	client  *Client
	method  string
	path    string
	headers http.Header
	query   url.Values
	body    io.Reader
}

// NewRequest creates a new request builder
func (c *Client) NewRequest(method, path string) *Request {
	return &Request{
		client:  c,
		method:  method,
		path:    path,
		headers: make(http.Header),
		query:   make(url.Values),
	}
}

// Get creates a GET request builder
func (c *Client) Get(path string) *Request {
	return c.NewRequest(http.MethodGet, path)
}

// Post creates a POST request builder
func (c *Client) Post(path string) *Request {
	return c.NewRequest(http.MethodPost, path)
}

// Put creates a PUT request builder
func (c *Client) Put(path string) *Request {
	return c.NewRequest(http.MethodPut, path)
}

// Delete creates a DELETE request builder
func (c *Client) Delete(path string) *Request {
	return c.NewRequest(http.MethodDelete, path)
}

// Head creates a HEAD request builder
func (c *Client) Head(path string) *Request {
	return c.NewRequest(http.MethodHead, path)
}

// Header adds a header to the request
func (r *Request) Header(key, value string) *Request {
	r.headers.Set(key, value)
	return r
}

// Query adds a query parameter to the request
func (r *Request) Query(key, value string) *Request {
	r.query.Set(key, value)
	return r
}

// Body sets the request body
func (r *Request) Body(body io.Reader) *Request {
	r.body = body
	return r
}

// JSON sets Content-Type and Accept headers to application/json
func (r *Request) JSON() *Request {
	return r.Header("Content-Type", "application/json").Header("Accept", "application/json")
}

// buildURL constructs the full URL for the request
func (r *Request) buildURL() string {
	// For Unix sockets, use path directly
	if r.client.baseURL == "http://unix" {
		return r.client.baseURL + filepath.Clean(filepath.Join("/", r.path))
	}
	// For TCP, append path to base URL
	return r.client.baseURL + filepath.Clean(filepath.Join("/", r.path))
}

// Do executes the request and returns the response
func (r *Request) Do(ctx context.Context) (*http.Response, error) {
	reqURL := r.buildURL()

	req, err := http.NewRequestWithContext(ctx, r.method, reqURL, r.body)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s request: %w", r.method, err)
	}

	req.Header = r.headers
	if len(r.query) > 0 {
		req.URL.RawQuery = r.query.Encode()
	}

	logrus.Debugf("http request: %s %s", req.Method, req.URL.String())

	resp, err := r.client.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP %s request failed: %w", r.method, err)
	}

	return resp, nil
}

// DoAndRead executes the request, reads the body, and closes the response
func (r *Request) DoAndRead(ctx context.Context) ([]byte, int, error) {
	resp, err := r.Do(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer CloseResponse(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, resp.StatusCode, nil
}

// CloseResponse safely closes HTTP response body
func CloseResponse(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		if err := resp.Body.Close(); err != nil {
			logrus.Debugf("failed to close response body: %v", err)
		}
	}
}
