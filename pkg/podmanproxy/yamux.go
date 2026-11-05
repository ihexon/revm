package podmanproxy

import (
	"context"
	"net"
	"time"

	"github.com/hashicorp/yamux"
)

func muxConfig() *yamux.Config {
	cfg := yamux.DefaultConfig()
	// A client can finish its input long before Podman finishes its output.
	cfg.StreamCloseTimeout = 0
	return cfg
}

// HashiCorp Yamux Close is a half-close. Expose it as CloseWrite for tcpproxy,
// and make net.Conn.Close also unblock local reads during cancellation.
type streamConn struct{ *yamux.Stream }

func (s streamConn) CloseWrite() error { return s.Stream.Close() }
func (s streamConn) Close() error {
	_ = s.SetDeadline(time.Now())
	return s.Stream.Close()
}

type streamListener struct{ *yamux.Session }

func (s streamListener) Accept() (net.Conn, error) {
	stream, err := s.AcceptStream()
	if err != nil {
		return nil, err
	}
	return streamConn{stream}, nil
}

// OpenStream has no context parameter. Cancel this dial without closing the
// shared session and interrupting unrelated Podman connections.
func openStream(ctx context.Context, session *yamux.Session) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	type result struct {
		stream *yamux.Stream
		err    error
	}
	opened := make(chan result)
	go func() {
		stream, err := session.OpenStream()
		select {
		case opened <- result{stream, err}:
		case <-ctx.Done():
			if stream != nil {
				_ = stream.Close()
			}
		}
	}()
	select {
	case r := <-opened:
		if r.err != nil {
			return nil, r.err
		}
		return streamConn{r.stream}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
