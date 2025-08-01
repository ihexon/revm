package gvproxy

import (
	"context"
	"fmt"
	"linuxvm/pkg/network"
	"linuxvm/pkg/vmconfig"

	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/containers/gvisor-tap-vsock/pkg/transport"
	gvptypes "github.com/containers/gvisor-tap-vsock/pkg/types"
	"github.com/containers/gvisor-tap-vsock/pkg/virtualnetwork"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const (
	gatewayIP      = "192.168.127.1"
	hostIP         = "192.168.127.254"
	host           = "host"
	gateway        = "gateway"
	subNet         = "192.168.127.0/24"
	gatewayMacAddr = "5a:94:ef:e4:0c:dd"
	guestMacAddr   = "5a:94:ef:e4:0c:ee"
	guestIPAddr    = "192.168.127.2"
)

func newGvpConfigure() *gvptypes.Configuration {
	config := gvptypes.Configuration{
		Debug:             false,
		MTU:               1500,
		Subnet:            subNet,
		GatewayIP:         gatewayIP,
		GatewayMacAddress: gatewayMacAddr,
		DHCPStaticLeases: map[string]string{
			guestIPAddr: guestMacAddr,
		},
		DNS: []gvptypes.Zone{
			{
				Name: "containers.internal.",
				Records: []gvptypes.Record{
					{
						Name: gateway,
						IP:   net.ParseIP(gatewayIP),
					},
					{
						Name: host,
						IP:   net.ParseIP(hostIP),
					},
				},
			},
			{
				Name: "docker.internal.",
				Records: []gvptypes.Record{
					{
						Name: gateway,
						IP:   net.ParseIP(gatewayIP),
					},
					{
						Name: host,
						IP:   net.ParseIP(hostIP),
					},
				},
			},
		},
		// by default, we forward host:2222 to 192.168.127.2:22
		NAT: map[string]string{
			hostIP: "127.0.0.1",
		},
		GatewayVirtualIPs: []string{hostIP},
		VpnKitUUIDMacAddresses: map[string]string{
			"c3d68012-0208-11ea-9fd7-f2189899ab08": guestMacAddr,
		},
		Protocol: gvptypes.VfkitProtocol,
	}

	return &config

}

func getForwardsMap() (map[string]string, error) {
	guestSideSSHAddr := fmt.Sprintf("%s:%s", guestIPAddr, "22")

	portInHostSide, err := network.GetAvailablePort()
	if err != nil {
		return map[string]string{}, fmt.Errorf("failed to get avaliable port: %w", err)
	}
	hostSideSSHAddr := fmt.Sprintf("%s:%d", "127.0.0.1", portInHostSide)

	return map[string]string{
		hostSideSSHAddr: guestSideSSHAddr,
	}, nil
}

func httpServe(ctx context.Context, g *errgroup.Group, ln net.Listener, mux http.Handler) {
	// if ctx is canceled, close the listener
	g.Go(func() error {
		<-ctx.Done()
		logrus.Infof("close gvproxy control endpoint on %q", ln.Addr())
		return ln.Close()
	})

	// if the ctx is canceled, the server will return an error immediately. so there is no
	// need to pass the ctx to the server.
	g.Go(func() error {
		server := &http.Server{
			Handler:      mux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}

		err := server.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	})
}

type EndPoints struct {
	// export the http api which control gvproxy
	ControlEndpoints string
	// the unix socket file that provides network to vm
	VFKitSocketEndpoint string
}

func run(ctx context.Context, g *errgroup.Group, configuration *gvptypes.Configuration, endpoints EndPoints) error {
	vn, err := virtualnetwork.New(configuration)
	if err != nil {
		return err
	}

	{
		logrus.Infof("listen gvproxy control endpoint: %q", endpoints.ControlEndpoints)
		ln, err := transport.Listen(endpoints.ControlEndpoints)
		if err != nil {
			return fmt.Errorf("failed to listen on %q: %w", endpoints.ControlEndpoints, err)
		}

		httpServe(ctx, g, ln, vn.Mux())
	}

	ln, err := vn.Listen("tcp", fmt.Sprintf("%s:80", gatewayIP))
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/services/forwarder/all", vn.Mux())
	mux.Handle("/services/forwarder/expose", vn.Mux())
	mux.Handle("/services/forwarder/unexpose", vn.Mux())
	httpServe(ctx, g, ln, mux)

	if endpoints.VFKitSocketEndpoint != "" {
		logrus.Infof("listen gvproxy network backend: %q", endpoints.VFKitSocketEndpoint)
		conn, err := transport.ListenUnixgram(endpoints.VFKitSocketEndpoint)
		if err != nil {
			return fmt.Errorf("failed to listen on %q: %w", endpoints.VFKitSocketEndpoint, err)
		}

		g.Go(func() error {
			<-ctx.Done()
			logrus.Infof("close gvproxy network backend on %q", endpoints.VFKitSocketEndpoint)
			if err := conn.Close(); err != nil {
				logrus.Errorf("error closing %s: %q", endpoints.VFKitSocketEndpoint, err)
			}

			vfkitSocketURI, err := url.Parse(endpoints.VFKitSocketEndpoint)
			if err != nil {
				logrus.Errorf("failed to parse %q: %v", endpoints.VFKitSocketEndpoint, err)
			}

			return os.Remove(vfkitSocketURI.Path)
		})

		g.Go(func() error {
			vfkitConn, err := transport.AcceptVfkit(conn)
			if err != nil {
				return fmt.Errorf("failed to accept connection on %q: %w", endpoints.VFKitSocketEndpoint, err)
			}
			logrus.Infof("accept connection on %q", endpoints.VFKitSocketEndpoint)
			return vn.AcceptVfkit(ctx, vfkitConn)
		})
	}

	return g.Wait()
}

func StartNetworking(ctx context.Context, vmc *vmconfig.VMConfig) error {
	g, ctx := errgroup.WithContext(ctx)

	endpoints := EndPoints{
		ControlEndpoints:    vmc.GVproxyEndpoint,
		VFKitSocketEndpoint: vmc.NetworkStackBackend,
	}

	if err := makeDirForUnixSocks(endpoints.ControlEndpoints); err != nil {
		return fmt.Errorf("failed to create dir for gvproxy control unix socket file %q: %w", endpoints.ControlEndpoints, err)
	}
	if err := makeDirForUnixSocks(endpoints.VFKitSocketEndpoint); err != nil {
		return fmt.Errorf("failed to create dir for gvproxy network unix socket file %q: %w", endpoints.VFKitSocketEndpoint, err)
	}

	gvpCfg := newGvpConfigure()

	forwardMaps, err := getForwardsMap()
	if err != nil {
		return fmt.Errorf("failed to get avaliable port: %w", err)
	}
	logrus.Infof("forward maps: %v", forwardMaps)
	gvpCfg.Forwards = forwardMaps
	vmc.PortForwardMap = forwardMaps

	return run(ctx, g, gvpCfg, endpoints)
}

func makeDirForUnixSocks(str string) error {
	parse, err := url.Parse(str)
	if err != nil {
		return err
	}

	dir := filepath.Dir(parse.Path)
	if err = os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return nil
}
