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

// UnixHTTPClient provides HTTP operations over Unix domain sockets
type UnixHTTPClient struct {
	client     *http.Client
	socketPath string
	timeout    time.Duration

	httpHeaders http.Header
	urlValues   url.Values
}

// NewUnixHTTPClient creates a new HTTP client for Unix socket communication
func NewUnixHTTPClient(socketPath string, timeout time.Duration) *UnixHTTPClient {
	if timeout == 0 {
		timeout = 2 * time.Second
	}

	return &UnixHTTPClient{
		socketPath:  socketPath,
		timeout:     timeout,
		httpHeaders: http.Header{},
		urlValues:   url.Values{},
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
	return c.do(ctx, http.MethodGet, path, c.httpHeaders, nil)
}

func (c *UnixHTTPClient) GetWithQuery(ctx context.Context, path string, query url.Values) (*http.Response, error) {
	myURL := "http://unix" + filepath.Clean(filepath.Join("/", path))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, myURL, nil)
	if err != nil {
		return nil, err
	}

	req.URL.RawQuery = query.Encode()

	return c.client.Do(req)
}

// Post performs an HTTP POST request
func (c *UnixHTTPClient) Post(ctx context.Context, path, contentType string, body io.Reader) (*http.Response, error) {
	if contentType != "" {
		c.AddHeader("Content-Type", contentType)
	}
	return c.do(ctx, http.MethodPost, path, c.httpHeaders, body)
}

// Head performs an HTTP HEAD request
func (c *UnixHTTPClient) Head(ctx context.Context, path string) (*http.Response, error) {
	return c.do(ctx, http.MethodHead, path, c.httpHeaders, nil)
}

// do performs the actual HTTP request
func (c *UnixHTTPClient) do(ctx context.Context, method, path string, headers http.Header, body io.Reader) (*http.Response, error) {
	myURL := "http://unix" + filepath.Clean(filepath.Join("/", path))

	req, err := http.NewRequestWithContext(ctx, method, myURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s request: %w", method, err)
	}

	req.Header = headers
	req.URL.RawQuery = c.urlValues.Encode()

	logrus.Debugf("do http request: %q %q", req.Method, req.URL.String())
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP %s request failed: %w", method, err)
	}

	return resp, nil
}

// GetJSON performs a GET request and returns the response body as bytes
func (c *UnixHTTPClient) GetJSON(ctx context.Context, path string) ([]byte, error) {
	c.httpHeaders.Add("Accept", "application/json")
	c.httpHeaders.Add("User-Agent", "revm-httpclient/1.0")

	resp, err := c.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	defer c.CloseResponse(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP GET failed with status: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// PostJSON performs a POST request with JSON content type
func (c *UnixHTTPClient) PostJSON(ctx context.Context, path string, body io.Reader) (*http.Response, error) {
	return c.Post(ctx, path, "application/json", body)
}

// CloseResponse safely closes HTTP response body and logs any errors
func (c *UnixHTTPClient) CloseResponse(resp *http.Response) {
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

func (c *UnixHTTPClient) AddHeader(key, value string) {
	c.httpHeaders.Add(key, value)
}

func (c *UnixHTTPClient) AddQuery(key, value string) {
	c.urlValues.Add(key, value)
}
