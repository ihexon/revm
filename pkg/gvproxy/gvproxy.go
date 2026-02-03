package gvproxy

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"linuxvm/pkg/vmconfig"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/containers/gvisor-tap-vsock/pkg/transport"
	"github.com/containers/gvisor-tap-vsock/pkg/types"
	"github.com/containers/gvisor-tap-vsock/pkg/virtualnetwork"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	yaml "gopkg.in/yaml.v3"
)

type GvproxyConfig struct {
	Listen     []string            `yaml:"listen,omitempty"`
	Stack      types.Configuration `yaml:"stack,omitempty"`
	Interfaces struct {
		VPNKit string `yaml:"vpnkit,omitempty"`
		Qemu   string `yaml:"qemu,omitempty"`
		Bess   string `yaml:"bess,omitempty"`
		Stdio  string `yaml:"stdio,omitempty"`
		Vfkit  string `yaml:"vfkit,omitempty"`
	} `yaml:"interfaces,omitempty"`
	Forwards []GvproxyConfigForward `yaml:"forwards,omitempty"`
	Services string                 `yaml:"probes,omitempty"`
}

type GvproxyConfigForward struct {
	Socket   string `yaml:"socket,omitempty"`
	Dest     string `yaml:"dest,omitempty"`
	User     string `yaml:"user,omitempty"`
	Identity string `yaml:"identity,omitempty"`
}

//go:embed config.yaml
var configYaml []byte

func InitCfg(vmc *vmconfig.VMConfig) (*GvproxyConfig, error) {
	var config GvproxyConfig

	if err := yaml.Unmarshal(configYaml, &config); err != nil {
		return nil, fmt.Errorf("failed to parse configuration: %w", err)
	}

	config.Listen = append(config.Listen, vmc.GvisorTapVsockEndpoint)
	config.Interfaces.Vfkit = vmc.GvisorTapVsockNetwork

	uri, err := url.Parse(config.Interfaces.Vfkit)
	if err != nil || uri == nil {
		return nil, fmt.Errorf("invalid value for vfkit listen address: %w", err)
	}
	if uri.Scheme != "unixgram" {
		return nil, fmt.Errorf("vfkit listen address must be unixgram:// address")
	}

	_ = os.Remove(uri.Path)

	config.Stack.Protocol = types.VfkitProtocol

	// BUG: Port TOCTOU, but we don't care about it for now
	port, err := network.GetAvailablePort(define.SSHLocalForwardListenPort)
	if err != nil {
		return nil, err
	}
	logrus.Infof("ssh listen port: %d", port)

	sshLocalForwardAddr := fmt.Sprintf("%s:%d", define.LocalHost, port)
	sshServerGuestAddr := fmt.Sprintf("%s:%d", define.GuestIP, define.GuestSSHServerPort)

	// ssh local forward: sshLocalForwardAddr -> sshServerGuestAddr
	// 						HOST					GUEST
	config.Stack.Forwards = map[string]string{
		sshLocalForwardAddr: sshServerGuestAddr,
	}

	vmc.SSHInfo.SSHLocalForwardAddr = sshLocalForwardAddr

	return &config, nil
}

func Run(ctx context.Context, vmc *vmconfig.VMConfig) error {
	config, err := InitCfg(vmc)
	if err != nil {
		return err
	}

	g, ctx := errgroup.WithContext(ctx)

	vn, err := virtualnetwork.New(&config.Stack)
	if err != nil {
		return err
	}

	for _, endpoint := range config.Listen {
		ln, err := transport.Listen(endpoint)
		if err != nil {
			return fmt.Errorf("cannot listen: %w", err)
		}
		httpServe(ctx, g, ln, withProfiler(vn))
	}

	if config.Services != "" {
		ln, err := transport.Listen(config.Services)
		if err != nil {
			return fmt.Errorf("cannot listen: %w", err)
		}
		httpServe(ctx, g, ln, vn.ServicesMux())
	}

	ln, err := vn.Listen("tcp", fmt.Sprintf("%s:80", config.Stack.GatewayIP))
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/services/forwarder/all", vn.Mux())
	mux.Handle("/services/forwarder/expose", vn.Mux())
	mux.Handle("/services/forwarder/unexpose", vn.Mux())
	httpServe(ctx, g, ln, mux)

	if config.Interfaces.Vfkit != "" {
		conn, err := transport.ListenUnixgram(config.Interfaces.Vfkit)
		if err != nil {
			return fmt.Errorf("vfkit listen error: %w", err)
		}

		g.Go(func() error {
			<-ctx.Done()
			if err := conn.Close(); err != nil {
				logrus.Errorf("error closing %s: %q", config.Interfaces.Vfkit, err)
			}
			vfkitSocketURI, _ := url.Parse(config.Interfaces.Vfkit)
			return os.Remove(vfkitSocketURI.Path)
		})

		g.Go(func() error {
			vfkitConn, err := transport.AcceptVfkit(conn)
			if err != nil {
				return fmt.Errorf("vfkit accept error: %w", err)
			}
			return vn.AcceptVfkit(ctx, vfkitConn)
		})
	}

	return g.Wait()
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

func withProfiler(vn *virtualnetwork.VirtualNetwork) http.Handler {
	mux := vn.Mux()
	return mux
}
