package network

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/containers/gvisor-tap-vsock/pkg/transport"
	"github.com/sirupsen/logrus"
)

// ForwardPodmanAPIOverVSock forwards the Podman API through a vsock proxy,
// enabling communication between the host and a container.
//
// Parameters:
//
//   - ctx controls the lifetime of the forwarding loop.
//   - gvpAddr is the unix address to the gvproxy control socket.
//   - listenAddr is the Unix domain socket address where the proxy listens for incoming connections.
//   - targetIP and targetPort specify the destination endpoint inside the container.
func ForwardPodmanAPIOverVSock(ctx context.Context, gvproxyCtlUnixAddr, listHostUnixAddr, targetIP string, targetPort uint16) error {
	gvpAddr, err := ParseUnixAddr(gvproxyCtlUnixAddr)
	if err != nil {
		return fmt.Errorf("failed to parse gvproxy socket address: %w", err)
	}
	if gvpAddr.Path == "" {
		return fmt.Errorf("parsed gvproxy control socket address is empty")
	}

	listenAddr, err := ParseUnixAddr(listHostUnixAddr)
	if err != nil {
		return fmt.Errorf("failed to parse listen socket address: %w", err)
	}

	if listenAddr.Path == "" {
		return fmt.Errorf("parsed listen socket address is empty")
	}

	// Ensure old socket is removed if exists
	if err := os.Remove(listenAddr.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove old socket %s: %w", listenAddr.Path, err)
	}

	ln, err := net.Listen("unix", listenAddr.Path)
	if err != nil {
		return fmt.Errorf("listen on %q: %w", listenAddr.Path, err)
	}
	defer func(ln net.Listener) {
		_ = ln.Close()
	}(ln)

	if err = os.Chmod(listenAddr.Path, 0600); err != nil {
		logrus.Warnf("chmod unix socket %q: %v", listenAddr.Path, err)
	}

	go func(ctx context.Context) {
		<-ctx.Done()
		_ = ln.Close()
	}(ctx)

	// Accept loop
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		conn, err := ln.Accept()
		if err != nil {
			logrus.Debugf("accept connection error: %v", err)
			continue
		}

		go handleConn(ctx, conn, gvpAddr.Path, targetIP, targetPort)
	}
}

func handleConn(ctx context.Context, clientConn net.Conn, gvproxyCtlSocksPath, targetIP string, targetPort uint16) {
	logrus.Debugf("accepted new connection from %v", clientConn.RemoteAddr())

	guestConn, err := net.Dial("unix", gvproxyCtlSocksPath)
	if err != nil {
		logrus.Errorf("dial gvproxy socket %q failed: %v", gvproxyCtlSocksPath, err)
		clientConn.Close() //nolint:errcheck
		return
	}

	if err := transport.Tunnel(guestConn, targetIP, int(targetPort)); err != nil {
		logrus.Errorf("setup tunnel to %s:%d failed: %v", targetIP, targetPort, err)
		guestConn.Close()  //nolint:errcheck
		clientConn.Close() //nolint:errcheck
		return
	}

	// Start bidirectional copy
	go proxyCopy(clientConn, guestConn, "guest->client")
	go proxyCopy(guestConn, clientConn, "client->guest")
}

func proxyCopy(dst, src net.Conn, direction string) {
	defer dst.Close() //nolint:errcheck
	defer src.Close() //nolint:errcheck

	if _, err := io.Copy(dst, src); err != nil && !isUseOfClosedErr(err) {
		logrus.Errorf("io copy error (%v): %v", direction, err)
	}
}

// helper: ignore "use of closed network connection" errors
func isUseOfClosedErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "use of closed network connection")
}
