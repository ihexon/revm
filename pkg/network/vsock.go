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

func TunnelHostUnixToGuest(ctx context.Context, gvproxyCtlUnixAddr, listHostUnixAddr, targetIP string, targetPort uint16) error {
	gvpAddr, err := ParseUnixAddr(gvproxyCtlUnixAddr)
	if err != nil {
		return fmt.Errorf("failed to parse gvproxy socket address: %w", err)
	}
	if gvpAddr.Path == "" {
		return fmt.Errorf("parsed gvproxy control socket address is empty")
	}

	listenAddr, err := ParseUnixAddr(listHostUnixAddr)
	if err != nil {
		return fmt.Errorf("failed to parse listen socket address %q: %w", listHostUnixAddr, err)
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
			logrus.Infof("accept connection error: %v", err)
			continue
		}

		go handleConn(ctx, conn, gvpAddr.Path, targetIP, targetPort)
	}
}

func handleConn(_ context.Context, clientConn net.Conn, gvproxyCtlSocksPath, targetIP string, targetPort uint16) {
	logrus.Infof("accepted new connection from %v", clientConn.RemoteAddr())

	guestConn, err := net.Dial("unix", gvproxyCtlSocksPath)
	if err != nil {
		logrus.Errorf("dial gvproxy socket %q failed: %v", gvproxyCtlSocksPath, err)
		clientConn.Close()
		return
	}

	if err := transport.Tunnel(guestConn, targetIP, int(targetPort)); err != nil {
		logrus.Errorf("setup tunnel to %s:%d failed: %v", targetIP, targetPort, err)
		guestConn.Close()
		clientConn.Close()
		return
	}

	// Start bidirectional copy
	go proxyCopy(clientConn, guestConn, "guest->client")
	go proxyCopy(guestConn, clientConn, "client->guest")
}

func proxyCopy(dst, src net.Conn, direction string) {
	defer dst.Close()
	defer src.Close()

	if _, err := io.Copy(dst, src); err != nil && !isUseOfClosedErr(err) {
		logrus.Errorf("io copy error (%v): %v", direction, err)
	}
}

// helper: ignore "use of closed network connection" errors
func isUseOfClosedErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "use of closed network connection")
}
