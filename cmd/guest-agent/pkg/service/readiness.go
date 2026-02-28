package service

import (
	"context"
	"guestAgent/pkg/network"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/ssh_v2"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

type Readiness struct {
	vmc *define.Machine
}

func NewServiceReadiness(vmc *define.Machine) *Readiness {
	return &Readiness{
		vmc: vmc,
	}
}

func (r *Readiness) IsSSHReady(ctx context.Context) bool {
	tmpKey := "/.sshkey"
	defer os.Remove(tmpKey)

	if err := os.WriteFile(tmpKey, []byte(r.vmc.SSHInfo.HostSSHPrivateKey), 0600); err != nil {
		logrus.Errorf("[readiness] failed to write ssh key: %v", err)
		return false
	}

	ticker := time.NewTicker(define.DefaultTimeTicker)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logrus.Debugf("[readiness] IsSSHReady check canceled")
			return false
		case <-ticker.C:
			logrus.Debugf("[readiness] check ssh ready")
			_, portStr, _ := net.SplitHostPort(r.vmc.SSHInfo.GuestSSHServerListenAddr)
			client, err := ssh_v2.Dial(ctx, net.JoinHostPort(define.LocalHost, portStr),
				ssh_v2.WithPrivateKey(tmpKey),
				ssh_v2.WithUser(define.DefaultGuestUser),
			)

			if err != nil {
				logrus.Debugf("[readiness] check ssh failed: %v", err)
				continue
			}

			if err = client.RunWith(ctx, define.BuiltinBusybox, nil, io.Discard, io.Discard); err != nil {
				_ = client.Close()
				logrus.Debugf("[readiness] check ssh run busybox failed: %v", err)
				continue
			}
			_ = client.Close()

			return true
		}
	}
}

func (r *Readiness) IsPodmanReady(ctx context.Context) bool {
	if r.vmc.RunMode == define.RootFsMode.String() {
		logrus.Info("[readiness] skip IsPodmanReady check")
		return true
	}

	ticker := time.NewTicker(define.DefaultTimeTicker)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logrus.Debugf("[readiness] IsPodmanReady check canceled")
			return false
		case <-ticker.C:
			logrus.Debugf("[readiness] check podman ready")
			_, portStr, _ := net.SplitHostPort(r.vmc.PodmanInfo.GuestPodmanAPIListenAddr)
			client := network.NewTCPClient(net.JoinHostPort(define.LocalHost, portStr))
			resp, err := client.NewRequest(http.MethodGet, "_ping").Do(ctx)
			if err != nil {
				_ = client.Close()
				logrus.Debugf("[readiness] ping podman api failed: %v", err)
				continue
			}
			if resp.StatusCode != http.StatusOK {
				_ = client.Close()
				network.CloseResponse(resp)
				logrus.Debugf("[readiness] ping podman api returned status code %d", resp.StatusCode)
				continue
			}
			_ = client.Close()
			network.CloseResponse(resp)
			return true
		}
	}

}

func (r *Readiness) IsInterfaceReady(ctx context.Context) bool {
	ticker := time.NewTicker(define.DefaultTimeTicker)
	defer ticker.Stop()

	ifName := "lo"
	switch r.vmc.VirtualNetworkMode {
	case define.GVISOR:
		ifName = "eth0"
	case define.TSI:
		ifName = "lo"
	default:
		return false
	}

	for {
		select {
		case <-ctx.Done():
			logrus.Debugf("[readiness] IsInterfaceReady check canceled")
			return false
		case <-ticker.C:
			logrus.Debugf("[readiness] check interface ready")
			iface, err := net.InterfaceByName(ifName)
			if err != nil {
				logrus.Debugf("[readiness] interface %s failed: %v", ifName, err)
				continue
			}

			addrs, err := iface.Addrs()
			if err != nil {
				logrus.Debugf("[readiness] get interface %s addresses failed: %v", ifName, err)
				continue
			}
			if len(addrs) <= 0 {
				logrus.Debugf("[readiness] interface %s has no addresses", ifName)
				continue
			}
			return true
		}
	}
}
