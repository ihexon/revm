//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

// vmOptions holds per-VM behavioural options set via [OptionFn] functions.
type vmOptions struct {
	dispatcher eventDispatcher
}

// OptionFn configures optional VM behaviour.
type OptionFn func(*vmOptions)

// WithEventHandler registers a callback that receives lifecycle events for the
// VM. The handler is called synchronously; long-running work should be
// dispatched to a goroutine by the caller.
func WithEventHandler(fn func(Event)) OptionFn {
	return WithEventSink(EventSinkFunc(fn))
}

// WithEventSink registers a sink for VM lifecycle events.
func WithEventSink(sink EventSink) OptionFn {
	return func(o *vmOptions) {
		o.dispatcher.addSink(sink)
	}
}
