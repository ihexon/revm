package network

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"

	"github.com/containers/gvisor-tap-vsock/pkg/transport"
	"github.com/sirupsen/logrus"
)

// TunnelHostUnixToGuest creates a Unix socket tunnel that forwards connections to a guest VM.
func TunnelHostUnixToGuest(ctx context.Context, gvproxyCtlUnixAddr, listenUnixAddr, targetIP string, targetPort uint16) error {
	gvproxyPath, err := parseUnixSocketPath(gvproxyCtlUnixAddr)
	if err != nil {
		return fmt.Errorf("invalid gvproxy socket address: %w", err)
	}

	listenPath, err := parseUnixSocketPath(listenUnixAddr)
	if err != nil {
		return fmt.Errorf("invalid listen socket address: %w", err)
	}

	ln, err := createUnixListenerSockFile(listenPath)
	if err != nil {
		return err
	}

	// Use sync.Once to ensure listener is closed exactly once
	var closeOnce sync.Once
	closeLn := func() { closeOnce.Do(func() { _ = ln.Close() }) }
	defer closeLn()

	// Close listener when context is cancelled to unblock Accept()
	go func() {
		<-ctx.Done()
		closeLn()
	}()

	return acceptLoop(ctx, ln, gvproxyPath, targetIP, targetPort)
}

func parseUnixSocketPath(addr string) (string, error) {
	parsed, err := ParseUnixAddr(addr)
	if err != nil {
		return "", err
	}
	if parsed.Path == "" {
		return "", fmt.Errorf("empty socket path")
	}
	return parsed.Path, nil
}

func createUnixListenerSockFile(path string) (net.Listener, error) {
	_ = os.Remove(path) // ignore error if not exists

	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on %q: %w", path, err)
	}

	if err = os.Chmod(path, 0600); err != nil {
		logrus.Warnf("chmod unix socket %q: %v", path, err)
	}

	return ln, nil
}

func acceptLoop(ctx context.Context, ln net.Listener, gvproxyPath, targetIP string, targetPort uint16) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}

		go handleTunnelConn(conn, gvproxyPath, targetIP, targetPort)
	}
}

func handleTunnelConn(clientConn net.Conn, gvproxyPath, targetIP string, targetPort uint16) {
	defer clientConn.Close()

	guestConn, err := net.Dial("unix", gvproxyPath)
	if err != nil {
		logrus.Errorf("dial gvproxy socket %q: %v", gvproxyPath, err)
		return
	}
	defer guestConn.Close()

	if err := transport.Tunnel(guestConn, targetIP, int(targetPort)); err != nil {
		logrus.Errorf("setup tunnel to %s:%d: %v", targetIP, targetPort, err)
		return
	}

	bidirectionalCopy(clientConn, guestConn)
}

func bidirectionalCopy(conn1, conn2 net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	copy := func(dst, src net.Conn) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
	}

	go copy(conn1, conn2)
	go copy(conn2, conn1)

	wg.Wait()
}
