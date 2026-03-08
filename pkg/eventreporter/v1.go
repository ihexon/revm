package eventreporter

import (
	"context"
	"fmt"
	"linuxvm/pkg/librevm"
	"linuxvm/pkg/network"
	"time"

	"github.com/sirupsen/logrus"
)

type v1Reporter struct {
	client  *network.Client
	runMode string
}

// NewV1 creates a v1 EventReporter that sends GET /v1/event requests.
// Returns nil if the endpoint is invalid.
func NewV1(endpoint string, runMode librevm.RunMode) librevm.EventReporter {
	client := newClient(endpoint)
	if client == nil {
		return nil
	}
	return &v1Reporter{client: client, runMode: string(runMode)}
}

func (r *v1Reporter) Report(evt librevm.Event) {
	req := r.client.Get("/v1/event").
		Query("session_id", evt.SessionID).
		Query("mode", r.runMode).
		Query("kind", string(evt.Kind)).
		Query("seq", fmt.Sprintf("%d", evt.Seq)).
		Query("msg", evt.Message).
		Query("time", evt.Time.Format(time.RFC3339Nano))
	resp, err := req.Do(context.Background())
	if err != nil {
		logrus.Warnf("v1 event sink: publish %s failed: %v", evt.Kind, err)
		return
	}
	network.CloseResponse(resp)
}

func (r *v1Reporter) Close() {
	if err := r.client.Close(); err != nil {
		logrus.Warnf("v1 event sink: close failed: %v", err)
	}
}
