//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package guestcontrol

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"linuxvm/pkg/protocol"
	"net/http"
	"time"
)

type Target struct {
	CID        uint32
	Port       uint32
	UnixSocket string
}

type ProcessOutput struct {
	StdoutPipeReader *io.PipeReader
	StderrPipeReader *io.PipeReader
	ErrChan          chan error
}

func Shutdown(ctx context.Context, target Target) error {
	client := newClient(target, true)
	defer client.Close()
	resp, err := client.Post("/v1/shutdown").Do(ctx)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("guest shutdown returned %d: %s", resp.StatusCode, body)
	}
	return nil
}

func DefaultTarget() Target {
	return Target{CID: define.GuestVSockCID, Port: define.GuestControlPort}
}

func GuestExec(ctx context.Context, target Target, bin string, args ...string) (*ProcessOutput, error) {
	return GuestExecRequest(ctx, target, protocol.GuestControlRequest{
		SchemaVersion: protocol.GuestControlVersion,
		Bin:           bin,
		Args:          args,
	})
}

func GuestExecWith(ctx context.Context, target Target, stdin io.Reader, bin string, args ...string) (*ProcessOutput, error) {
	var data []byte
	if stdin != nil {
		var err error
		data, err = io.ReadAll(stdin)
		if err != nil {
			return nil, err
		}
	}
	return GuestExecRequest(ctx, target, protocol.GuestControlRequest{
		SchemaVersion: protocol.GuestControlVersion,
		Bin:           bin,
		Args:          args,
		Stdin:         data,
	})
}

func GuestExecRequest(ctx context.Context, target Target, request protocol.GuestControlRequest) (*ProcessOutput, error) {
	client := newClient(target, false)
	request.SchemaVersion = protocol.GuestControlVersion
	reqBody, err := json.Marshal(request)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	var resp *http.Response
	var lastErr error
	ready := time.NewTimer(10 * time.Second)
	defer ready.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		resp, lastErr = client.Post("/v1/exec").JSON().Body(bytes.NewReader(reqBody)).Do(ctx)
		if lastErr == nil {
			break
		}
		select {
		case <-ctx.Done():
			_ = client.Close()
			return nil, fmt.Errorf("guest control exec: %w", context.Cause(ctx))
		case <-ready.C:
			_ = client.Close()
			return nil, fmt.Errorf("guest control exec: %w", lastErr)
		case <-ticker.C:
		}
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		_ = client.Close()
		return nil, fmt.Errorf("guest control exec returned %d: %s", resp.StatusCode, body)
	}

	stdoutReader, stdoutWriter := io.Pipe()
	stderrReader, stderrWriter := io.Pipe()
	errChan := make(chan error, 1)
	streamDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = stdoutReader.CloseWithError(context.Cause(ctx))
			_ = stderrReader.CloseWithError(context.Cause(ctx))
		case <-streamDone:
		}
	}()
	go func() {
		defer close(streamDone)
		defer client.Close()
		defer resp.Body.Close()
		defer stdoutWriter.Close()
		defer stderrWriter.Close()
		sendErr := func(err error) {
			errChan <- err
		}
		dec := json.NewDecoder(bufio.NewReader(resp.Body))
		var exitCode *int
		for {
			var frame protocol.GuestControlFrame
			if err := dec.Decode(&frame); err != nil {
				if err == io.EOF {
					if exitCode == nil {
						sendErr(errors.New("guest control stream ended without exit status"))
						return
					}
					if *exitCode != 0 {
						sendErr(fmt.Errorf("guest command exited with code %d", *exitCode))
						return
					}
					sendErr(nil)
					return
				}
				sendErr(err)
				return
			}
			switch frame.Type {
			case protocol.GuestControlStdout:
				if _, err := stdoutWriter.Write(frame.Data); err != nil {
					sendErr(err)
					return
				}
			case protocol.GuestControlStderr:
				if _, err := stderrWriter.Write(frame.Data); err != nil {
					sendErr(err)
					return
				}
			case protocol.GuestControlExit:
				if frame.ExitCode == nil {
					sendErr(errors.New("guest control exit frame has no exit code"))
					return
				}
				exitCode = frame.ExitCode
			case protocol.GuestControlError:
				sendErr(fmt.Errorf("guest command failed: %s", frame.Error))
				return
			}
		}
	}()
	return &ProcessOutput{StdoutPipeReader: stdoutReader, StderrPipeReader: stderrReader, ErrChan: errChan}, nil
}

func newClient(target Target, bounded bool) *network.Client {
	if target.UnixSocket != "" {
		if bounded {
			return network.NewUnixClient(target.UnixSocket, network.WithTimeout(2*time.Second))
		}
		return network.NewUnixClient(target.UnixSocket, network.WithTimeout(0), network.WithKeepAlive(false))
	}
	if target.CID == 0 {
		target.CID = define.GuestVSockCID
	}
	if target.Port == 0 {
		target.Port = define.GuestControlPort
	}
	if bounded {
		return network.NewVSockClient(target.CID, target.Port, network.WithTimeout(2*time.Second))
	}
	return network.NewVSockClient(target.CID, target.Port, network.WithTimeout(0), network.WithKeepAlive(false))
}
