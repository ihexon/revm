//go:build linux

// Socket probes run inside a real TSI VM against local host fixtures.
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"time"
)

const payload = "tsi-socket-ok\n"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func exchange(c net.Conn) error {
	defer c.Close()
	if err := c.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	if _, err := io.WriteString(c, payload); err != nil {
		return err
	}
	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(c, buf); err != nil {
		return err
	}
	if string(buf) != payload {
		return fmt.Errorf("unexpected echo %q", buf)
	}
	return nil
}

func run() error {
	switch os.Args[1] {
	case "client":
		c, err := net.DialTimeout(os.Args[2], os.Args[3], 5*time.Second)
		if err != nil {
			return err
		}
		return exchange(c)
	case "datagram":
		peer, err := net.ResolveUDPAddr(os.Args[2], os.Args[3])
		if err != nil {
			return err
		}
		c, err := net.ListenUDP(os.Args[2], nil)
		if err != nil {
			return err
		}
		defer c.Close()
		if err := c.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return err
		}
		if _, err := c.WriteToUDP([]byte(payload), peer); err != nil {
			return err
		}
		buf := make([]byte, 1024)
		n, _, err := c.ReadFromUDP(buf)
		if err != nil {
			return err
		}
		if string(buf[:n]) != payload {
			return fmt.Errorf("unexpected datagram %q", buf[:n])
		}
		return nil
	case "serve":
		listener, err := net.Listen(os.Args[2], os.Args[3])
		if err != nil {
			return err
		}
		defer listener.Close()
		fmt.Println("ready")
		for {
			c, err := listener.Accept()
			if err != nil {
				return err
			}
			_, err = io.Copy(c, c)
			c.Close()
			if err != nil {
				return err
			}
		}
	case "dns":
		resolver := net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, os.Args[2])
		}}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		addresses, err := resolver.LookupIP(ctx, "ip4", "tsi.test")
		if err != nil {
			return err
		}
		if len(addresses) != 1 || addresses[0].String() != "192.0.2.42" {
			return fmt.Errorf("unexpected DNS answer %v", addresses)
		}
		return nil
	case "unix":
		listener, err := net.Listen("unix", os.Args[2])
		if err != nil {
			return err
		}
		defer listener.Close()
		go func() {
			c, err := listener.Accept()
			if err == nil {
				io.Copy(c, c)
				c.Close()
			}
		}()
		c, err := net.DialTimeout("unix", os.Args[2], 5*time.Second)
		if err != nil {
			return err
		}
		return exchange(c)
	default:
		return fmt.Errorf("unknown probe %q", os.Args[1])
	}
}
