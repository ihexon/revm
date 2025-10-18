package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"linuxvm/pkg/define"
	"linuxvm/pkg/network"
	"linuxvm/pkg/ssh"
	"linuxvm/pkg/vmconfig"
	"net/http"
	"net/url"
	"sync"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	gossh "golang.org/x/crypto/ssh"
)

type execAction struct {
	Bin  string   `json:"bin,omitempty"`
	Args []string `json:"args,omitempty"`
	Env  []string `json:"env,omitempty"`
}

type ProcessStatusWrapper struct {
	StdoutPipeReader *io.PipeReader
	StderrPipeReader *io.PipeReader
	errChan          chan error
}

func GuestExec(ctx context.Context, vmc *vmconfig.VMConfig, bin string, args ...string) (*ProcessStatusWrapper, error) {
	addr, err := network.ParseUnixAddr(vmc.GVproxyEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to parse gvproxy endpoint: %w", err)
	}

	// Parse gvproxy endpoint
	endpoint, err := url.Parse(vmc.GVproxyEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to parse gvproxy endpoint: %w", err)
	}

	// Configure SSH client
	cfg := ssh.NewClientConfig(
		define.DefaultGuestAddr,
		uint16(define.DefaultGuestSSHDPort),
		define.DefaultGuestUser,
		vmc.SSHInfo.HostSSHKeyPairFile,
	).WithGVProxySocket(endpoint.Path)

	client, err := ssh.NewClient(ctx, cfg)

	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", addr.Path, err)
	}

	// remember to close the writer when the process exits
	stdOutReader, stdoutWriter := io.Pipe()
	stderrOutReader, stderrWriter := io.Pipe()
	errChan := make(chan error, 1)

	go func() {
		defer func(client *ssh.Client) {
			if err := client.Close(); err != nil {
				logrus.Errorf("failed to close client: %v", err)
			}
		}(client)

		defer func() {
			close(errChan)
			if err := stdoutWriter.Close(); err != nil {
				logrus.Errorf("failed to close stdoutWriter: %v", err)
			}
			if err := stderrWriter.Close(); err != nil {
				logrus.Errorf("failed to close stderrWriter: %v", err)
			}
		}()

		cmdSlice := append([]string{bin}, args...)
		errChan <- ssh.NewExecutor(client).Exec(ctx, &ssh.ExecOptions{
			Stdout:       stdoutWriter,
			Stderr:       stderrWriter,
			EnablePTY:    false,
			CancelSignal: gossh.SIGKILL,
		}, cmdSlice...)
	}()

	procStat := &ProcessStatusWrapper{
		StdoutPipeReader: stdOutReader,
		StderrPipeReader: stderrOutReader,
		errChan:          errChan,
	}

	return procStat, nil
}

func (s *Server) doExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if sseServer == nil {
		sseServer = newSSEServer()
	}

	var action execAction
	if err := json.NewDecoder(r.Body).Decode(&action); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid json"))
		return
	}

	ctx, cancel := context.WithCancel(context.WithValue(r.Context(), topicKey, "sess-"+uuid.NewString())) //nolint:staticcheck
	defer cancel()

	go func() {
		defer cancel()

		topic, ok := ctx.Value(topicKey).(string)
		if !ok {
			logrus.Warn("empty topic in go func")
			return
		}

		procStat, err := GuestExec(ctx, s.Vmc, action.Bin, action.Args...)
		if err != nil {
			createSSEMsgAndPublish(TypeErr, "guest exec failed: "+err.Error(), sseServer, topic)
			return
		}

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			sc := bufio.NewScanner(procStat.StdoutPipeReader)
			sc.Buffer(make([]byte, 64*1024), 1<<20) // 1MB
			for sc.Scan() {
				createSSEMsgAndPublish(TypeOut, sc.Text(), sseServer, topic)
			}
			logrus.Debugf("read process.outPipeReader line by line done")
		}()

		go func() {
			defer wg.Done()
			sc := bufio.NewScanner(procStat.StderrPipeReader)
			sc.Buffer(make([]byte, 64*1024), 1<<20) // 1MB
			for sc.Scan() {
				createSSEMsgAndPublish(TypeErr, sc.Text(), sseServer, topic)
			}
			logrus.Debugf("read process.errPipeReader line by line done")
		}()

		wg.Wait()
		logrus.Debugf("read process.outPipeReader/process.errPipeReader line by line group done")

		if err = <-procStat.errChan; err != nil {
			createSSEMsgAndPublish(TypeErr, "wait: "+err.Error(), sseServer, topic)
			return
		}

		createSSEMsgAndPublish("done", "done", sseServer, topic)
	}()

	sseServer.ServeHTTP(w, r.WithContext(ctx))
}
