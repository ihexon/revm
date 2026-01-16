package network

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

// UnixHTTPClient provides HTTP operations over Unix domain sockets
type UnixHTTPClient struct {
	client     *http.Client
	socketPath string
	timeout    time.Duration
}

// NewUnixHTTPClient creates a new HTTP client for Unix socket communication
func NewUnixHTTPClient(socketPath string, timeout time.Duration) *UnixHTTPClient {
	if timeout == 0 {
		timeout = 2 * time.Second
	}

	return &UnixHTTPClient{
		socketPath: socketPath,
		timeout:    timeout,
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					dialer := &net.Dialer{
						Timeout: timeout,
					}
					return dialer.DialContext(ctx, "unix", socketPath)
				},
				DisableKeepAlives:     true, // Ensure connections are closed immediately
				MaxIdleConnsPerHost:   1,
				IdleConnTimeout:       1 * time.Second,
				ResponseHeaderTimeout: timeout,
			},
		},
	}
}

// Get performs an HTTP GET request
func (c *UnixHTTPClient) Get(ctx context.Context, path string) (*http.Response, error) {
	return c.do(ctx, http.MethodGet, path, "", nil)
}

// Post performs an HTTP POST request
func (c *UnixHTTPClient) Post(ctx context.Context, path, contentType string, body io.Reader) (*http.Response, error) {
	return c.do(ctx, http.MethodPost, path, contentType, body)
}

// Head performs an HTTP HEAD request
func (c *UnixHTTPClient) Head(ctx context.Context, path string) (*http.Response, error) {
	return c.do(ctx, http.MethodHead, path, "", nil)
}

// do performs the actual HTTP request
func (c *UnixHTTPClient) do(ctx context.Context, method, path, contentType string, body io.Reader) (*http.Response, error) {
	url := "http://unix" + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s request: %w", method, err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	logrus.Infof("HTTP %s: %s via unix:%s", method, url, c.socketPath)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP %s request failed: %w", method, err)
	}

	return resp, nil
}

// GetJSON performs a GET request and returns the response body as bytes
func (c *UnixHTTPClient) GetJSON(ctx context.Context, path string) ([]byte, error) {
	resp, err := c.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer c.closeResponse(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP GET failed with status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// PostJSON performs a POST request with JSON content type
func (c *UnixHTTPClient) PostJSON(ctx context.Context, path string, body io.Reader) (*http.Response, error) {
	return c.Post(ctx, path, "application/json", body)
}

// closeResponse safely closes HTTP response body and logs any errors
func (c *UnixHTTPClient) closeResponse(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		// Drain and close the response body to ensure connection reuse
		_, _ = io.Copy(io.Discard, resp.Body)
		if err := resp.Body.Close(); err != nil {
			logrus.Infof("failed to close response body: %v", err)
		}
	}
}

// Close closes the HTTP client and cleans up resources
func (c *UnixHTTPClient) Close() error {
	if transport, ok := c.client.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
	return nil
}
