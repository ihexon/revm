package event

import (
	"context"
	"linuxvm/pkg/network"
	"time"
)

// global is the process-wide reporter singleton, set once at startup via Setup.
var global *Reporter

type Reporter struct {
	client *network.Client
	stage  StageName
}

// Setup initializes the global reporter. Safe to call with empty endpoint (becomes no-op).
func Setup(endpoint string, stageName StageName) {
	if endpoint == "" {
		return
	}
	if stageName == "" {
		return
	}

	addr, err := network.ParseUnixAddr(endpoint)
	if err != nil {
		return
	}

	global = &Reporter{
		client: network.NewUnixClient(addr.Path, network.WithTimeout(1*time.Second)),
		stage:  stageName,
	}
}

// Emit sends an event via the global reporter using the stage set at Setup. Nil-safe.
func Emit(name EvtName) {
	if global == nil {
		return
	}
	_ = global.sendEvent(global.stage, name, "")
}

// EmitError sends an error event if err is non-nil. Nil-safe.
func EmitError(err error) {
	if err == nil || global == nil {
		return
	}
	_ = global.sendEvent(global.stage, Error, err.Error())
}

func (r *Reporter) sendEvent(stage StageName, evtName EvtName, value string) error {
	if r == nil || r.client == nil {
		return nil
	}

	resp, err := r.client.Get("/notify").
		Query("stage", string(stage)).
		Query("name", string(evtName)).
		Query("value", value).
		Do(context.Background())
	if err != nil {
		return err
	}

	network.CloseResponse(resp)
	return nil
}

// Close closes the global reporter's HTTP client.
func Close() error {
	if global != nil && global.client != nil {
		return global.client.Close()
	}
	return nil
}
