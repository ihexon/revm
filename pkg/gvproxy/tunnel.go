package gvproxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"linuxvm/pkg/network"
	"net"
	"os"
	"sync"
	"time"

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

	var closeOnce sync.Once
	closeLn := func() { closeOnce.Do(func() { _ = ln.Close() }) }
	defer closeLn()

	go func() {
		<-ctx.Done()
		closeLn()
	}()

	return acceptLoop(ctx, ln, gvproxyPath, targetIP, targetPort)
}

func parseUnixSocketPath(addr string) (string, error) {
	parsed, err := network.ParseUnixAddr(addr)
	if err != nil {
		return "", err
	}
	if parsed.Path == "" {
		return "", fmt.Errorf("empty socket path")
	}
	return parsed.Path, nil
}

func createUnixListenerSockFile(path string) (net.Listener, error) {
	_ = os.Remove(path)

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
			if errors.Is(err, net.ErrClosed) {
				return fmt.Errorf("listener %q closed", ln.Addr())
			}

			logrus.Warnf("tunnel accept error: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		go handleTunnelConn(ctx, conn, gvproxyPath, targetIP, targetPort)
	}
}

func handleTunnelConn(ctx context.Context, hostConn net.Conn, gvproxyPath, targetIP string, targetPort uint16) {
	defer hostConn.Close()

	guestConn, err := net.Dial("unix", gvproxyPath)
	if err != nil {
		logrus.Errorf("dial gvproxy %s: %v", gvproxyPath, err)
		return
	}
	defer guestConn.Close()

	if err := transport.Tunnel(guestConn, targetIP, int(targetPort)); err != nil {
		logrus.Errorf("tunnel setup to %s:%d failed: %v", targetIP, targetPort, err)
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = hostConn.Close()
			_ = guestConn.Close()
		case <-done:
		}
	}()

	go copyDataGuestToHost(&wg, hostConn, guestConn)
	go copyDataHostToGuest(&wg, hostConn, guestConn)

	wg.Wait()
	close(done)
}

// copyDataGuestToHost copies guest output back to the host and propagates EOF.
func copyDataGuestToHost(wg *sync.WaitGroup, hostConn, guestConn net.Conn) {
	defer wg.Done()

	_, _ = io.Copy(hostConn, guestConn)
	_ = closeWrite(hostConn)
}

// copyDataHostToGuest copies host input into the guest.
// EOF stays local so read-only attach flows do not get closed prematurely.
func copyDataHostToGuest(wg *sync.WaitGroup, hostConn, guestConn net.Conn) {
	defer wg.Done()

	_, _ = io.Copy(guestConn, hostConn)
}

func closeWrite(conn net.Conn) error {
	if tcp, ok := conn.(*net.TCPConn); ok {
		return tcp.CloseWrite()
	}
	if unix, ok := conn.(*net.UnixConn); ok {
		return unix.CloseWrite()
	}
	return conn.Close()
}
