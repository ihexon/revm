package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/execution"
	"linuxvm/pkg/network"
	"time"
)

func ReportCommandResult(result execution.Result) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client := network.NewVSockClient(2, define.DefaultVSockPort)
	defer client.Close()
	b, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, status, err := client.Post("/v1/result").JSON().Body(bytes.NewReader(b)).DoAndRead(ctx)
	if err != nil {
		return err
	}
	if status != 204 {
		return fmt.Errorf("report command result: status %d", status)
	}
	return nil
}
