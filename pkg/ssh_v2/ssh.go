// Package ssh_v2 provides a simplified SSH client for connecting to guest VMs.
//
// Example usage:
//
//	// Connect via gvproxy tunnel
//	client, err := ssh.Dial(ctx, "192.168.127.2:22",
//	    ssh.WithPrivateKey("/path/to/key"),
//	    ssh.WithTunnel("/path/to/gvproxy.sock"),
//	)
//	if err != nil {
//	    return err
//	}
//	defer client.Close()
//
//	// Run a command
//	if err := client.Run(ctx, "ls -la"); err != nil {
//	    return err
//	}
//
//	// Interactive shell with PTY
//	if err := client.Shell(ctx); err != nil {
//	    return err
//	}
package ssh_v2

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/containers/gvisor-tap-vsock/pkg/transport"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// Client represents an SSH connection to a remote host.
type Client struct {
	conn      net.Conn
	sshClient *ssh.Client
	opts      *options

	closed    chan struct{}
	closeOnce sync.Once
}

// options holds all configuration for the SSH client.
type options struct {
	user           string
	privateKeyPath string
	tunnelSocket   string
	dialTimeout    time.Duration
	keepalive      time.Duration
}

// Option configures the SSH client.
type Option func(*options)

// WithUser sets the SSH username (default: "root").
func WithUser(user string) Option {
	return func(o *options) { o.user = user }
}

// WithPrivateKey sets the path to the private key file.
func WithPrivateKey(path string) Option {
	return func(o *options) { o.privateKeyPath = path }
}

// WithTunnel enables connection through a gvproxy unix socket.
// If not set, a direct TCP connection is used.
func WithTunnel(socketPath string) Option {
	return func(o *options) { o.tunnelSocket = socketPath }
}

// WithTimeout sets the dial timeout (default: 5s).
func WithTimeout(d time.Duration) Option {
	return func(o *options) { o.dialTimeout = d }
}

// WithKeepalive sets the keepalive interval (default: 5s, 0 to disable).
func WithKeepalive(d time.Duration) Option {
	return func(o *options) { o.keepalive = d }
}

// Dial establishes an SSH connection to the given address.
//
// The address should be in the format "host:port".
// WithPrivateKey is required.
func Dial(ctx context.Context, addr string, opts ...Option) (*Client, error) {
	o := &options{
		user:        "root",
		dialTimeout: 5 * time.Second,
		keepalive:   5 * time.Second,
	}
	for _, opt := range opts {
		opt(o)
	}

	if o.privateKeyPath == "" {
		return nil, fmt.Errorf("private key path is required")
	}

	// Load private key
	keyData, err := os.ReadFile(o.privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}

	// Establish network connection
	var conn net.Conn
	if o.tunnelSocket != "" {
		conn, err = dialTunnel(o.tunnelSocket, addr, o.dialTimeout)
	} else {
		conn, err = dialDirect(ctx, addr, o.dialTimeout)
	}
	if err != nil {
		return nil, err
	}

	// SSH handshake
	sshConfig := &ssh.ClientConfig{
		User:            o.user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         o.dialTimeout,
	}

	clientConn, chans, reqs, err := ssh.NewClientConn(conn, addr, sshConfig)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ssh handshake: %w", err)
	}

	c := &Client{
		conn:      conn,
		sshClient: ssh.NewClient(clientConn, chans, reqs),
		opts:      o,
		closed:    make(chan struct{}),
	}

	if o.keepalive > 0 {
		go c.keepaliveLoop()
	}

	logrus.Debugf("ssh: connected to %s", addr)
	return c, nil
}

func dialDirect(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	return conn, nil
}

func dialTunnel(socket, addr string, timeout time.Duration) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address %q: %w", addr, err)
	}

	var portNum int
	if _, err := fmt.Sscanf(port, "%d", &portNum); err != nil {
		return nil, fmt.Errorf("invalid port %q: %w", port, err)
	}

	conn, err := net.DialTimeout("unix", socket, timeout)
	if err != nil {
		return nil, fmt.Errorf("dial gvproxy %s: %w", socket, err)
	}

	if err := transport.Tunnel(conn, host, portNum); err != nil {
		conn.Close()
		return nil, fmt.Errorf("establish tunnel: %w", err)
	}
	return conn, nil
}

