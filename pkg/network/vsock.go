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
//   - gvproxyCtlSocks is the path to the gvproxy control socket.
//   - listenInSocketAddr is the Unix domain socket address where the proxy listens for incoming connections.
//   - targetIP and targetPort specify the destination endpoint inside the container.
func ForwardPodmanAPIOverVSock(ctx context.Context, gvproxyCtlSocks, listenInSocketAddr, targetIP string, targetPort uint16) error {
	// Ensure old socket is removed if exists
	if err := os.Remove(listenInSocketAddr); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove old socket %s: %w", listenInSocketAddr, err)
	}

	ln, err := net.Listen("unix", listenInSocketAddr)
	if err != nil {
		return fmt.Errorf("listen on %q: %w", listenInSocketAddr, err)
	}
	defer func(ln net.Listener) {
		if err := ln.Close(); err != nil {
			logrus.Errorf("close unix socket %q: %v", listenInSocketAddr, err)
		}
	}(ln)

	if err = os.Chmod(listenInSocketAddr, 0600); err != nil {
		logrus.Warnf("chmod unix socket %q: %v", listenInSocketAddr, err)
	}

	// Accept loop
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		conn, err := ln.Accept()
		if err != nil {
			logrus.Warnf("accept connection error: %v", err)
			continue
		}

		go handleConn(ctx, conn, gvproxyCtlSocks, targetIP, targetPort)
	}
}

func handleConn(ctx context.Context, clientConn net.Conn, gvproxyCtlSocks, targetIP string, targetPort uint16) {
	logrus.Infof("accepted new connection from %v", clientConn.RemoteAddr())

	guestConn, err := net.Dial("unix", gvproxyCtlSocks)
	if err != nil {
		logrus.Errorf("dial gvproxy socket %q failed: %v", gvproxyCtlSocks, err)
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
