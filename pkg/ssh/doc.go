/*
Package ssh provides a clean, idiomatic Go interface for SSH operations with
automatic resource management and comprehensive error handling.

This package is a complete redesign of the original pkg/ssh package, addressing
resource leaks, improving separation of concerns, and providing a more intuitive API.

# Architecture

The package is organized into clear layers:

  - keygen.go: SSH key pair generation
  - config.go: Configuration structures
  - client.go: SSH client connection management
  - session.go: SSH session lifecycle management
  - executor.go: High-level command execution

# Basic Usage

The most common use case is executing commands on a remote host:

	package main

	import (
		"context"
		"linuxvm/pkg/ssh"
		"log"
	)

	func main() {
		ctx := context.Background()

		// Configure client connection
		cfg := ssh.NewClientConfig("192.168.127.2", "root", "/path/to/key").
			WithPort(22).
			WithGVProxySocket("/tmp/gvproxy.sock")

		// Create client
		client, err := ssh.NewClient(ctx, cfg)
		if err != nil {
			log.Fatal(err)
		}
		defer client.Close()

		// Create executor
		executor := ssh.NewExecutor(client)

		// Execute command
		opts := ssh.DefaultExecOptions()
		if err := executor.Exec(ctx, opts, "ls", "-la", "/tmp"); err != nil {
			log.Fatal(err)
		}
	}

# Key Generation

Generate SSH key pairs for authentication:

	opts := ssh.DefaultKeyGenOptions()
	keyPair, err := ssh.GenerateKeyPair("/path/to/keyfile", opts)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Private key:", keyPair.PrivateKeyPath())
	fmt.Println("Public key:", keyPair.PublicKeyPath())

# Client Connection

Clients manage the SSH connection lifecycle and support both direct TCP
connections and gvproxy tunneling:

	// Direct TCP connection
	cfg := ssh.NewClientConfig("192.168.1.100", "user", "/path/to/key")

	// Or via gvproxy tunnel
	cfg.WithGVProxySocket("/tmp/gvproxy.sock")

	client, err := ssh.NewClient(ctx, cfg)
	if err != nil {
		return err
	}
	defer client.Close()

Clients automatically manage keepalive messages and handle connection cleanup.

# Session Management

For more control, create sessions directly:

	session, err := client.NewSession(ctx)
	if err != nil {
		return err
	}
	defer session.Close()

	// Configure I/O
	session.SetStdout(os.Stdout)
	session.SetStderr(os.Stderr)

	// Request PTY for interactive commands
	if err := session.RequestPTY(ctx, "xterm-256color", 80, 24); err != nil {
		return err
	}

	// Execute command
	if err := session.Run(ctx, "bash"); err != nil {
		return err
	}

# Command Execution

The Executor provides convenient methods for common scenarios:

Simple execution:

	executor := ssh.NewExecutor(client)
	opts := ssh.DefaultExecOptions()
	err := executor.Exec(ctx, opts, "echo", "hello")

Capture output:

	stdout, stderr, err := executor.ExecWithOutput(ctx, "uptime")
	if err != nil {
		return err
	}
	fmt.Println(string(stdout))

Interactive execution with PTY:

	err := executor.ExecInteractive(ctx, "/bin/bash")

Streaming execution:

	stream, err := executor.ExecStream(ctx, opts, "tail", "-f", "/var/log/syslog")
	if err != nil {
		return err
	}
	defer stream.Close()

	// Do other work while command runs...

	// Wait for completion
	if err := stream.Wait(); err != nil {
		return err
	}

# Resource Management

All types implement proper resource cleanup:

  - Client: Closes SSH connection, stops keepalive, closes network socket
  - Session: Closes SSH session, restores terminal state, cleans up I/O pipes
  - Stream: Cancels context, closes session

Always use defer to ensure cleanup:

	client, err := ssh.NewClient(ctx, cfg)
	if err != nil {
		return err
	}
	defer client.Close()

# Error Handling

The package defines sentinel errors for common failure modes:

	var (
		ErrClientClosed         // Client operations on closed client
		ErrConnectionFailed     // SSH connection establishment failed
		ErrAuthenticationFailed // SSH authentication failed
		ErrSessionClosed        // Session operations on closed session
		ErrPTYRequestFailed     // PTY allocation failed
		ErrCommandFailed        // Command execution failed
		ErrInvalidConfig        // Invalid configuration
		ErrEmptyDestination     // Empty key generation destination
		ErrKeyGenerationFailed  // SSH key generation failed
		ErrKeyWriteFailed       // Failed to write SSH keys
	)

Use errors.Is() for error checking:

	if errors.Is(err, ssh.ErrAuthenticationFailed) {
		// Handle authentication failure
	}

All errors include detailed context via error wrapping.

# Context Support

All operations support context-based cancellation:

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := ssh.NewClient(ctx, cfg)
	if err != nil {
		return err
	}

When a context is canceled during command execution, the configured signal
(default: SIGTERM) is sent to the remote process.

# Comparison with pkg/ssh

Key improvements over the original pkg/ssh package:

1. Resource Management:
  - Automatic cleanup via defer instead of manual callback tracking
  - No resource leaks from partially initialized objects
  - Proper use of sync.Once for idempotent Close() methods

2. Separation of Concerns:
  - Client handles connection lifecycle
  - Session handles individual command execution
  - Executor provides high-level convenience methods
  - Configuration is separate from execution logic

3. Error Handling:
  - Sentinel errors for common cases
  - Comprehensive error context via wrapping
  - No silently ignored errors

4. API Design:
  - Builder pattern for configuration
  - Functional options where appropriate
  - Clear ownership semantics
  - Context-aware cancellation

5. Thread Safety:
  - All types are safe for concurrent use
  - Proper mutex protection for shared state
  - No race conditions in cleanup paths

# Thread Safety

All exported types are safe for concurrent use:

  - Multiple goroutines can share a Client
  - Each goroutine should use its own Session
  - Executor is stateless and safe to share

Example:

	client, _ := ssh.NewClient(ctx, cfg)
	defer client.Close()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			session, _ := client.NewSession(ctx)
			defer session.Close()

			session.Run(ctx, "echo", fmt.Sprintf("Worker %d", id))
		}(i)
	}
	wg.Wait()

# Best Practices

1. Always use defer for resource cleanup
2. Check errors from all operations
3. Use context for timeout and cancellation control
4. Prefer Executor methods for simple use cases
5. Use Session directly when you need fine-grained control
6. Enable keepalive for long-lived connections
7. Request PTY only when needed (interactive commands)

# Migration from pkg/ssh

Old code:

	cfg := ssh.NewCfg(addr, user, port, keyFile)
	defer cfg.CleanUp.CleanIfErr(&err)

	cfg.SetCmdLine(bin, args)
	cfg.SetPty(false)

	if err := cfg.Connect(ctx, gvproxySocket); err != nil {
		return err
	}

	if err := cfg.WriteOutputTo(stdout, stderr); err != nil {
		return err
	}

	if err := cfg.Run(ctx); err != nil {
		return err
	}

New code:

	clientCfg := ssh.NewClientConfig(addr, user, keyFile).
		WithPort(uint16(port)).
		WithGVProxySocket(gvproxySocket)

	client, err := ssh.NewClient(ctx, clientCfg)
	if err != nil {
		return err
	}
	defer client.Close()

	executor := ssh.NewExecutor(client)
	opts := ssh.DefaultExecOptions().
		WithStdout(stdout).
		WithStderr(stderr)

	cmd := append([]string{bin}, args...)
	if err := executor.Exec(ctx, opts, cmd...); err != nil {
		return err
	}
*/
package ssh
