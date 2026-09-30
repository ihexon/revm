//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package management

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	httpv2 "linuxvm/pkg/http"
	"linuxvm/pkg/protocol"
	guestcontrol "linuxvm/pkg/service/guestcontrol"
	ssev2 "linuxvm/pkg/sse"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type Server struct {
	srv     *httpv2.Server
	sse     *ssev2.Server
	machine Machine
}

type errResponse struct {
	Error string `json:"error"`
}

type Machine interface {
	RequestShutdown(ctx context.Context) error
	ManagementView() VMConfigView
	AttachSpec() protocol.AttachSpec
	GuestControlTarget() guestcontrol.Target
}

func writeJSON(w http.ResponseWriter, code int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value) //nolint:errchkjson
}

func NewServer(machine Machine) (*Server, error) {
	if machine == nil {
		return nil, fmt.Errorf("machine is nil")
	}
	config := machine.ManagementView()
	if config.Endpoints.ManagementAPI == "" {
		return nil, fmt.Errorf("management API endpoint is empty")
	}
	return &Server{
		machine: machine,
		srv:     httpv2.NewUnixSockHTTPServer("management-api", config.Endpoints.ManagementAPI),
		sse:     ssev2.NewServer(),
	}, nil
}

func (s *Server) Start(ctx context.Context) error {
	s.srv.Mux.HandleFunc("/v2/healthz", s.handleHealth)
	s.srv.Mux.HandleFunc("/v2/vmconfig", s.handleVMConfig)
	s.srv.Mux.HandleFunc("/v2/attach", s.handleAttach)
	s.srv.Mux.HandleFunc("/v2/exec", s.handleExec)
	s.srv.Mux.HandleFunc("/v2/stop", s.handleRequestVMStop)

	err := s.srv.Serve(ctx)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = s.sse.Shutdown(shutdownCtx)

	return err
}

type Info struct {
	PodmanAPIProxyOnHost string `json:"podmanSocketPath"`
	SSHProxyPortOnHost   int    `json:"sshPort"`
	SSHUserOnGuest       string `json:"sshUser"`
	HostDNSInGVPNetwork  string `json:"hostEndpoint"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

func (s *Server) handleVMConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	writeJSON(w, http.StatusOK, s.machine.ManagementView())
}

func (s *Server) handleAttach(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	writeJSON(w, http.StatusOK, s.machine.AttachSpec())
}

func (s *Server) handleRequestVMStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	if err := s.machine.RequestShutdown(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

type execRequest struct {
	Bin     string   `json:"bin,omitempty"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
	WorkDir string   `json:"workDir,omitempty"`
}

func (s *Server) handleExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req execRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	sess := s.sse.BeginSession()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go s.executeCommand(ctx, cancel, sess, req)
	sess.ServeHTTP(w, r.WithContext(ctx))
}

func (s *Server) executeCommand(ctx context.Context, cancel context.CancelFunc, sess *ssev2.Session, req execRequest) {
	defer cancel()
	proc, err := guestcontrol.GuestExecRequest(ctx, s.machine.GuestControlTarget(), protocol.GuestControlRequest{
		SchemaVersion: protocol.GuestControlVersion,
		Bin:           req.Bin,
		Args:          req.Args,
		Env:           req.Env,
		WorkDir:       req.WorkDir,
	})
	if err != nil {
		publish(sess, ssev2.Stderr, "guest exec failed: "+err.Error())
		return
	}
	var wg sync.WaitGroup
	streamErrCh := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		streamErrCh <- streamOutput(proc.StdoutPipeReader, ssev2.Stdout, sess)
	}()
	go func() {
		defer wg.Done()
		streamErrCh <- streamOutput(proc.StderrPipeReader, ssev2.Stderr, sess)
	}()
	wg.Wait()
	var streamErr error
	for i := 0; i < 2; i++ {
		if err := <-streamErrCh; err != nil && streamErr == nil {
			streamErr = err
		}
	}
	if streamErr != nil {
		publish(sess, ssev2.Stderr, "stream output failed: "+streamErr.Error())
	}
	if err := <-proc.ErrChan; err != nil {
		publish(sess, ssev2.Stderr, "wait: "+err.Error())
		return
	}
	publish(sess, ssev2.Done, "done")
}

func streamOutput(r io.Reader, typ ssev2.EventType, sess *ssev2.Session) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			publish(sess, typ, string(buf[:n]))
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func publish(sess *ssev2.Session, typ ssev2.EventType, data string) {
	if err := sess.Publish(typ, data); err != nil {
		logrus.Warnf("sse: publish failed on session %s: %v", sess.Topic(), err)
	}
}
