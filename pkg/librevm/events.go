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

// RoutedEvent resolves into a concrete wire event kind for the current mode.
type RoutedEvent interface {
	resolveForMode(runMode RunMode) (kind string, ok bool)
}

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

// OVMEventKind identifies an OVM-mode event.
type OVMEventKind string

const (
	EventOVMInitSuccess OVMEventKind = "Success"
	EventOVMPodmanReady OVMEventKind = "Ready"
	EventOVMError       OVMEventKind = "Error"
	EventOVMExit        OVMEventKind = "Exit"
)

func (k REVMEventKind) resolveForMode(runMode RunMode) (string, bool) {
	if runMode.IsOVM() {
		return "", false
	}
	return string(k), true
}

func (k OVMEventKind) resolveForMode(runMode RunMode) (string, bool) {
	if !runMode.IsOVM() {
		return "", false
	}
	switch runMode {
	case ModeOVMRun:
		switch k {
		case EventOVMPodmanReady, EventOVMError, EventOVMExit:
			return string(k), true
		}
	case ModeOVMInit:
		switch k {
		case EventOVMInitSuccess, EventOVMError, EventOVMExit:
			return string(k), true
		}
	}
	return "", false
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

type eventProtocol int

const (
	eventProtocolREVM eventProtocol = iota
	eventProtocolOVM
)

func eventProtocolForRunMode(runMode RunMode) eventProtocol {
	if runMode.IsOVM() {
		return eventProtocolOVM
	}
	return eventProtocolREVM
}

func buildNotifyRequest(
	client *network.Client,
	protocol eventProtocol,
	runMode RunMode,
	sessionID string,
	evt Event,
) (*network.Request, bool) {
	switch protocol {
	case eventProtocolOVM:
		value := ""
		if evt.Kind == string(EventOVMError) {
			value = evt.Message
		}
		return client.Get("/notify").
			Query("stage", string(runMode)).
			Query("name", evt.Kind).
			Query("value", value), true
	default:
		return client.Get("/notify").
			Query("session_id", sessionID).
			Query("mode", string(runMode)).
			Query("kind", string(evt.Kind)).
			Query("seq", fmt.Sprintf("%d", evt.Seq)).
			Query("msg", evt.Message).
			Query("time", evt.Time.Format(time.RFC3339Nano)), true
	}
}

// registerHTTPEventSink sets up an HTTP event reporter on the eventDispatcher.
func registerHTTPEventSink(d *eventDispatcher, endpoint string, runMode RunMode, sessionID string) {
	if endpoint == "" || string(runMode) == "" {
		logrus.Warnf("http event sink: endpoint or runMode is empty, skipping")
		return
	}

	client := newEventSinkClient(endpoint)
	if client == nil {
		return
	}
	protocol := eventProtocolForRunMode(runMode)

	handler := func(evt Event) {
		req, ok := buildNotifyRequest(client, protocol, runMode, sessionID, evt)
		if !ok {
			return
		}

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
	d.addHandler(handler, closer)
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
