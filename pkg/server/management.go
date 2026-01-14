//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"linuxvm/pkg/vmconfig"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

// ManagementAPIServer provides a REST API for managing the VM from the host.
// It listens on a Unix socket on the host.
//
// Endpoints:
//   - GET  /healthz  - Health check
//   - GET  /vmconfig - Returns the complete VM configuration as JSON
//   - POST /exec     - Execute a command in the guest (SSE streaming output)
type ManagementAPIServer struct {
	vmc *vmconfig.VMConfig
	srv *httpServer
	sse *sseServer
}

// NewManagementAPIServer creates a server for host-side VM management.
func NewManagementAPIServer(vmc *vmconfig.VMConfig) *ManagementAPIServer {
	return &ManagementAPIServer{
		vmc: vmc,
		srv: newHTTPServer("management-api", vmc.RestAPIAddress),
		sse: newSSEServer(),
	}
}

// Start begins serving requests. Blocks until context is cancelled.
func (s *ManagementAPIServer) Start(ctx context.Context) error {
	s.srv.mux.HandleFunc("/healthz", s.handleHealth)
	s.srv.mux.HandleFunc("/vmconfig", s.handleVMConfig)
	s.srv.mux.HandleFunc("/exec", s.handleExec)

	return s.srv.serve(ctx)
}

func (s *ManagementAPIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	logrus.Debug("management-api: /healthz")
	WriteJSON(w, http.StatusOK, nil)
}

func (s *ManagementAPIServer) handleVMConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	logrus.Debug("management-api: /vmconfig")
	WriteJSON(w, http.StatusOK, s.vmc)
}

// execRequest represents a command execution request.
type execRequest struct {
	Bin  string   `json:"bin,omitempty"`
	Args []string `json:"args,omitempty"`
	Env  []string `json:"env,omitempty"`
}

func (s *ManagementAPIServer) handleExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req execRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	topic := "sess-" + uuid.NewString()
	ctx, cancel := context.WithCancel(context.WithValue(r.Context(), sseTopicKey, topic)) //nolint:staticcheck
	defer cancel()

	go s.executeCommand(ctx, cancel, topic, req)

	s.sse.ServeHTTP(w, r.WithContext(ctx))
}

func (s *ManagementAPIServer) executeCommand(ctx context.Context, cancel context.CancelFunc, topic string, req execRequest) {
	defer cancel()

	proc, err := GuestExec(ctx, s.vmc, req.Bin, req.Args...)
	if err != nil {
		s.sse.publish(topic, sseTypeErr, "guest exec failed: "+err.Error())
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Stream stdout
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(proc.StdoutPipeReader)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			s.sse.publish(topic, sseTypeOut, sc.Text())
		}
	}()

	// Stream stderr
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(proc.StderrPipeReader)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			s.sse.publish(topic, sseTypeErr, sc.Text())
		}
	}()

	wg.Wait()

	if err := <-proc.errChan; err != nil {
		s.sse.publish(topic, sseTypeErr, "wait: "+err.Error())
		return
	}

	s.sse.publish(topic, "done", "done")
}
