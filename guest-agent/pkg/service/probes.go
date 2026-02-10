package service

import (
	"context"
	"net"
	"time"
)

func probeLocalTCP(ctx context.Context, addr string, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
			if err != nil {
				continue
			}
			conn.Close()
			return nil
		}
	}
}
