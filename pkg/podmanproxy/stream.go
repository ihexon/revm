package podmanproxy

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
)

const maxFrameSize = 32 << 10

// Each direction sends uint32 big-endian length + data; zero length means EOF.
// There are no stream IDs, acknowledgments or window updates. Reading never
// writes to the peer, so a completed sender cannot interrupt buffered output.
// The socket supplies backpressure; malformed or incomplete frames are fatal.
type framedConn struct {
	net.Conn
	readMu, writeMu   sync.Mutex
	remaining         uint32
	readEOF, writeEOF bool
}

func (c *framedConn) Read(p []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if len(p) == 0 {
		return 0, nil
	}
	if c.readEOF {
		return 0, io.EOF
	}
	if c.remaining == 0 {
		var header [4]byte
		if _, err := io.ReadFull(c.Conn, header[:]); err != nil {
			c.Close()
			if err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return 0, err
		}
		c.remaining = binary.BigEndian.Uint32(header[:])
		if c.remaining > maxFrameSize {
			c.Close()
			return 0, fmt.Errorf("Podman frame exceeds %d bytes", maxFrameSize)
		}
		if c.remaining == 0 {
			c.readEOF = true
			return 0, io.EOF
		}
	}
	p = p[:min(len(p), int(c.remaining))]
	n, err := io.ReadFull(c.Conn, p)
	c.remaining -= uint32(n)
	if err != nil {
		c.Close()
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
	}
	return n, err
}

func (c *framedConn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.writeEOF {
		return 0, io.ErrClosedPipe
	}
	total := 0
	for len(p) > 0 {
		chunk := p[:min(len(p), maxFrameSize)]
		n, err := c.writeFrame(chunk)
		total += n
		if err != nil {
			return total, err
		}
		p = p[n:]
	}
	return total, nil
}

func (c *framedConn) CloseWrite() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.writeEOF {
		return nil
	}
	c.writeEOF = true
	_, err := c.writeFrame(nil)
	return err
}

func (c *framedConn) writeFrame(p []byte) (n int, err error) {
	defer func() {
		if err != nil {
			c.Close()
		}
	}()
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(p)))
	if _, err = c.Conn.Write(header[:]); err != nil || len(p) == 0 {
		return 0, err
	}
	return c.Conn.Write(p)
}
