package guestcontrol

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"linuxvm/pkg/execution"
	"linuxvm/pkg/network"
	"linuxvm/pkg/protocol"
	"net"
	"net/http"
	"time"
)

func WaitReady(ctx context.Context, socket string) (protocol.ExecReady, error) {
	var lastErr error
	for {
		data, status, err := network.UnixRequest(ctx, socket, http.MethodGet, "/v3/ready", nil)
		if err == nil {
			if status == 404 {
				return protocol.ExecReady{}, fmt.Errorf("guest lacks exec v3 capability; restart with the current revm")
			}
			if status == 200 {
				var ready protocol.ExecReady
				if err = json.Unmarshal(data, &ready); err != nil {
					return ready, err
				}
				if ready.Version != execution.ProtocolVersion || ready.InstanceID == "" {
					return ready, fmt.Errorf("unsupported guest execution protocol")
				}
				return ready, nil
			}
			lastErr = fmt.Errorf("guest readiness returned HTTP %d", status)
		} else if ctx.Err() == nil {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return protocol.ExecReady{}, fmt.Errorf("wait for guest readiness: %w (last probe: %v)", context.Cause(ctx), lastErr)
			}
			return protocol.ExecReady{}, fmt.Errorf("wait for guest readiness: %w", context.Cause(ctx))
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func StreamExec(ctx context.Context, socket string, req execution.Request, opts execution.Options) (execution.Result, error) {
	readyCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	ready, err := WaitReady(readyCtx, socket)
	cancel()
	if err != nil {
		return execution.Result{}, err
	}
	req.Version, req.InstanceID = execution.ProtocolVersion, ready.InstanceID
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socket)
	if err != nil {
		return execution.Result{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err = io.WriteString(conn, "GET /v3/exec HTTP/1.1\r\nHost: guest\r\nConnection: Upgrade\r\nUpgrade: revm-exec-v3\r\n\r\n"); err != nil {
		return execution.Result{}, err
	}
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		return execution.Result{}, err
	}
	if resp.StatusCode != 101 {
		return execution.Result{}, fmt.Errorf("guest exec upgrade: %s", resp.Status)
	}
	_ = conn.SetDeadline(time.Time{})
	return execution.Execute(ctx, &bufferedConn{Conn: conn, reader: reader}, req, opts)
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
