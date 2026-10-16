//go:build linux

// This probe runs inside an ordinary container, with seccomp disabled so a
// denied vsock syscall cannot mask an exposed management endpoint.
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	yamux "github.com/libp2p/go-yamux/v5"
	"github.com/mdlayher/vsock"
	"linuxvm/pkg/define"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "serve" {
		if err := http.ListenAndServe(":8080", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "podman-port-ok")
		})); err != nil {
			panic(err)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "trusted" {
		client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", define.GuestPodmanSocket)
			},
		}}
		response, err := client.Get("http://podman/_ping")
		if err != nil {
			panic(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil || response.StatusCode != http.StatusOK || string(body) != "OK" {
			panic("trusted Supervisor cannot reach Podman")
		}
		fmt.Println("trusted Supervisor can reach Podman")
		return
	}
	if _, err := os.Stat(define.GuestPodmanSocket); !os.IsNotExist(err) {
		panic(fmt.Sprintf("guest engine socket is visible in container: %v", err))
	}
	for _, cid := range []uint32{vsock.Local, vsock.Host, 3} {
		for _, port := range []uint32{define.GuestControlPort, define.GuestPodmanPort} {
			probe(cid, port)
		}
	}
}

func probe(cid, port uint32) {
	c, err := vsock.Dial(cid, port, nil)
	if err != nil {
		fmt.Printf("blocked %d:%d: %v\n", cid, port, err)
		return
	}
	defer c.Close()
	var stream net.Conn = c
	if port == define.GuestPodmanPort {
		session, err := yamux.Client(c, nil, nil)
		if err != nil {
			panic(err)
		}
		defer session.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		stream, err = session.Open(ctx)
		if err != nil {
			fmt.Printf("blocked %d:%d: %v\n", cid, port, err)
			return
		}
	}
	_ = stream.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = io.WriteString(stream, "GET /_ping HTTP/1.1\r\nHost: probe\r\nConnection: close\r\n\r\n")
	response, err := http.ReadResponse(bufio.NewReader(stream), nil)
	if err == nil {
		response.Body.Close()
		panic(fmt.Sprintf("management endpoint reachable at %d:%d: %s", cid, port, response.Status))
	}
	fmt.Printf("blocked %d:%d: %v\n", cid, port, err)
}
