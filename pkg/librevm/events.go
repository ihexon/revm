//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"sync"
	"time"
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

// EventReporter consumes VM lifecycle events.
type EventReporter interface {
	Report(evt Event)
	Close()
}

type eventDispatcher struct {
	mu        sync.RWMutex
	reporters []EventReporter
	closed    bool
}

func (d *eventDispatcher) addReporter(r EventReporter) {
	if d == nil || r == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		r.Close()
		return
	}
	d.reporters = append(d.reporters, r)
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
	for _, r := range d.reporters {
		r.Report(evt)
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
	reporters := make([]EventReporter, len(d.reporters))
	copy(reporters, d.reporters)
	d.reporters = nil
	d.mu.Unlock()

	for _, r := range reporters {
		r.Close()
	}
}

func (vm *VM) emit(kind EventKind, msg string) {
	if vm == nil || vm.cfg == nil {
		return
	}
	vm.eventDispatcher.publish(vm.cfg.SessionID, vm.cfg.RunMode, kind, msg, vm.seq.Add(1))
}
