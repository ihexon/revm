package vsock

import (
	"errors"
	"net"
	"testing"

	"github.com/mdlayher/vsock"
)

type peerConn struct {
	net.Conn
	addr   net.Addr
	closed bool
}

func (c *peerConn) RemoteAddr() net.Addr { return c.addr }
func (c *peerConn) Close() error         { c.closed = true; return nil }

type peerListener struct {
	net.Listener
	peers []net.Conn
}

func (l *peerListener) Accept() (net.Conn, error) {
	if len(l.peers) == 0 {
		return nil, net.ErrClosed
	}
	c := l.peers[0]
	l.peers = l.peers[1:]
	return c, nil
}

// Guest loopback must never reach either the root command runner or Podman.
func TestHostListenerRejectsGuestPeers(t *testing.T) {
	loopback := &peerConn{addr: &vsock.Addr{ContextID: vsock.Local}}
	guest := &peerConn{addr: &vsock.Addr{ContextID: 3}}
	otherGuest := &peerConn{addr: &vsock.Addr{ContextID: 4}}
	unknown := &peerConn{addr: &net.UnixAddr{Name: "untrusted", Net: "unix"}}
	host := &peerConn{addr: &vsock.Addr{ContextID: vsock.Host}}
	l := hostListener{&peerListener{peers: []net.Conn{loopback, guest, otherGuest, unknown, host}}}
	c, err := l.Accept()
	if err != nil || c != host {
		t.Fatalf("expected host connection, got %v, %v", c, err)
	}
	for _, rejected := range []*peerConn{loopback, guest, otherGuest, unknown} {
		if !rejected.closed {
			t.Errorf("untrusted peer %v was not closed", rejected.addr)
		}
	}
	if host.closed {
		t.Fatal("host connection was closed")
	}
	if _, err := l.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("listener close was not propagated: %v", err)
	}
}
