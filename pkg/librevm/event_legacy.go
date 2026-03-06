//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"context"
	"fmt"
	"linuxvm/pkg/network"
	"time"

	"github.com/sirupsen/logrus"
)

func (vm *VM) registerLegacyEventSink() {
	endpoint := vm.cfg.LegacyEventReportURL
	runMode := vm.cfg.RunMode
	sessionID := vm.cfg.SessionID

	if endpoint == "" {
		return
	}
	if runMode == "" {
		logrus.Warnf("legacy event sink: run mode is empty, skip reporting to %q", endpoint)
		return
	}

	client := newEventSinkClient(endpoint)
	if client == nil {
		return
	}

	handler := func(evt Event) {
		req := client.Get("/legacy/event").
			Query("session_id", sessionID).
			Query("mode", string(runMode)).
			Query("kind", string(evt.Kind)).
			Query("seq", fmt.Sprintf("%d", evt.Seq)).
			Query("msg", evt.Message).
			Query("time", evt.Time.Format(time.RFC3339Nano))
		resp, err := req.Do(context.Background()) //nolint:bodyclose
		if err != nil {
			logrus.Warnf("legacy event sink: publish %s failed: %v", evt.Kind, err)
			return
		}
		network.CloseResponse(resp)
	}

	closer := func() {
		if err := client.Close(); err != nil {
			logrus.Warnf("legacy event sink: close failed: %v", err)
		}
	}
	vm.eventDispatcher.addHandler(SinkLegacy, handler, closer)
}
