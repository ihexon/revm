//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"context"
	"fmt"
	"linuxvm/pkg/network"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// REVMEventKind identifies a common VM lifecycle event.
type REVMEventKind string

const (
	EventStopping REVMEventKind = "stopping"
	EventStopped  REVMEventKind = "stopped"
	EventError    REVMEventKind = "error"

	// Service startup phases.
	EventNetworkStarting     REVMEventKind = "network_starting"
	EventIgnitionStarting    REVMEventKind = "ignition_starting"
	EventManagementStarting  REVMEventKind = "management_starting"
	EventPodmanProxyStarting REVMEventKind = "podman_proxy_starting"

	// Readiness milestones.
	EventNetworkReady REVMEventKind = "network_ready"
	EventSSHReady     REVMEventKind = "ssh_ready"
	EventPodmanReady  REVMEventKind = "podman_ready"

	// VM process lifecycle.
	EventVMStarting REVMEventKind = "vm_starting"
)

// Event represents a single VM lifecycle event.
type Event struct {
	RunMode   RunMode   `json:"runMode"`
	Kind      string    `json:"kind"`
	Message   string    `json:"message,omitempty"`
	SessionID string    `json:"sessionID,omitempty"`
	Seq       uint64    `json:"seq,omitempty"`
	Time      time.Time `json:"time"`
}

type eventDispatcher struct {
	mu       sync.RWMutex
	handlers []func(Event)
	closers  []func()
	closed   bool
}

func (d *eventDispatcher) addHandler(fn func(Event), closer func()) {
	if d == nil || fn == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		if closer != nil {
			closer()
		}
		return
	}
	d.handlers = append(d.handlers, fn)
	if closer != nil {
		d.closers = append(d.closers, closer)
	}
}

func (d *eventDispatcher) publish(sessionID string, runMode RunMode, kind string, msg string, seq uint64) {
	if d == nil {
		return
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return
	}

	evt := Event{
		SessionID: sessionID,
		RunMode:   runMode,
		Kind:      kind,
		Message:   msg,
		Seq:       seq,
		Time:      time.Now(),
	}
	for _, fn := range d.handlers {
		fn(evt)
	}
}

func (d *eventDispatcher) close() {
	if d == nil {
		return
	}

	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.closed = true
	closers := make([]func(), len(d.closers))
	copy(closers, d.closers)
	d.handlers = nil
	d.closers = nil
	d.mu.Unlock()

	for _, fn := range closers {
		fn()
	}
}

func buildNotifyRequest(
	client *network.Client,
	runMode RunMode,
	sessionID string,
	evt Event,
) *network.Request {
	return client.Get("/notify").
		Query("session_id", sessionID).
		Query("mode", string(runMode)).
		Query("kind", evt.Kind).
		Query("seq", fmt.Sprintf("%d", evt.Seq)).
		Query("msg", evt.Message).
		Query("time", evt.Time.Format(time.RFC3339Nano))
}

// registerHTTPEventSink sets up an HTTP event reporter on the eventDispatcher.
func (vm *VM) registerHTTPEventSink() {
	endpoint := vm.cfg.ReportURL
	runMode := vm.cfg.RunMode
	sessionID := vm.cfg.SessionID

	if endpoint == "" {
		return
	}

	if runMode == "" {
		logrus.Warnf("http event sink: run mode is empty, can not send event to %q", endpoint)
		return
	}

	client := newEventSinkClient(endpoint)
	if client == nil {
		return
	}

	handler := func(evt Event) {
		req := buildNotifyRequest(client, runMode, sessionID, evt)
		resp, err := req.Do(context.Background()) //nolint:bodyclose
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
	vm.eventDispatcher.addHandler(handler, closer)
}

func newEventSinkClient(endpoint string) *network.Client {
	switch {
	case strings.HasPrefix(endpoint, "unix://") || strings.HasPrefix(endpoint, "unixgram://"):
		addr, err := network.ParseUnixAddr(endpoint)
		if err != nil {
			logrus.Warnf("http event sink: invalid unix endpoint %q: %v", endpoint, err)
			return nil
		}
		return network.NewUnixClient(addr.Path, network.WithTimeout(1*time.Second))
	case strings.HasPrefix(endpoint, "tcp://"):
		addr, err := network.ParseTcpAddr(endpoint)
		if err != nil {
			logrus.Warnf("http event sink: invalid tcp endpoint %q: %v", endpoint, err)
			return nil
		}
		hostPort := fmt.Sprintf("%s:%d", addr.Host, addr.Port)
		return network.NewTCPClient(hostPort, network.WithTimeout(1*time.Second))
	default:
		logrus.Warnf("http event sink: unsupported endpoint scheme %q", endpoint)
		return nil
	}
}
