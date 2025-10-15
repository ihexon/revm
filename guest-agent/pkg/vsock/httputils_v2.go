package vsock

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/mdlayher/vsock"
	"github.com/sirupsen/logrus"
)

// Transport defines the interface for different connection types
type Transport interface {
	// DialContext creates a connection using the transport
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
	// BaseURL returns the base URL for this transport
	BaseURL() string
	// Name returns the transport name for logging
	Name() string
}

// UnixSocketTransport implements Transport for Unix sockets
type UnixSocketTransport struct {
	socketPath string
}

func (t *UnixSocketTransport) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{}
	return dialer.DialContext(ctx, "unix", t.socketPath)
}

func (t *UnixSocketTransport) BaseURL() string {
	return "http://unix"
}

func (t *UnixSocketTransport) Name() string {
	return fmt.Sprintf("unix:%s", t.socketPath)
}

// VSockTransport implements Transport for VSock connections
type VSockTransport struct {
	cid  uint32
	port uint32
}

func (t *VSockTransport) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	result := make(chan struct {
		c   net.Conn
		err error
	}, 1)

	go func() {
		c, err := vsock.Dial(t.cid, t.port, nil)
		result <- struct {
			c   net.Conn
			err error
		}{c, err}
	}()

	select {
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	case r := <-result:
		return r.c, r.err
	}
}

func (t *VSockTransport) BaseURL() string {
	return "http://vsock"
}

func (t *VSockTransport) Name() string {
	return fmt.Sprintf("vsock:%d:%d", t.cid, t.port)
}

// HTTPClient provides a unified HTTP client interface
type HTTPClient struct {
	client    *http.Client
	transport Transport
	timeout   time.Duration
}

// ClientConfig holds configuration for HTTPClient
type ClientConfig struct {
	Timeout   time.Duration
	Transport Transport
}

// NewHTTPClient creates a new HTTP client with the specified transport
func NewHTTPClient(config ClientConfig) *HTTPClient {
	if config.Timeout == 0 {
		config.Timeout = 2 * time.Second
	}

	return &HTTPClient{
		transport: config.Transport,
		timeout:   config.Timeout,
		client: &http.Client{
			Timeout: config.Timeout,
			Transport: &http.Transport{
				DialContext: config.Transport.DialContext,
			},
		},
	}
}

// buildURL constructs the full URL for a request
func (c *HTTPClient) buildURL(path string) string {
	return c.transport.BaseURL() + path
}

// Get performs an HTTP GET request
func (c *HTTPClient) Get(ctx context.Context, path string) (*http.Response, error) {
	url := c.buildURL(path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create GET request: %w", err)
	}

	logrus.Debugf("HTTP GET: %s via %s", url, c.transport.Name())
	return c.client.Do(req)
}

// Post performs an HTTP POST request
func (c *HTTPClient) Post(ctx context.Context, path string, contentType string, body io.Reader) (*http.Response, error) {
	url := c.buildURL(path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create POST request: %w", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	logrus.Debugf("HTTP POST: %s via %s", url, c.transport.Name())
	return c.client.Do(req)
}

// GetJSON performs a GET request and returns the response body as bytes
func (c *HTTPClient) GetJSON(ctx context.Context, path string) ([]byte, error) {
	resp, err := c.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		if err := Body.Close(); err != nil {
			logrus.Errorf("failed to close response body: %v", err)
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP GET failed with status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// Close closes the HTTP client
func (c *HTTPClient) Close() error {
	return nil
}

// NewVSockHTTPClientV2 creates an HTTP client for VSock communication
func NewVSockHTTPClientV2(cid, port uint32, timeout time.Duration) *HTTPClient {
	return NewHTTPClient(ClientConfig{
		Transport: &VSockTransport{
			cid:  cid,
			port: port,
		},
		Timeout: timeout,
	})
}
