package ssh

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/containers/gvisor-tap-vsock/pkg/transport"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

var (
	// ErrClientClosed is returned when operations are attempted on a closed client
	ErrClientClosed = errors.New("SSH client is closed")
	// ErrConnectionFailed is returned when the SSH connection cannot be established
	ErrConnectionFailed = errors.New("failed to establish SSH connection")
	// ErrAuthenticationFailed is returned when SSH authentication fails
	ErrAuthenticationFailed = errors.New("SSH authentication failed")
)

// Client represents an SSH client connection with automatic resource management
type Client struct {
	config *ClientConfig
	client *ssh.Client
	conn   net.Conn

	// Lifecycle management
	closeOnce sync.Once
	closed    chan struct{}
	mu        sync.RWMutex
}

// NewClient creates a new SSH client using the provided configuration.
// The client must be explicitly closed by calling Close() when done.
//
// Example:
//
//	cfg := ssh.NewClientConfig("192.168.127.2", "root", "/path/to/key")
//	client, err := ssh.NewClient(ctx, cfg)
//	if err != nil {
//	    return err
//	}
//	defer client.Close()
func NewClient(ctx context.Context, config *ClientConfig) (*Client, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	client := &Client{
		config: config,
		closed: make(chan struct{}),
	}

	if err := client.connect(ctx); err != nil {
		return nil, err
	}

	// Start keepalive if interval is configured
	if config.KeepaliveInterval > 0 {
		client.startKeepalive()
	}

	return client, nil
}

// connect establishes the SSH connection
func (c *Client) connect(ctx context.Context) error {
	// Read private key
	privateKeyBytes, err := os.ReadFile(c.config.PrivateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key from %q: %w", c.config.PrivateKeyPath, err)
	}

	// Parse private key
	signer, err := ssh.ParsePrivateKey(privateKeyBytes)
	if err != nil {
		return fmt.Errorf("%w: failed to parse private key: %v", ErrAuthenticationFailed, err)
	}

	// Prepare SSH client configuration
	sshConfig := &ssh.ClientConfig{
		User: c.config.User,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Note: For VM use case, this is acceptable
		Timeout:         c.config.DialTimeout,
	}
	c.config.sshConfig = sshConfig

	// Establish network connection
	if err := c.dial(ctx); err != nil {
		return err
	}

	// Establish SSH connection
	addr := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)
	clientConn, chans, reqs, err := ssh.NewClientConn(c.conn, addr, sshConfig)
	if err != nil {
		c.conn.Close()
		return fmt.Errorf("%w: %v", ErrConnectionFailed, err)
	}

	c.client = ssh.NewClient(clientConn, chans, reqs)
	logrus.Debugf("SSH client connected to %s@%s", c.config.User, addr)

	return nil
}

// dial establishes the network connection (either direct or via gvproxy)
func (c *Client) dial(ctx context.Context) error {
	var conn net.Conn
	var err error

	// Use gvproxy tunnel if configured
	if c.config.GVProxySocketPath != "" {
		conn, err = c.dialViaGVProxy(ctx)
	} else {
		conn, err = c.dialDirect(ctx)
	}

	if err != nil {
		return fmt.Errorf("failed to establish network connection: %w", err)
	}

	c.conn = conn
	return nil
}

// dialDirect creates a direct TCP connection
func (c *Client) dialDirect(ctx context.Context) (net.Conn, error) {
	addr := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)

	dialer := &net.Dialer{
		Timeout: c.config.DialTimeout,
	}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", addr, err)
	}

	return conn, nil
}

// dialViaGVProxy creates a connection tunneled through gvproxy
func (c *Client) dialViaGVProxy(ctx context.Context) (net.Conn, error) {
	// Connect to gvproxy control socket
	conn, err := net.DialTimeout("unix", c.config.GVProxySocketPath, c.config.DialTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gvproxy at %q: %w", c.config.GVProxySocketPath, err)
	}

	// Establish tunnel
	if err := transport.Tunnel(conn, c.config.Host, int(c.config.Port)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create gvproxy tunnel: %w", err)
	}

	logrus.Debugf("Established gvproxy tunnel to %s:%d", c.config.Host, c.config.Port)
	return conn, nil
}

// NewSession creates a new SSH session from this client.
// The session must be explicitly closed by calling Close() when done.
//
// Example:
//
//	session, err := client.NewSession(ctx)
//	if err != nil {
//	    return err
//	}
//	defer session.Close()
func (c *Client) NewSession(ctx context.Context) (*Session, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.isClosed() {
		return nil, ErrClientClosed
	}

	sshSession, err := c.client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH session: %w", err)
	}

	return &Session{
		session: sshSession,
		client:  c,
		closed:  make(chan struct{}),
	}, nil
}

// startKeepalive starts sending periodic keepalive messages
func (c *Client) startKeepalive() {
	go c.keepaliveLoop()
}

// keepaliveLoop sends periodic keepalive messages, when client closed, it will return immediately
func (c *Client) keepaliveLoop() {
	ticker := time.NewTicker(c.config.KeepaliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.closed:
			return
		case <-ticker.C:
			c.mu.RLock()
			client := c.client
			c.mu.RUnlock()

			if client == nil {
				return
			}

			// Send keepalive request
			_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				logrus.Debugf("Keepalive failed: %v", err)
				return
			}
		}
	}
}

func isErrorIsConnectionAlreadyClosed(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "use of closed network connection") ||
		strings.Contains(err.Error(), "connection already closed")
}

// Close closes the SSH client and releases all resources.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		defer c.mu.Unlock()

		// Close SSH client (this also closes the underlying connection)
		if c.client != nil {
			if err := c.client.Close(); err != nil {
				logrus.Errorf("failed to close SSH client: %v", err)
			}
			c.client = nil
		}

		// Close connection if it wasn't already closed by client.Close()
		if c.conn != nil {
			if err := c.conn.Close(); err != nil && !isErrorIsConnectionAlreadyClosed(err) {
				logrus.Errorf("failed to close connection: %v", err)
			}
			c.conn = nil
		}

		// Signal closure
		close(c.closed)

		logrus.Debug("SSH client closed")
	})

	return nil
}

// isClosed returns true if the client has been closed
func (c *Client) isClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

// Wait blocks until the client connection is closed
func (c *Client) Wait() {
	<-c.closed
}
