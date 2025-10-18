package ssh

import (
	"errors"
	"time"

	"al.essio.dev/pkg/shellescape"
	"golang.org/x/crypto/ssh"
)

var (
	// ErrInvalidConfig is returned when the configuration is invalid
	ErrInvalidConfig = errors.New("invalid SSH configuration")
)

// Default configuration values
const (
	DefaultDialTimeout       = 5 * time.Second
	DefaultKeepaliveInterval = 5 * time.Second
)

// ClientConfig contains all the information needed to establish an SSH connection
type ClientConfig struct {
	// Connection details
	Host string
	Port uint16
	User string

	// Authentication
	PrivateKeyPath string

	// Network configuration
	DialTimeout       time.Duration
	KeepaliveInterval time.Duration

	// For gvproxy tunneling
	GVProxySocketPath string

	// SSH client configuration (internal use)
	sshConfig *ssh.ClientConfig
}

// NewClientConfig creates a new ClientConfig with default values
func NewClientConfig(host string, port uint16, user, privateKeyPath string) *ClientConfig {
	return &ClientConfig{
		Host:              host,
		Port:              port,
		User:              user,
		PrivateKeyPath:    privateKeyPath,
		DialTimeout:       DefaultDialTimeout,
		KeepaliveInterval: DefaultKeepaliveInterval,
	}
}

// WithPort sets the SSH port
func (c *ClientConfig) WithPort(port uint16) *ClientConfig {
	c.Port = port
	return c
}

// WithDialTimeout sets the connection timeout
func (c *ClientConfig) WithDialTimeout(timeout time.Duration) *ClientConfig {
	c.DialTimeout = timeout
	return c
}

// WithKeepaliveInterval sets the keepalive interval
func (c *ClientConfig) WithKeepaliveInterval(interval time.Duration) *ClientConfig {
	c.KeepaliveInterval = interval
	return c
}

// WithGVProxySocket sets the gvproxy socket path for tunneling
func (c *ClientConfig) WithGVProxySocket(socketPath string) *ClientConfig {
	c.GVProxySocketPath = socketPath
	return c
}

// Validate checks if the configuration is valid
func (c *ClientConfig) Validate() error {
	if c.Host == "" {
		return errors.Join(ErrInvalidConfig, errors.New("host cannot be empty"))
	}
	if c.User == "" {
		return errors.Join(ErrInvalidConfig, errors.New("user cannot be empty"))
	}
	if c.PrivateKeyPath == "" {
		return errors.Join(ErrInvalidConfig, errors.New("private key path cannot be empty"))
	}
	if c.Port == 0 {
		return errors.Join(ErrInvalidConfig, errors.New("port must be greater than 0"))
	}
	if c.DialTimeout <= 0 {
		return errors.Join(ErrInvalidConfig, errors.New("dial timeout must be positive"))
	}
	return nil
}

// SessionConfig contains configuration for an SSH session
type SessionConfig struct {
	// Command to execute
	Command []string

	// Terminal configuration
	EnablePTY      bool
	TerminalType   string
	TerminalWidth  int
	TerminalHeight int

	// Signal to send on context cancellation
	CancelSignal ssh.Signal
}

func (s *SessionConfig) CommandString() string {
	return shellescape.QuoteCommand(s.Command)
}

// NewSessionConfig creates a new SessionConfig with default values
func NewSessionConfig(command ...string) *SessionConfig {
	return &SessionConfig{
		Command:        command,
		EnablePTY:      false,
		TerminalType:   "xterm-256color",
		TerminalWidth:  80,
		TerminalHeight: 24,
		CancelSignal:   ssh.SIGTERM,
	}
}

// WithPTY enables PTY with the specified terminal type and dimensions
func (s *SessionConfig) WithPTY(termType string, width, height int) *SessionConfig {
	s.EnablePTY = true
	s.TerminalType = termType
	s.TerminalWidth = width
	s.TerminalHeight = height
	return s
}

// WithCancelSignal sets the signal to send when the context is canceled
func (s *SessionConfig) WithCancelSignal(signal ssh.Signal) *SessionConfig {
	s.CancelSignal = signal
	return s
}
