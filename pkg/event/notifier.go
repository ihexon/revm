package event

import (
	"context"
	"linuxvm/pkg/network"
	"time"
)

type ReportFuncKeyType string

var ReportFuncKey ReportFuncKeyType = "ReportFuncKey"

type Reporter struct {
	client *network.Client
}

func InitializeReporter(endpoint string) *Reporter {
	if endpoint == "" {
		return nil
	}

	addr, err := network.ParseUnixAddr(endpoint)
	if err != nil {
		return nil
	}

	return &Reporter{
		client: network.NewUnixClient(addr.Path, network.WithTimeout(1*time.Second)),
	}
}

// SendEventInInitLifeCycle sends an event during the `init` stage, distinguishing between error and non-error cases.
func (r *Reporter) SendEventInInitLifeCycle(ctx context.Context, evtName EvtName, errMsg string) error {
	if errMsg != "" {
		return r.sendEvent(ctx, Init, Error, errMsg)
	}
	return r.sendEvent(ctx, Init, evtName, errMsg)
}

// SendEventInRunLifeCycle sends an event during the `run` stage, distinguishing between error and non-error cases.
func (r *Reporter) SendEventInRunLifeCycle(ctx context.Context, evtName EvtName, errMsg string) error {
	if errMsg != "" {
		return r.sendEvent(ctx, Run, Error, errMsg)
	}
	return r.sendEvent(ctx, Run, evtName, errMsg)
}

func (r *Reporter) sendEvent(ctx context.Context, stage StageName, evtName EvtName, value string) error {
	if r == nil || r.client == nil {
		return nil
	}

	resp, err := r.client.Get("/notify").
		Query("stage", string(stage)).
		Query("name", string(evtName)).
		Query("value", value).
		Do(ctx)
	if err != nil {
		return err
	}

	network.CloseResponse(resp)
	return nil
}

// Close closes the reporter's HTTP client
func (r *Reporter) Close() error {
	if r != nil && r.client != nil {
		return r.client.Close()
	}
	return nil
}

func GetReporterFromCtx(ctx context.Context) *Reporter {
	if ctx == nil {
		return nil
	}

	v := ctx.Value(ReportFuncKey)
	if v == nil {
		return nil
	}

	fn, ok := v.(*Reporter)
	if !ok {
		return nil
	}

	return fn
}
