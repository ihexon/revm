//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package management

import (
	"bufio"
	"context"
	"encoding/json"
	httpv2 "linuxvm/pkg/http"
	"linuxvm/pkg/interfaces"
	sshsvc "linuxvm/pkg/service/ssh"
	ssev2 "linuxvm/pkg/sse"
	"net/http"
	"sync"

	"github.com/google/uuid"
)

type Server struct {
	srv *httpv2.Server
	sse *ssev2.Server
	vmp interfaces.VMMProvider
}

type errResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, code int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value) //nolint:errchkjson
}

func NewServer(vmp interfaces.VMMProvider) (*Server, error) {
	return &Server{
		vmp: vmp,
		srv: httpv2.NewUnixSockHTTPServer("management-api", vmp.GetVMConfig().VMCtlAddr),
		sse: ssev2.NewSSEServer(),
	}, nil
}

func (s *Server) Start(ctx context.Context) error {
	// new management api
	s.srv.Mux.HandleFunc("/v2/healthz", s.handleHealth)
	s.srv.Mux.HandleFunc("/v2/vmconfig", s.handleVMConfig)
	s.srv.Mux.HandleFunc("/v2/exec", s.handleExec)
	s.srv.Mux.HandleFunc("/v2/stop", s.handleRequestVMStop)

	return s.srv.Serve(ctx)
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
	writeJSON(w, http.StatusOK, s.vmp.GetVMConfig())
}

func (s *Server) handleRequestVMStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	_ = s.vmp.Stop()
	writeJSON(w, http.StatusOK, nil)
}

type execRequest struct {
	Bin  string   `json:"bin,omitempty"`
	Args []string `json:"args,omitempty"`
	Env  []string `json:"env,omitempty"`
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
	topic := "sess-" + uuid.NewString()
	ctx, cancel := context.WithCancel(context.WithValue(r.Context(), ssev2.TopicKey, topic)) //nolint:staticcheck
	defer cancel()
	go s.executeCommand(ctx, cancel, topic, req)
	s.sse.ServeHTTP(w, r.WithContext(ctx))
}

func (s *Server) executeCommand(ctx context.Context, cancel context.CancelFunc, topic string, req execRequest) {
	defer cancel()
	vmc := s.vmp.GetVMConfig()
	proc, err := sshsvc.GuestExec(ctx, vmc, req.Bin, req.Args...)
	if err != nil {
		s.sse.Publish(topic, ssev2.TypeErr, "guest exec failed: "+err.Error())
		return
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(proc.StdoutPipeReader)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			s.sse.Publish(topic, ssev2.TypeOut, sc.Text())
		}
	}()
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(proc.StderrPipeReader)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			s.sse.Publish(topic, ssev2.TypeErr, sc.Text())
		}
	}()
	wg.Wait()
	if err := <-proc.ErrChan; err != nil {
		s.sse.Publish(topic, ssev2.TypeErr, "wait: "+err.Error())
		return
	}
	s.sse.Publish(topic, "done", "done")
}
