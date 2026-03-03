//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestEventDispatcherCloseIdempotent(t *testing.T) {
	var d eventDispatcher
	var closed atomic.Int64
	d.addHandler(func(Event) {}, func() { closed.Add(1) })

	d.close()
	d.close()

	if got := closed.Load(); got != 1 {
		t.Fatalf("close count = %d, want 1", got)
	}
}

func TestEventDispatcherConcurrentPublishAndClose(t *testing.T) {
	var d eventDispatcher
	var published atomic.Int64
	var closed atomic.Int64
	d.addHandler(func(Event) { published.Add(1) }, func() { closed.Add(1) })

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

	if got := closed.Load(); got != 1 {
		t.Fatalf("close count = %d, want 1", got)
	}
}
