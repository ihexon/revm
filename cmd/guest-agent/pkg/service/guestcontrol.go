package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/execution"
	"linuxvm/pkg/protocol"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	guestvsock "guestAgent/pkg/vsock"
)

// StartGuestControlServer exposes the guest command runner over a dedicated
// vsock port. SSH remains available for interactive compatibility, but all
// host control operations use this endpoint.
var execRegistry execution.Registry
var guestInstance = uuid.NewString()

func StartGuestControlServer(ctx context.Context) error {
	listener, err := guestvsock.ListenHost(define.GuestControlPort)
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
	if r.URL.Path == "/v2/ready" {
		_ = json.NewEncoder(w).Encode(protocol.ExecReady{Version: 2, InstanceID: guestInstance})
		return
	}
	if r.URL.Path == "/v2/exec" {
		if r.Header.Get("Upgrade") != "revm-exec-v2" {
			http.Error(w, "exec v2 upgrade required", 400)
			return
		}
		h, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "upgrade unavailable", 500)
			return
		}
		conn, rw, err := h.Hijack()
		if err != nil {
			return
		}
		_, err = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: revm-exec-v2\r\n\r\n")
		if err != nil || rw.Flush() != nil {
			conn.Close()
			return
		}
		execution.Serve(context.Background(), conn, guestInstance, &execRegistry)
		return
	}
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
		writeControlFrame(w, nil, protocol.GuestControlFrame{Type: protocol.GuestControlExit, Result: resultPointer(execution.FromProcess(err))})
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
	result := execution.FromProcess(waitErr)
	write(protocol.GuestControlFrame{Type: protocol.GuestControlExit, ExitCode: result.Code, Result: &result})
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

func resultPointer(r execution.Result) *execution.Result { return &r }
