//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"context"
	"fmt"
	"linuxvm/pkg/network"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

type v1EventReporter struct {
	client *network.Client
}

func newEventReporter(endpoint string) EventReporter {
	if endpoint == "" {
		return nil
	}
	client := newEventReporterClient(endpoint)
	if client == nil {
		return nil
	}
	return &v1EventReporter{client: client}
}

func newEventReporterClient(endpoint string) *network.Client {
	switch {
	case strings.HasPrefix(endpoint, "unix://") || strings.HasPrefix(endpoint, "unixgram://"):
		addr, err := network.ParseUnixAddr(endpoint)
		if err != nil {
			logrus.Warnf("event sink: invalid unix endpoint %q: %v", endpoint, err)
			return nil
		}
		return network.NewUnixClient(addr.Path, network.WithTimeout(1*time.Second))
	case strings.HasPrefix(endpoint, "tcp://"):
		addr, err := network.ParseTcpAddr(endpoint)
		if err != nil {
			logrus.Warnf("event sink: invalid tcp endpoint %q: %v", endpoint, err)
			return nil
		}
		hostPort := fmt.Sprintf("%s:%d", addr.Host, addr.Port)
		return network.NewTCPClient(hostPort, network.WithTimeout(1*time.Second))
	default:
		logrus.Warnf("event sink: unsupported endpoint scheme %q", endpoint)
		return nil
	}
}

func (r *v1EventReporter) Report(evt Event) {
	req := r.client.Get("/v1/event").
		Query("session_id", evt.SessionID).
		Query("mode", string(evt.RunMode)).
		Query("kind", string(evt.Kind)).
		Query("seq", fmt.Sprintf("%d", evt.Seq)).
		Query("msg", evt.Message).
		Query("time", evt.Time.Format(time.RFC3339Nano))
	resp, err := req.Do(context.Background()) //nolint:bodyclose
	if err != nil {
		logrus.Warnf("v1 event sink: emit %s failed: %v", evt.Kind, err)
		return
	}
	network.CloseResponse(resp)
}

func (r *v1EventReporter) Close() {
	if err := r.client.Close(); err != nil {
		logrus.Warnf("v1 event sink: close failed: %v", err)
	}
}
