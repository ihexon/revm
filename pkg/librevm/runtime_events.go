//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"sync"
	"time"
)

// EventSink receives VM lifecycle events.
type EventSink interface {
	Publish(evt Event)
	Close() error
}

// EventSinkFunc adapts a function to an EventSink.
type EventSinkFunc func(Event)

func (f EventSinkFunc) Publish(evt Event) {
	if f != nil {
		f(evt)
	}
}

func (f EventSinkFunc) Close() error { return nil }

type eventDispatcher struct {
	mu     sync.RWMutex
	sinks  []EventSink
	closed bool
}

func (d *eventDispatcher) publish(kind EventKind, msg, vmName string, seq uint64) {
	if d == nil {
		return
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return
	}

	evt := Event{
		Kind:    kind,
		Message: msg,
		VMName:  vmName,
		Seq:     seq,
		Time:    time.Now(),
	}
	for _, sink := range d.sinks {
		if sink != nil {
			sink.Publish(evt)
		}
	}
}

func (d *eventDispatcher) addSink(sink EventSink) {
	if d == nil || sink == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		_ = sink.Close()
		return
	}
	d.sinks = append(d.sinks, sink)
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
	sinks := append([]EventSink(nil), d.sinks...)
	d.sinks = nil
	d.mu.Unlock()

	for _, sink := range sinks {
		_ = sink.Close()
	}
}
