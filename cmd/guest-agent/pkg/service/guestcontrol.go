package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/protocol"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/mdlayher/vsock"
	"github.com/sirupsen/logrus"
)

// StartGuestControlServer exposes the guest command runner over a dedicated
// vsock port. SSH remains available for interactive compatibility, but all
// host control operations use this endpoint.
func StartGuestControlServer(ctx context.Context) error {
	listener, err := vsock.Listen(define.GuestControlPort, nil)
	if err != nil {
		return fmt.Errorf("listen guest control vsock: %w", err)
	}

	srv := &http.Server{Handler: http.HandlerFunc(handleGuestControl)}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		_ = listener.Close()
	}()
	logrus.Infof("guest control listening on vsock port %d", define.GuestControlPort)
	if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("guest control server: %w", err)
	}
	return nil
}

func handleGuestControl(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v1/shutdown" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		go func() {
			// Let the response reach the host before the init signal path starts
			// its disk-sync and reboot sequence.
			_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
		}()
		return
	}
	if r.Method != http.MethodPost || r.URL.Path != "/v1/exec" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	var req protocol.GuestControlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.SchemaVersion != protocol.GuestControlVersion || req.Bin == "" {
		http.Error(w, "unsupported or empty command", http.StatusBadRequest)
		return
	}

	cmd := exec.CommandContext(r.Context(), req.Bin, req.Args...)
	cmd.Dir = req.WorkDir
	if cmd.Dir == "" {
		cmd.Dir = "/"
	}
	cmd.Env = append(os.Environ(), req.Env...)
	if len(req.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(req.Stdin)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := cmd.Start(); err != nil {
		writeControlFrame(w, nil, protocol.GuestControlFrame{Type: protocol.GuestControlError, Error: err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	var writeMu sync.Mutex
	write := func(frame protocol.GuestControlFrame) {
		writeMu.Lock()
		defer writeMu.Unlock()
		if err := writeControlFrame(w, flusher, frame); err != nil {
			logrus.Debugf("guest control response write failed: %v", err)
		}
	}

	var wg sync.WaitGroup
	copyOutput := func(src io.Reader, typ string) {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, readErr := src.Read(buf)
			if n > 0 {
				data := append([]byte(nil), buf[:n]...)
				write(protocol.GuestControlFrame{Type: typ, Data: data})
			}
			if readErr != nil {
				return
			}
		}
	}
	wg.Add(2)
	go copyOutput(stdout, protocol.GuestControlStdout)
	go copyOutput(stderr, protocol.GuestControlStderr)
	waitErr := cmd.Wait()
	wg.Wait()
	code := 0
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			write(protocol.GuestControlFrame{Type: protocol.GuestControlError, Error: waitErr.Error()})
			code = 1
		}
	}
	write(protocol.GuestControlFrame{Type: protocol.GuestControlExit, ExitCode: &code})
}

func writeControlFrame(w http.ResponseWriter, flusher http.Flusher, frame protocol.GuestControlFrame) error {
	if err := json.NewEncoder(w).Encode(frame); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}
