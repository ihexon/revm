//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const eventQueueSize = 32

// Event represents a VM lifecycle event.
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

// eventDispatcher keeps event reporting asynchronous so an unavailable sink
// cannot hold up the VM. revm has one optional sink, so a single queue is
// enough; supporting a reporter registry here only adds lifecycle states.
type eventDispatcher struct {
	mu       sync.RWMutex
	reporter EventReporter
	events   chan Event
	done     chan struct{}
	closed   bool
}

func (d *eventDispatcher) start(r EventReporter) {
	if d == nil || r == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || d.reporter != nil {
		r.Close()
		return
	}
	d.reporter = r
	d.events = make(chan Event, eventQueueSize)
	d.done = make(chan struct{})
	go d.run(d.events, d.reporter, d.done)
}

func (d *eventDispatcher) emit(sessionID string, runMode RunMode, kind EventKind, msg string, seq uint64) {
	if d == nil {
		return
	}
	d.enqueue(Event{
		SessionID: sessionID,
		RunMode:   runMode,
		Kind:      kind,
		Message:   msg,
		Seq:       seq,
		Time:      time.Now(),
	})
}

func (d *eventDispatcher) enqueue(evt Event) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed || d.events == nil {
		return
	}
	select {
	case d.events <- evt:
	default:
		logrus.Warnf("event sink queue full, dropping event %s", evt.Kind)
	}
}

func (d *eventDispatcher) run(events <-chan Event, reporter EventReporter, done chan<- struct{}) {
	defer close(done)
	for evt := range events {
		reporter.Report(evt)
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
	events, done, reporter := d.events, d.done, d.reporter
	d.events = nil
	d.done = nil
	d.mu.Unlock()

	if events != nil {
		close(events)
		<-done
	}
	if reporter != nil {
		reporter.Close()
	}
	d.mu.Lock()
	d.reporter = nil
	d.mu.Unlock()
}
