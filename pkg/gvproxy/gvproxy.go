package gvproxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/containers/gvisor-tap-vsock/pkg/notification"
	"github.com/containers/gvisor-tap-vsock/pkg/transport"
	"github.com/containers/gvisor-tap-vsock/pkg/types"
	"github.com/containers/gvisor-tap-vsock/pkg/virtualnetwork"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const (
	defaultMTU        = 1500
	gatewayIP         = "192.168.127.1"
	hostIP            = "192.168.127.254"
	gatewayMACAddress = "5a:94:ef:e4:0c:dd"
	guestMACAddress   = "5a:94:ef:e4:0c:ee"
)

type Config struct {
	ControlAddr string
	NetAddr     string
	NotifyAddr  string
	Stack       types.Configuration
}

func NewConfig(vmc *define.Machine) (*Config, error) {
	if vmc.GVPCtlAddr == "" {
		return nil, errors.New("gvproxy control address is empty")
	}
	if vmc.GVPVNetAddr == "" {
		return nil, errors.New("gvproxy network address is empty")
	}
	if vmc.GVPNotifyAddr == "" {
		return nil, errors.New("gvproxy notification address is empty")
	}
	if err := validateUnixAddr(vmc.GVPCtlAddr, "unix"); err != nil {
		return nil, fmt.Errorf("invalid gvproxy control address: %w", err)
	}
	if err := validateUnixAddr(vmc.GVPVNetAddr, "unixgram"); err != nil {
		return nil, fmt.Errorf("invalid gvproxy network address: %w", err)
	}
	if err := validateUnixAddr(vmc.GVPNotifyAddr, "unix"); err != nil {
		return nil, fmt.Errorf("invalid gvproxy notification address: %w", err)
	}

	port, err := network.GetAvailablePort(define.SSHLocalForwardListenPort)
	if err != nil {
		return nil, fmt.Errorf("get available port for ssh forwarding: %w", err)
	}

	sshLocalForwardAddr := fmt.Sprintf("%s:%d", define.LocalHost, port)
	_, sshPortStr, err := net.SplitHostPort(vmc.SSHInfo.GuestSSHServerListenAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid guest ssh listen address: %w", err)
	}
	sshServerGuestAddr := net.JoinHostPort(define.GuestIP, sshPortStr)

	logrus.Infof("configuring local port forwarding from %s to %s", sshLocalForwardAddr, sshServerGuestAddr)
	vmc.SSHInfo.HostSSHProxyListenAddr = sshLocalForwardAddr

	return &Config{
		ControlAddr: vmc.GVPCtlAddr,
		NetAddr:     vmc.GVPVNetAddr,
		NotifyAddr:  vmc.GVPNotifyAddr,
		Stack: types.Configuration{
			MTU:               defaultMTU,
			Subnet:            "192.168.127.0/24",
			GatewayIP:         gatewayIP,
			DeviceIP:          define.GuestIP,
			HostIP:            hostIP,
			GatewayMacAddress: gatewayMACAddress,
			DNS: []types.Zone{
				internalZone("containers.internal."),
				internalZone("docker.internal."),
				internalZone("revm.internal."),
			},
			Forwards: map[string]string{
				sshLocalForwardAddr: sshServerGuestAddr,
			},
			NAT: map[string]string{
				hostIP: define.LocalHost,
			},
			GatewayVirtualIPs: []string{hostIP},
			DHCPStaticLeases: map[string]string{
				define.GuestIP: guestMACAddress,
			},
			Protocol: types.VfkitProtocol,
		},
	}, nil
}

