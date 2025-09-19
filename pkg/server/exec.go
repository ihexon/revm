package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"linuxvm/pkg/network"
	"linuxvm/pkg/ssh"
	"linuxvm/pkg/vmconfig"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

type execAction struct {
	Bin  string   `json:"bin,omitempty"`
	Args []string `json:"args,omitempty"`
	Env  []string `json:"env,omitempty"`
}

type ExecProcess struct {
	outPipeReader io.Reader
	errPipeReader io.Reader
	exitChan      chan error
}

func GuestExec(ctx context.Context, vmc *vmconfig.VMConfig, bin string, args ...string) (*ExecProcess, error) {
	process := &ExecProcess{
		exitChan: make(chan error, 1),
	}

	addr, err := network.ParseUnixAddr(vmc.GVproxyEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to parse gvproxy endpoint: %w", err)
	}

	cfg := ssh.NewCfg(vmc.SSHInfo.GuestAddr, vmc.SSHInfo.User, vmc.SSHInfo.Port, vmc.SSHInfo.HostSSHKeyPairFile)
	defer cfg.CleanUp.CleanIfErr(&err)

	cfg.SetPty(false)
	cfg.SetCmdLine(bin, args)

	if err = cfg.Connect(ctx, addr.Path); err != nil {
		return nil, fmt.Errorf("failed to connect to gvproxy: %w", err)
	}

	keepAliveCtx, keepAliveCancel := context.WithCancel(ctx)
	defer keepAliveCancel()
	go func() {
		ssh.StartKeepAlive(keepAliveCtx, cfg.SSHClient)
	}()

	// remember to close the writer when the process exits
	stdOutReader, stdoutWriter := io.Pipe()
	stderrOutReader, stderrWriter := io.Pipe()

	cleanFunc := func() {
		_ = stdoutWriter.Close()
		_ = stderrWriter.Close()
	}

	if err = cfg.WriteOutputTo(stdoutWriter, stderrWriter); err != nil {
		return nil, fmt.Errorf("failed to make std pipe: %w", err)
	}

	process.outPipeReader = stdOutReader
	process.errPipeReader = stderrOutReader

	go func() {
		err := cfg.Run(ctx)
		logrus.Debugf("process exit with %v", err)
		defer cfg.CleanUp.CleanIfErr(&err)
		defer cleanFunc()
		process.exitChan <- err
	}()

	return process, nil
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
		var (
			process *ExecProcess
			err     error
		)

		defer cancel()

		topic, ok := ctx.Value(topicKey).(string)
		if !ok {
			logrus.Warn("empty topic in go func")
			return
		}

		process, err = GuestExec(ctx, s.Vmc, action.Bin, action.Args...)
		if err != nil {
			createSSEMsgAndPublish(TypeErr, "guest exec failed: "+err.Error(), sseServer, topic)
			return
		}

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			sc := bufio.NewScanner(process.outPipeReader)
			sc.Buffer(make([]byte, 64*1024), 1<<20) // 1MB
			for sc.Scan() {
				createSSEMsgAndPublish(TypeOut, sc.Text(), sseServer, topic)
			}
			logrus.Debugf("read process.outPipeReader line by line done")
		}()

		go func() {
			defer wg.Done()
			sc := bufio.NewScanner(process.errPipeReader)
			sc.Buffer(make([]byte, 64*1024), 1<<20) // 1MB
			for sc.Scan() {
				createSSEMsgAndPublish(TypeErr, sc.Text(), sseServer, topic)
			}
			logrus.Debugf("read process.errPipeReader line by line done")
		}()

		wg.Wait()
		logrus.Debugf("read process.outPipeReader/process.errPipeReader line by line group done")

		if err = <-process.exitChan; err != nil {
			logrus.Debugf("process exit with %v", err)
			createSSEMsgAndPublish(TypeErr, "wait: "+err.Error(), sseServer, topic)
			return
		}

		createSSEMsgAndPublish("done", "done", sseServer, topic)
	}()

	sseServer.ServeHTTP(w, r.WithContext(ctx))
}
