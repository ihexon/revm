package eventreporter

import (
	"context"
	"linuxvm/pkg/librevm"
	"linuxvm/pkg/network"

	"github.com/sirupsen/logrus"
)

type legacyReporter struct {
	client  *network.Client
	runMode string // raw runMode, mapped by evtProxy at report time
}

// NewLegacyReporter creates a legacy EventReporter that sends GET /notify requests.
// Returns nil if the endpoint is invalid.
func NewLegacyReporter(endpoint string, runMode librevm.RunMode) librevm.EventReporter {
	client := newClient(endpoint)
	if client == nil {
		return nil
	}
	return &legacyReporter{client: client, runMode: string(runMode)}
}

func (r *legacyReporter) Report(evt librevm.Event) {
	stage, kind := r.evtProxy(r.runMode, evt.Kind)
	value := ""
	if kind == librevm.EventError {
		value = evt.Message
	}

	req := r.client.Get("/notify").
		Query("stage", stage).
		Query("name", string(kind)).
		Query("value", value)
	resp, err := req.Do(context.Background())
	if err != nil {
		logrus.Warnf("legacy event sink: publish %s failed: %v", evt.Kind, err)
		return
	}
	network.CloseResponse(resp)
}

func (r *legacyReporter) Close() {
	if err := r.client.Close(); err != nil {
		logrus.Warnf("legacy event sink: close failed: %v", err)
	}
}

// for compatibility ovm-js
//
// runMode mapping:
//
//	cfggen -> init,
//	container -> start
//
// event kind mapping:
//
//	stopped -> Exit,
//	podmanReady -> Ready,
//	error -> Error,
//	success -> Success,
//	exit -> Exit
func (r *legacyReporter) evtProxy(runMode string, kind librevm.EventKind) (string, librevm.EventKind) {
	switch runMode {
	case string(librevm.ModeCfgGen):
		runMode = "init"
	case string(librevm.ModeContainer):
		runMode = "start"
	}
	switch kind {
	case librevm.EventStopped:
		kind = "Exit"
	case librevm.EventPodmanReady:
		kind = "Ready"
	case librevm.EventError:
		kind = "Error"
	case librevm.EventSuccess:
		kind = "Success"
	case librevm.EventExit:
		kind = "Exit"
	}
	return runMode, kind
}
