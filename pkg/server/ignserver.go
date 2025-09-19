package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"linuxvm/pkg/system"
	"linuxvm/pkg/vmconfig"
	"net"
	"net/http"
	"path/filepath"

	"github.com/sirupsen/logrus"
)

func IgnProvisionerServer(ctx context.Context, vmc *vmconfig.VMConfig, ignServerAddr string) error {
	addr, err := network.ParseUnixAddr(ignServerAddr)
	if err != nil {
		return fmt.Errorf("failed to parse unix socket address: %w", err)
	}

	data, err := json.Marshal(vmc)
	if err != nil {
		return fmt.Errorf("failed to marshal vmconfig: %w", err)
	}

	host3rdDir, err := system.Get3rdDir()
	if err != nil {
		return fmt.Errorf("failed to get 3rd dir: %w", err)
	}
	linux3rdBinDir := filepath.Join(host3rdDir, "linux", "bin")

	mux := http.NewServeMux()

	// default return the vmconfig.json
	mux.HandleFunc(define.RestAPIVMConfigURL, func(w http.ResponseWriter, _ *http.Request) {
		_, err := io.Copy(w, bytes.NewReader(data))
		if err != nil {
			logrus.Errorf("failed to serve ignition file: %v", err)
		}
	})

	// 3rd utils for guest file server /
	logrus.Debugf("file server path: %q", linux3rdBinDir)

	mux.Handle(define.RestAPI3rdFileServerURL, http.StripPrefix(define.RestAPI3rdFileServerURL, http.FileServer(http.Dir(linux3rdBinDir))))

	// inform the guest podman ready
	// mux.HandleFunc(define.RestAPIPodmanReadyURL, func(w http.ResponseWriter, _ *http.Request) {
	//	logrus.Debugf("rest api recved the podman api ready request")
	//	close(vmc.Stage.PodmanReadyChan)
	//})
	// inform the guest sshd ready
	//mux.HandleFunc(define.RestAPISSHReadyURL, func(w http.ResponseWriter, _ *http.Request) {
	//	logrus.Debugf("rest api recved the ssh api ready request")
	//	close(vmc.Stage.SSHDReadyChan)
	//})

	logrus.Infof("start ignition server on %q", ignServerAddr)
	listener, err := net.Listen("unix", addr.Path)
	if err != nil {
		return fmt.Errorf("failed to listen on unix socket %q: %w", ignServerAddr, err)
	}

	defer func() {
		logrus.Debugf("stop listening on %q", listener.Addr().String())
		_ = listener.Close()
	}()

	errChan := make(chan error, 1)

	srv := &http.Server{
		Handler: mux,
	}

	go func() {
		errChan <- srv.Serve(listener)
	}()

	defer func() {
		logrus.Debugf("stop ignition server")
		_ = srv.Close()
	}()

	// signal to the main process that ignserver is ready
	close(vmc.Stage.IgnServerChan)

	select {
	case err = <-errChan:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
