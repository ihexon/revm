package service

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"linuxvm/pkg/execution"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServerOwnsUpgradedExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- serveGuestControl(ctx, listener, nil) }()
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprint(conn, "GET /v2/exec HTTP/1.1\r\nHost: guest\r\nConnection: Upgrade\r\nUpgrade: revm-exec-v2\r\n\r\n")
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil || response.StatusCode != 101 {
		t.Fatalf("upgrade: %v %v", response, err)
	}
	output, writer := io.Pipe()
	defer output.Close()
	defer writer.Close()
	done := make(chan error, 1)
	clientCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	go func() {
		_, err := execution.Execute(clientCtx, conn, execution.Request{Version: 2, InstanceID: guestInstance, RequestID: "shutdown", Argv: []string{"sh", "-c", "trap '' TERM; printf ready; exec sleep 100"}}, execution.Options{IO: execution.IO{Stdout: writer}})
		writer.CloseWithError(err)
		done <- err
	}()
	ready := make([]byte, 5)
	if _, err := io.ReadFull(output, ready); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-served:
	case <-time.After(6 * time.Second):
		t.Fatal("server did not wait and stop execution")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled execution reported success")
		}
	case <-time.After(time.Second):
		t.Fatal("execution connection retained after server exit")
	}
}

func TestControlListenerFailure(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listener.Close()
	if err := serveGuestControl(context.Background(), listener, nil); err == nil {
		t.Fatal("listener failure ignored")
	}
}

func TestRepeatedRequestIDOnSeparateConnections(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- serveGuestControl(ctx, listener, nil) }()
	defer func() { cancel(); <-served }()
	for range 2 {
		conn, err := net.Dial("tcp", listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprint(conn, "GET /v2/exec HTTP/1.1\r\nHost: guest\r\nConnection: Upgrade\r\nUpgrade: revm-exec-v2\r\n\r\n")
		response, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil || response.StatusCode != 101 {
			t.Fatalf("upgrade: %v %v", response, err)
		}
		_, err = execution.Execute(ctx, conn, execution.Request{Version: 2, InstanceID: guestInstance, RequestID: "same-id", Argv: []string{"true"}}, execution.Options{})
		conn.Close()
		if err != nil {
			t.Fatal("new connection was rejected by request history", err)
		}
	}
}