func Run(ctx context.Context, vmc *define.Machine) error {
	config, err := NewConfig(vmc)
	if err != nil {
		return fmt.Errorf("init gvproxy config: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)

	vn, err := virtualnetwork.New(&config.Stack)
	if err != nil {
		return fmt.Errorf("create virtual network: %w", err)
	}

	if err := startNotificationListener(ctx, g, config.NotifyAddr, vmc); err != nil {
		return err
	}
	if err := startControlAPI(ctx, g, config.ControlAddr, vn); err != nil {
		return err
	}
	if err := startGatewayAPI(ctx, g, config.Stack.GatewayIP, vn); err != nil {
		return err
	}
	if err := startUnixgramNet(ctx, g, config.NetAddr, vn); err != nil {
		return err
	}

	notifyPath, _ := unixSocketPath(config.NotifyAddr)
	notificationSender := notification.NewNotificationSender(notifyPath)
	vn.SetNotificationSender(notificationSender)
	g.Go(func() error {
		notificationSender.Start(ctx)
		return nil
	})
	notificationSender.Send(types.NotificationMessage{NotificationType: types.Ready})

	return g.Wait()
}

func internalZone(name string) types.Zone {
	return types.Zone{
		Name: name,
		Records: []types.Record{
			{Name: "gateway", IP: net.ParseIP(gatewayIP)},
			{Name: "host", IP: net.ParseIP(hostIP)},
		},
	}
}

func startControlAPI(ctx context.Context, g *errgroup.Group, addr string, vn *virtualnetwork.VirtualNetwork) error {
	ln, err := transport.Listen(addr)
	if err != nil {
		return fmt.Errorf("control listen error: %w", err)
	}
	httpServe(ctx, g, ln, vn.Mux())
	return nil
}

func startGatewayAPI(ctx context.Context, g *errgroup.Group, gateway string, vn *virtualnetwork.VirtualNetwork) error {
	ln, err := vn.Listen("tcp", net.JoinHostPort(gateway, "80"))
	if err != nil {
		return fmt.Errorf("gateway listen error: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/services/forwarder/all", vn.Mux())
	mux.Handle("/services/forwarder/expose", vn.Mux())
	mux.Handle("/services/forwarder/unexpose", vn.Mux())
	httpServe(ctx, g, ln, mux)
	return nil
}

func startUnixgramNet(ctx context.Context, g *errgroup.Group, addr string, vn *virtualnetwork.VirtualNetwork) error {
	conn, err := transport.ListenUnixgram(addr)
	if err != nil {
		return fmt.Errorf("unixgram listen error: %w", err)
	}

	g.Go(func() error {
		<-ctx.Done()
		if err := conn.Close(); err != nil {
			logrus.Warnf("close gvproxy unixgram socket %s: %v", addr, err)
		}
		path, err := unixSocketPath(addr)
		if err == nil {
			_ = os.Remove(path)
		}
		return nil
	})

	g.Go(func() error {
		vfkitConn, err := transport.AcceptVfkit(conn)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("unixgram accept error: %w", err)
		}
		return vn.AcceptVfkit(ctx, vfkitConn)
	})

	return nil
}

func startNotificationListener(ctx context.Context, g *errgroup.Group, addr string, vmc *define.Machine) error {
	path, err := unixSocketPath(addr)
	if err != nil {
		return fmt.Errorf("notification address: %w", err)
	}

	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return fmt.Errorf("notification listen error: %w", err)
	}

	g.Go(func() error {
		defer func() {
			_ = ln.Close()
			_ = os.Remove(path)
		}()

		go func() {
			<-ctx.Done()
			_ = ln.Close()
		}()

		for {
			conn, err := ln.Accept()
			if err != nil {
				if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
					return nil
				}
				return fmt.Errorf("notification accept error: %w", err)
			}
			g.Go(func() error {
				handleNotification(conn, vmc)
				return nil
			})
		}
	})
	return nil
}

func handleNotification(conn net.Conn, vmc *define.Machine) {
	defer conn.Close()

	var msg types.NotificationMessage
	if err := json.NewDecoder(conn).Decode(&msg); err != nil {
		logrus.Warnf("decode gvproxy notification: %v", err)
		return
	}

	switch msg.NotificationType {
	case types.Ready:
		if vmc.Readiness.SignalVNetHostReady() {
			logrus.Infof("gvproxy network stack ready")
		}
	case types.ConnectionEstablished:
		logrus.Infof("gvproxy network connection established: %s", msg.MacAddress)
	case types.ConnectionClosed:
		logrus.Infof("gvproxy network connection closed: %s", msg.MacAddress)
	case types.HypervisorError:
		logrus.Errorf("gvproxy hypervisor error")
	default:
		logrus.Debugf("unknown gvproxy notification: %s", msg.NotificationType)
	}
}

func validateUnixAddr(raw, scheme string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != scheme {
		return fmt.Errorf("expected %s:// address, got %q", scheme, parsed.Scheme)
	}
	if parsed.Path == "" {
		return errors.New("empty socket path")
	}
	return nil
}

func unixSocketPath(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.Path == "" {
		return "", errors.New("empty socket path")
	}
	return parsed.Path, nil
}

func httpServe(ctx context.Context, g *errgroup.Group, ln net.Listener, mux http.Handler) {
	g.Go(func() error {
		<-ctx.Done()
		return ln.Close()
	})

	g.Go(func() error {
		s := &http.Server{
			Handler:      mux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}
		if err := s.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}

		return nil
	})
}
