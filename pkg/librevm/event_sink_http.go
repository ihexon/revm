//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"context"
	"fmt"
	"linuxvm/pkg/network"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// registerHTTPEventSink sets up an HTTP event reporter on the eventDispatcher.
func registerHTTPEventSink(d *eventDispatcher, endpoint, stage string) {
	if endpoint == "" || stage == "" {
		logrus.Warnf("http event sink: endpoint or stage is empty, skipping")
		return
	}

	var client *network.Client
	switch {
	case strings.HasPrefix(endpoint, "unix://") || strings.HasPrefix(endpoint, "unixgram://"):
		addr, err := network.ParseUnixAddr(endpoint)
		if err != nil {
			logrus.Warnf("http event sink: invalid unix endpoint %q: %v", endpoint, err)
			return
		}
		client = network.NewUnixClient(addr.Path, network.WithTimeout(1*time.Second))
	case strings.HasPrefix(endpoint, "tcp://"):
		addr, err := network.ParseTcpAddr(endpoint)
		if err != nil {
			logrus.Warnf("http event sink: invalid tcp endpoint %q: %v", endpoint, err)
			return
		}
		hostPort := fmt.Sprintf("%s:%d", addr.Host, addr.Port)
		client = network.NewTCPClient(hostPort, network.WithTimeout(1*time.Second))
	default:
		logrus.Warnf("http event sink: unsupported endpoint scheme %q", endpoint)
		return
	}

	handler := func(evt Event) {
		value := ""
		if evt.Kind == EventError {
			value = evt.Message
		}
		resp, err := client.Get("/notify").
			Query("stage", stage).
			Query("name", string(evt.Kind)).
			Query("value", value).
			Do(context.Background()) //nolint:bodyclose
		if err != nil {
			logrus.Warnf("http event sink: publish %s failed: %v", evt.Kind, err)
			return
		}
		network.CloseResponse(resp)
	}

	closer := func() {
		if err := client.Close(); err != nil {
			logrus.Warnf("http event sink: close failed: %v", err)
		}
	}

	d.addHandler(handler, closer)
}
