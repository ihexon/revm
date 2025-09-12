//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/vmconfig"
	"net/http"
	"net/url"
	"sync"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/tmaxmax/go-sse"
)

type Server struct {
	Vmc        *vmconfig.VMConfig
	Server     *http.Server
	Mux        *http.ServeMux
	ListenAddr url.URL
}

func NewAPIServer(vmc *vmconfig.VMConfig) *Server {
	mux := http.NewServeMux()
	server := &Server{
		Mux:        mux,
		Vmc:        vmc,
		ListenAddr: url.URL{Scheme: "http", Host: define.DefaultRestAddr},
	}
	return server
}

func (s *Server) handleVMConfig(w http.ResponseWriter, r *http.Request) {
	logrus.Debugf("handle /vmconfig request")
	if r.Method != http.MethodGet {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}

	WriteJSON(w, http.StatusOK, s.Vmc)
}

func newSSEServer() *sse.Server {
	sseServer = &sse.Server{
		OnSession: func(w http.ResponseWriter, r *http.Request) (topics []string, allowed bool) {
			topic, ok := r.Context().Value(topicKey).(string)

			if topic == "" || !ok {
				logrus.Warn("empty topic in OnSession")
				return nil, false
			}

			return []string{topic}, true
		},
	}
	return sseServer
}

func (s *Server) doExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if sseServer == nil {
		sseServer = newSSEServer()
	}

	var action ExecAction
	if err := json.NewDecoder(r.Body).Decode(&action); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid json"))
		return
	}

	ctx, cancel := context.WithCancel(context.WithValue(r.Context(), topicKey, "sess-"+uuid.NewString())) //nolint:staticcheck
	defer cancel()

	go func() {
		var (
			execInfo *ExecInfo
			err      error
		)
		defer cancel()

		topic, ok := ctx.Value(topicKey).(string)
		if !ok {
			logrus.Warn("empty topic in go func")
			return
		}

		if r.URL.Path == hostexecURL {
			execInfo, err = LocalExec(ctx, action.Bin, action.Args...)
			if err != nil {
				createSSEMsgAndPublish(TypeErr, "local exec failed: "+err.Error(), sseServer, topic)
				return
			}
		}

		if r.URL.Path == guestexecURL {
			execInfo, err = GuestExec(ctx, s.Vmc, action.Bin, action.Args...)
			if err != nil {
				createSSEMsgAndPublish(TypeErr, "local exec failed: "+err.Error(), sseServer, topic)
				return
			}
		}

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			sc := bufio.NewScanner(execInfo.outPip)
			sc.Buffer(make([]byte, 64*1024), 1<<20) // 1MB
			for sc.Scan() {
				createSSEMsgAndPublish(TypeOut, sc.Text(), sseServer, topic)
			}
		}()

		go func() {
			defer wg.Done()
			sc := bufio.NewScanner(execInfo.errPip)
			sc.Buffer(make([]byte, 64*1024), 1<<20) // 1MB
			for sc.Scan() {
				createSSEMsgAndPublish(TypeErr, sc.Text(), sseServer, topic)
			}
		}()

		wg.Wait()

		if err = <-execInfo.exitChan; err != nil {
			logrus.Debugf("process exit with %v", err)
			createSSEMsgAndPublish(TypeErr, "wait: "+err.Error(), sseServer, topic)
			return
		}

		createSSEMsgAndPublish("done", "done", sseServer, topic)
	}()

	sseServer.ServeHTTP(w, r.WithContext(ctx))
}

const (
	hostexecURL  = "/hostexec"
	guestexecURL = "/guestexec"
	vmconfigURL  = "/vmconfig"
)

func (s *Server) registerRouter() {
	s.Mux.HandleFunc(vmconfigURL, s.handleVMConfig)
	s.Mux.HandleFunc(guestexecURL, s.doExec)
	s.Mux.HandleFunc(hostexecURL, s.doExec)
}

func (s *Server) Start(ctx context.Context) error {
	s.registerRouter()

	s.Server = &http.Server{
		Addr:    s.ListenAddr.Host,
		Handler: s.Mux,
	}
	errChan := make(chan error, 1)

	go func() {
		logrus.Infof("start revm API server on %q", s.ListenAddr.String())
		if err := s.Server.ListenAndServe(); err != nil {
			errChan <- err
		}
	}()

	select {
	case err := <-errChan:
		return fmt.Errorf("start rest server error: %w", err)
	case <-ctx.Done():
		logrus.Infof("close rest server on %q", s.ListenAddr.String())
		return context.Cause(ctx)
	}
}

// WriteJSON writes an interface value encoded as JSON to w
func WriteJSON(w http.ResponseWriter, code int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	coder := json.NewEncoder(w)
	coder.SetEscapeHTML(true)
	if err := coder.Encode(value); err != nil {
		logrus.Errorf("ynable to encode json: %q", err)
	}
}
