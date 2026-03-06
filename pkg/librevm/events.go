//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"fmt"
	"linuxvm/pkg/network"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Event represents a single VM lifecycle event.
type Event struct {
	RunMode   RunMode   `json:"runMode"`
	Kind      EventKind `json:"kind"`
	Message   string    `json:"message,omitempty"`
	SessionID string    `json:"sessionID,omitempty"`
	Seq       uint64    `json:"seq,omitempty"`
	Time      time.Time `json:"time"`
}

// SinkKind identifies the type of event sink.
type SinkKind int

const (
	SinkV1     SinkKind = iota // v1 event reporting
	SinkLegacy                 // legacy event reporting
)

// EventProxy transforms an Event per sink kind.
// Implementations can return different Event values for v1 vs legacy.
type EventProxy func(sink SinkKind, evt Event) Event

type sinkEntry struct {
	kind SinkKind
	fn   func(Event)
}

type eventDispatcher struct {
	mu      sync.RWMutex
	proxy   EventProxy
	sinks   []sinkEntry
	closers []func()
	closed  bool
}

func (d *eventDispatcher) addHandler(kind SinkKind, fn func(Event), closer func()) {
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
	d.sinks = append(d.sinks, sinkEntry{kind: kind, fn: fn})
	if closer != nil {
		d.closers = append(d.closers, closer)
	}
}

func (d *eventDispatcher) publish(sessionID string, runMode RunMode, kind EventKind, msg string, seq uint64) {
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
	for _, s := range d.sinks {
		out := evt
		if d.proxy != nil {
			out = d.proxy(s.kind, evt)
		}
		s.fn(out)
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
	d.sinks = nil
	d.closers = nil
	d.mu.Unlock()

	for _, fn := range closers {
		fn()
	}
}

func (vm *VM) emit(kind EventKind, msg string) {
	if vm == nil || vm.cfg == nil {
		return
	}
	vm.eventDispatcher.publish(vm.cfg.SessionID, vm.cfg.RunMode, kind, msg, vm.seq.Add(1))
}

func newEventSinkClient(endpoint string) *network.Client {
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
