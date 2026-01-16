package event

import (
	"context"
	"linuxvm/pkg/network"
	"net/url"
	"time"
)

type ReportFuncKeyType string

var ReportFuncKey ReportFuncKeyType = "ReportFuncKey"

type Reporter struct {
	client *network.UnixHTTPClient
}

func InitializeReporter(endpoint string) *Reporter {
	if endpoint == "" {
		return nil
	}
	return &Reporter{client: network.NewUnixHTTPClient(endpoint, 1*time.Second)}
}

func (r *Reporter) SendEventInInitLifeCycle(ctx context.Context, evtName EvtName, errMsg string) error {
	if errMsg != "" {
		return r.SendEvent(ctx, Init, Error, errMsg)
	}
	return r.SendEvent(ctx, Init, evtName, errMsg)
}

func (r *Reporter) SendEventInRunLifeCycle(ctx context.Context, evtName EvtName, errMsg string) error {
	if errMsg != "" {
		return r.SendEvent(ctx, Run, Error, errMsg)
	}
	return r.SendEvent(ctx, Run, evtName, errMsg)
}

func (r *Reporter) SendEvent(ctx context.Context, stage StageName, evtName EvtName, value string) error {
	if r.client == nil {
		return nil
	}

	// 1. 构造局部 query，不会污染 r.client 的内部状态
	query := url.Values{}
	query.Set("stage", string(stage))
	query.Set("name", string(evtName))
	query.Set("value", value)

	// 2. 使用专门设计的 GetWithQuery 方法
	resp, err := r.client.GetWithQuery(ctx, "/notify", query)
	if err != nil {
		return err
	}

	defer r.client.CloseResponse(resp)
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
