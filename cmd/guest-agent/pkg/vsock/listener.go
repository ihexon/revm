package vsock

import (
	"net"

	"github.com/mdlayher/vsock"
)

// ListenHost rejects guest loopback peers, including host-network containers.
// The kernel supplies the CID; HTTP headers cannot override this identity.
func ListenHost(port uint32) (net.Listener, error) {
	listener, err := vsock.Listen(port, nil)
	if err != nil {
		return nil, err
	}
	return hostListener{listener}, nil
}

type hostListener struct{ net.Listener }

func (l hostListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if addr, ok := conn.RemoteAddr().(*vsock.Addr); ok && addr.ContextID == vsock.Host {
			return conn, nil
		}
		_ = conn.Close()
	}
}
