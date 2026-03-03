//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"context"
	"fmt"
	"linuxvm/pkg/network"
	"strings"
	"time"
)

// httpEventSink sends VM lifecycle events to an HTTP endpoint (Unix socket or
// TCP) via GET /notify?stage=…&name=…&value=….
type httpEventSink struct {
	client *network.Client
	stage  string
}

// NewHTTPEventSink creates an EventSink that reports to the given endpoint.
// Supported schemes: unix:///path/to/sock, tcp://host:port.
// Returns nil (no-op) if endpoint is empty or unparseable.
func NewHTTPEventSink(endpoint, stage string) EventSink {
	if endpoint == "" || stage == "" {
		return nil
	}

	var client *network.Client
	switch {
	case strings.HasPrefix(endpoint, "unix://") || strings.HasPrefix(endpoint, "unixgram://"):
		addr, err := network.ParseUnixAddr(endpoint)
		if err != nil {
			return nil
		}
		client = network.NewUnixClient(addr.Path, network.WithTimeout(1*time.Second))
	case strings.HasPrefix(endpoint, "tcp://"):
		addr, err := network.ParseTcpAddr(endpoint)
		if err != nil {
			return nil
		}
		hostPort := fmt.Sprintf("%s:%d", addr.Host, addr.Port)
		client = network.NewTCPClient(hostPort, network.WithTimeout(1*time.Second))
	default:
		return nil
	}

	return &httpEventSink{client: client, stage: stage}
}

func (s *httpEventSink) Publish(evt Event) {
	if s == nil || s.client == nil {
		return
	}
	value := ""
	if evt.Kind == EventError {
		value = evt.Message
	}
	resp, err := s.client.Get("/notify").
		Query("stage", s.stage).
		Query("name", string(evt.Kind)).
		Query("value", value).
		Do(context.Background()) //nolint:bodyclose
	if err != nil {
		return
	}
	network.CloseResponse(resp)
}

func (s *httpEventSink) Close() error {
	if s != nil && s.client != nil {
		return s.client.Close()
	}
	return nil
}