func (c *Client) keepaliveLoop() {
	ticker := time.NewTicker(c.opts.keepalive)
	defer ticker.Stop()

	for {
		select {
		case <-c.closed:
			return
		case <-ticker.C:
			if _, _, err := c.sshClient.SendRequest("keepalive@openssh.com", true, nil); err != nil {
				logrus.Debugf("ssh: keepalive failed: %v", err)
				return
			}
		}
	}
}

// ErrClientClosed is returned when operations are attempted on a closed client.
var ErrClientClosed = fmt.Errorf("ssh client is closed")

func (c *Client) isClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

// Run executes a command on the remote host.
// Stdout and stderr are written to os.Stdout and os.Stderr.
func (c *Client) Run(ctx context.Context, cmd string) error {
	if c.isClosed() {
		return ErrClientClosed
	}
	return c.RunWith(ctx, cmd, nil, os.Stdout, os.Stderr)
}

// RunWith executes a command with custom I/O streams.
// Any of stdin, stdout, stderr can be nil.
func (c *Client) RunWith(ctx context.Context, cmd string, stdin io.Reader, stdout, stderr io.Writer) error {
	if c.isClosed() {
		return ErrClientClosed
	}
	session, err := c.sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	defer session.Close()

	session.Stdin = stdin
	session.Stdout = stdout
	session.Stderr = stderr

	errCh := make(chan error, 1)
	go func() {
		errCh <- session.Run(cmd)
	}()

	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGTERM)
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// Output executes a command and returns its stdout.
func (c *Client) Output(ctx context.Context, cmd string) ([]byte, error) {
	if c.isClosed() {
		return nil, ErrClientClosed
	}
	session, err := c.sshClient.NewSession()
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	defer session.Close()

	type result struct {
		out []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := session.Output(cmd)
		ch <- result{out, err}
	}()

	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGTERM)
		return nil, ctx.Err()
	case r := <-ch:
		return r.out, r.err
	}
}

// Shell starts an interactive shell with PTY.
// It takes over stdin/stdout/stderr and handles terminal resizing.
func (c *Client) Shell(ctx context.Context) error {
	return c.ShellWith(ctx, os.Stdin, os.Stdout, os.Stderr)
}

// ShellWith starts an interactive shell with custom I/O.
// If stdin is a terminal, it will be set to raw mode.
func (c *Client) ShellWith(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) error {
	if c.isClosed() {
		return ErrClientClosed
	}

	session, err := c.sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	defer session.Close()

	// Get terminal size
	width, height := 80, 24
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		if w, h, err := term.GetSize(int(f.Fd())); err == nil {
			width, height = w, h
		}
	}

	// Request PTY
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", height, width, modes); err != nil {
		return fmt.Errorf("request pty: %w", err)
	}

	// Set raw mode if stdin is a terminal
	var restoreFunc func()
	var resizeDone chan struct{}

	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		oldState, err := term.MakeRaw(int(f.Fd()))
		if err != nil {
			return fmt.Errorf("set raw mode: %w", err)
		}
		restoreFunc = func() {
			if err = term.Restore(int(f.Fd()), oldState); err != nil {
				logrus.Warnf("restore terminal state: %v", err)
			}
		}
		defer restoreFunc()

		// Handle window resize with done channel
		resizeDone = make(chan struct{})
		go c.watchResize(ctx, session, f, resizeDone)
	}

	session.Stdin = stdin
	session.Stdout = stdout
	session.Stderr = stderr

	if err := session.Shell(); err != nil {
		return fmt.Errorf("start shell: %w", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- session.Wait()
	}()

	var result error
	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGTERM)
		result = ctx.Err()
	case err := <-errCh:
		result = err
	}

	// Signal watchResize to stop
	if resizeDone != nil {
		close(resizeDone)
	}

	return result
}

func (c *Client) watchResize(ctx context.Context, session *ssh.Session, f *os.File, done <-chan struct{}) {
	sigCh := make(chan os.Signal, 1)
	signalNotify(sigCh)
	defer signalStop(sigCh)

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.closed:
			return
		case <-done:
			return
		case <-sigCh:
			if w, h, err := term.GetSize(int(f.Fd())); err == nil {
				_ = session.WindowChange(h, w)
			}
		}
	}
}

// Close closes the SSH connection.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		close(c.closed)
		if c.sshClient != nil {
			c.sshClient.Close()
		}
		if c.conn != nil {
			c.conn.Close()
		}
		logrus.Debugf("ssh: connection closed")
	})
	return nil
}
