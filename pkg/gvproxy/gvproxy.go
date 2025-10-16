package gvproxy

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"linuxvm/pkg/vmconfig"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/containers/gvisor-tap-vsock/pkg/sshclient"
	"github.com/containers/gvisor-tap-vsock/pkg/transport"
	"github.com/containers/gvisor-tap-vsock/pkg/types"
	"github.com/containers/gvisor-tap-vsock/pkg/virtualnetwork"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	yaml "gopkg.in/yaml.v3"
)

const (
	sshHostPort = "192.168.127.2:22"
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
	Services string                 `yaml:"services,omitempty"`
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

	config.Listen = append(config.Listen, vmc.GVproxyEndpoint)
	config.Interfaces.Vfkit = vmc.NetworkStackBackend

	uri, err := url.Parse(config.Interfaces.Vfkit)
	if err != nil || uri == nil {
		return nil, fmt.Errorf("invalid value for vfkit listen address: %w", err)
	}
	if uri.Scheme != "unixgram" {
		return nil, errors.New("vfkit listen address must be unixgram:// address")
	}

	_ = os.Remove(uri.Path)

	config.Stack.Protocol = types.VfkitProtocol

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
	logrus.Info("gvproxy virtual network waiting for clients...")

	for _, endpoint := range config.Listen {
		logrus.Infof("listening %s", endpoint)
		ln, err := transport.Listen(endpoint)
		if err != nil {
			return fmt.Errorf("cannot listen: %w", err)
		}
		httpServe(ctx, g, ln, withProfiler(vn))
	}

	if config.Services != "" {
		logrus.Infof("enabling services API. Listening %s", config.Services)
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

	for i := range config.Forwards {
		var (
			src *url.URL
			err error
		)
		if strings.Contains(config.Forwards[i].Socket, "://") {
			src, err = url.Parse(config.Forwards[i].Socket)
			if err != nil {
				return err
			}
		} else {
			src = &url.URL{
				Scheme: "unix",
				Path:   config.Forwards[i].Socket,
			}
		}

		dest := &url.URL{
			Scheme: "ssh",
			User:   url.User(config.Forwards[i].User),
			Host:   sshHostPort,
			Path:   config.Forwards[i].Dest,
		}
		j := i
		g.Go(func() error {
			defer os.Remove(config.Forwards[j].Socket)
			forward, err := sshclient.CreateSSHForward(ctx, src, dest, config.Forwards[j].Identity, vn)
			if err != nil {
				return err
			}
			go func() {
				<-ctx.Done()
				// Abort pending accepts
				forward.Close()
			}()
		loop:
			for {
				select {
				case <-ctx.Done():
					break loop
				default:
					// proceed
				}
				err := forward.AcceptAndTunnel(ctx)
				if err != nil {
					logrus.Debugf("Error occurred handling ssh forwarded connection: %q", err)
				}
			}
			return nil
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
