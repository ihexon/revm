//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package revm

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const eventQueueSize = 32

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
	events    chan Event
	once      sync.Once
	wg        sync.WaitGroup
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
	d.startLocked()
	d.reporters = append(d.reporters, r)
}

func (d *eventDispatcher) emit(sessionID string, runMode RunMode, kind EventKind, msg string, seq uint64) {
	if d == nil {
		return
	}
	evt := newEvent(sessionID, runMode, kind, msg, seq)
	d.enqueue(evt)
}

func newEvent(sessionID string, runMode RunMode, kind EventKind, msg string, seq uint64) Event {
	return Event{
		SessionID: sessionID,
		RunMode:   runMode,
		Kind:      kind,
		Message:   msg,
		Seq:       seq,
		Time:      time.Now(),
	}
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

func (d *eventDispatcher) startLocked() {
	d.once.Do(func() {
		d.events = make(chan Event, eventQueueSize)
		d.wg.Add(1)
		go d.run()
	})
}

func (d *eventDispatcher) run() {
	defer d.wg.Done()
	for evt := range d.events {
		d.mu.RLock()
		reporters := make([]EventReporter, len(d.reporters))
		copy(reporters, d.reporters)
		d.mu.RUnlock()

		for _, r := range reporters {
			r.Report(evt)
		}
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
	events := d.events
	d.events = nil
	reporters := make([]EventReporter, len(d.reporters))
	copy(reporters, d.reporters)
	d.reporters = nil
	d.mu.Unlock()

	if events != nil {
		close(events)
		d.wg.Wait()
	}

	for _, r := range reporters {
		r.Close()
	}
}
