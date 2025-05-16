package network

import (
	"bufio"
	"context"
	"fmt"
	"github.com/containers/gvisor-tap-vsock/pkg/transport"
	gvptypes "github.com/containers/gvisor-tap-vsock/pkg/types"
	"github.com/containers/gvisor-tap-vsock/pkg/virtualnetwork"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"
	"linuxvm/pkg/vmconfig"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"
)

const (
	gatewayIP   = "192.168.127.1"
	sshHostPort = "192.168.127.2:22"
	hostIP      = "192.168.127.254"
	host        = "host"
	gateway     = "gateway"
)

func newGvpConfigure() *gvptypes.Configuration {
	protocol := gvptypes.VfkitProtocol
	config := gvptypes.Configuration{
		Debug:             false,
		CaptureFile:       "",
		MTU:               1500,
		Subnet:            "192.168.127.0/24",
		GatewayIP:         gatewayIP,
		GatewayMacAddress: "5a:94:ef:e4:0c:dd",
		DHCPStaticLeases: map[string]string{
			"192.168.127.2": "5a:94:ef:e4:0c:ee",
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
		DNSSearchDomains: searchDomains(),
		// by default, we forward host:2222 to 192.168.127.2:22
		Forwards: getForwardsMap(2222, sshHostPort),
		NAT: map[string]string{
			hostIP: "127.0.0.1",
		},
		GatewayVirtualIPs: []string{hostIP},
		VpnKitUUIDMacAddresses: map[string]string{
			"c3d68012-0208-11ea-9fd7-f2189899ab08": "5a:94:ef:e4:0c:ee",
		},
		Protocol: protocol,
	}

	return &config

}

func searchDomains() []string {
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		f, err := os.Open("/etc/resolv.conf")
		if err != nil {
			logrus.Errorf("open file error: %v", err)
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		searchPrefix := "search "
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), searchPrefix) {
				return parseSearchString(sc.Text(), searchPrefix)
			}
		}
		if err := sc.Err(); err != nil {
			logrus.Errorf("scan file error: %v", err)
			return nil
		}
	}
	return nil
}

// Parse and sanitize search list
// macOS has limitation on number of domains (6) and general string length (256 characters)
// since glibc 2.26 Linux has no limitation on 'search' field
func parseSearchString(text, searchPrefix string) []string {
	// macOS allow only 265 characters in search list
	if runtime.GOOS == "darwin" && len(text) > 256 {
		logrus.Errorf("Search domains list is too long, it should not exceed 256 chars on macOS: %d", len(text))
		text = text[:256]
		lastSpace := strings.LastIndex(text, " ")
		if lastSpace != -1 {
			text = text[:lastSpace]
		}
	}

	searchDomains := strings.Split(strings.TrimPrefix(text, searchPrefix), " ")
	logrus.Infof("Using search domains: %v", searchDomains)

	// macOS allow only 6 domains in search list
	if runtime.GOOS == "darwin" && len(searchDomains) > 6 {
		logrus.Errorf("Search domains list is too long, it should not exceed 6 domains on macOS: %d", len(searchDomains))
		searchDomains = searchDomains[:6]
	}

	return searchDomains
}

func getForwardsMap(sshPort int, sshHostPort string) map[string]string {
	if sshPort == -1 {
		return map[string]string{}
	}
	return map[string]string{
		fmt.Sprintf("127.0.0.1:%d", sshPort): sshHostPort,
	}
}

func httpServe(ctx context.Context, g *errgroup.Group, ln net.Listener, mux http.Handler) {
	// if ctx is canceled, close the listener
	g.Go(func() error {
		<-ctx.Done()
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

func withProfiler(vn *virtualnetwork.VirtualNetwork) http.Handler {
	mux := vn.Mux()
	return mux
}

type EndPoints struct {
	// export the http api
	ControlEndpoints    []string
	VFKitSocketEndpoint string
}

func run(ctx context.Context, g *errgroup.Group, configuration *gvptypes.Configuration, endpoints EndPoints) error {
	vn, err := virtualnetwork.New(configuration)
	if err != nil {
		return err
	}

	for _, endpoint := range endpoints.ControlEndpoints {
		logrus.Infof("gvproxy control endpoint: %q", endpoints.ControlEndpoints)

		ln, err := transport.Listen(endpoint)
		if err != nil {
			return errors.Wrap(err, "cannot listen")
		}
		httpServe(ctx, g, ln, withProfiler(vn))
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
		logrus.Infof("network backend: %q", endpoints.VFKitSocketEndpoint)
		conn, err := transport.ListenUnixgram(endpoints.VFKitSocketEndpoint)
		if err != nil {
			return errors.Wrap(err, "vfkit listen error")
		}

		g.Go(func() error {
			<-ctx.Done()
			if err := conn.Close(); err != nil {
				logrus.Errorf("error closing %s: %q", endpoints.VFKitSocketEndpoint, err)
			}
			vfkitSocketURI, _ := url.Parse(endpoints.VFKitSocketEndpoint)
			return os.Remove(vfkitSocketURI.Path)
		})

		g.Go(func() error {
			vfkitConn, err := transport.AcceptVfkit(conn)
			if err != nil {
				return errors.Wrap(err, "vfkit accept error")
			}
			return vn.AcceptVfkit(ctx, vfkitConn)
		})
	}

	return g.Wait()
}

func WaitForSocket(ctx context.Context, socketPath string) error {
	var backoff = 30 * time.Millisecond
	for range 100 {
		select {
		case <-ctx.Done():
			return fmt.Errorf("cancel waitForSocket,ctx cancelled: %w", context.Cause(ctx))
		default:
			if err := Exists(socketPath); err != nil {
				logrus.Warnf("Gvproxy network backend socket not ready, try test %q again....", socketPath)
				time.Sleep(backoff)
				continue
			}
			return nil
		}
	}
	return fmt.Errorf("gvproxy network backend socket file not created in %q", socketPath)
}

func Exists(path string) error {
	// It uses unix.Faccessat which is a faster operation compared to os.Stat for
	// simply checking the existence of a file.
	err := unix.Faccessat(unix.AT_FDCWD, path, unix.F_OK, 0)
	if err != nil {
		return &os.PathError{Op: "faccessat", Path: path, Err: err}
	}
	return nil
}

func StartNetworking(ctx context.Context, vmc *vmconfig.VMConfig) error {
	g, ctx := errgroup.WithContext(ctx)

	endpoints := EndPoints{
		ControlEndpoints:    []string{vmc.GVproxyEndpoint},
		VFKitSocketEndpoint: vmc.NetworkStackBackend,
	}

	return run(ctx, g, newGvpConfigure(), endpoints)
}
