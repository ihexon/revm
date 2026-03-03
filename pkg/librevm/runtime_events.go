//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"sync"
	"time"
)

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
