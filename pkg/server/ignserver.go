package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	linux3rdDir := filepath.Join(host3rdDir, "linux")

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, err := io.Copy(w, bytes.NewReader(data))
		if err != nil {
			logrus.Errorf("failed to serve ignition file: %v", err)
		}
	})
	mux.Handle("/3rd/linux/", http.StripPrefix("/3rd/linux/", http.FileServer(http.Dir(linux3rdDir))))

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
