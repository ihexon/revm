//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"sync"
	"sync/atomic"
	"testing"
)

type countingSink struct {
	published atomic.Int64
	closed    atomic.Int64
}

func (s *countingSink) Publish(Event) {
	s.published.Add(1)
}

func (s *countingSink) Close() error {
	s.closed.Add(1)
	return nil
}

func TestEventDispatcherCloseIdempotent(t *testing.T) {
	var d eventDispatcher
	sink := &countingSink{}
	d.addSink(sink)

	d.close()
	d.close()

	if got := sink.closed.Load(); got != 1 {
		t.Fatalf("close count = %d, want 1", got)
	}
}

func TestEventDispatcherConcurrentPublishAndClose(t *testing.T) {
	var d eventDispatcher
	sink := &countingSink{}
	d.addSink(sink)

	var wg sync.WaitGroup
	for i := 0; i < 128; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.publish(EventVMStarting, "start", "vm", 1)
		}()
	}

	d.close()
	wg.Wait()

	if got := sink.closed.Load(); got != 1 {
		t.Fatalf("close count = %d, want 1", got)
	}
}
