//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	httpv2 "linuxvm/pkg/http"
	ssev2 "linuxvm/pkg/sse"
	"linuxvm/pkg/vmbuilder"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"github.com/google/uuid"
)

type ManagementAPIServer struct {
	vmc *vmbuilder.VM
	srv *httpv2.Server
	sse *ssev2.Server
}

// NewManagementAPIServer creates a httpserver for host-side VM management.
func NewManagementAPIServer(vmc *vmbuilder.VM) *ManagementAPIServer {
	return &ManagementAPIServer{
		vmc: vmc,
		srv: httpv2.NewUnixSockHTTPServer("management-api", vmc.VMCtlAddress),
		sse: ssev2.NewSSEServer(),
	}
}

// Start begins serving requests. Blocks until context is cancelled.
func (s *ManagementAPIServer) Start(ctx context.Context) error {
	s.srv.Mux.HandleFunc("/healthz", s.handleHealth)
	s.srv.Mux.HandleFunc("/vmconfig", s.handleVMConfig)
	s.srv.Mux.HandleFunc("/exec", s.handleExec)
	s.srv.Mux.HandleFunc("/stop", s.handleRequestVMStop)

	// Legacy API for compat
	if s.vmc.RunMode == define.OVMode.String() {
		s.srv.Mux.HandleFunc("/info", s.handleInfo)
	}

	return s.srv.Serve(ctx)
}

type Info struct {
	PodmanAPIProxyOnHost string `json:"podmanSocketPath"`
	SSHProxyPortOnHost   int    `json:"sshPort"`
	SSHUserOnGuest       string `json:"sshUser"`
	HostDNSInGVPNetwork  string `json:"hostEndpoint"`
}

// handleInfo Legacy API for OVM mode
func (s *ManagementAPIServer) handleInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}

	podmanProxyaddr, err := url.Parse(s.vmc.PodmanInfo.PodmanProxyAddr)
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, ErrResponse{Error: err.Error()})
		return
	}

	sshProxyAddr, err := url.Parse(fmt.Sprintf("tcp://%s", s.vmc.SSHInfo.SSHLocalForwardAddr))
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, ErrResponse{Error: err.Error()})
		return
	}

	sshPort, err := strconv.Atoi(sshProxyAddr.Port())
	if err != nil {
		WriteJSON(w, http.StatusInternalServerError, ErrResponse{Error: err.Error()})
		return
	}

	info := Info{
		PodmanAPIProxyOnHost: podmanProxyaddr.Path,
		SSHProxyPortOnHost:   sshPort,
		SSHUserOnGuest:       define.DefaultGuestUser,
		HostDNSInGVPNetwork:  define.HostDomainInGVPNet,
	}

	WriteJSON(w, http.StatusOK, info)
}

func (s *ManagementAPIServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	WriteJSON(w, http.StatusOK, nil)
}

func (s *ManagementAPIServer) handleVMConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}
	WriteJSON(w, http.StatusOK, s.vmc)
}

func (s *ManagementAPIServer) handleRequestVMStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteJSON(w, http.StatusMethodNotAllowed, nil)
		return
	}

	WriteJSON(w, http.StatusOK, nil)
	s.vmc.StopOnce.Do(func() {
		close(s.vmc.StopCh)
	})
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
	ctx, cancel := context.WithCancel(context.WithValue(r.Context(), ssev2.TopicKey, topic)) //nolint:staticcheck
	defer cancel()

	go s.executeCommand(ctx, cancel, topic, req)

	s.sse.ServeHTTP(w, r.WithContext(ctx))
}

func (s *ManagementAPIServer) executeCommand(ctx context.Context, cancel context.CancelFunc, topic string, req execRequest) {
	defer cancel()

	proc, err := GuestExec(ctx, s.vmc, req.Bin, req.Args...)
	if err != nil {
		s.sse.Publish(topic, ssev2.TypeErr, "guest exec failed: "+err.Error())
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
			s.sse.Publish(topic, ssev2.TypeOut, sc.Text())
		}
	}()

	// Stream stderr
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(proc.StderrPipeReader)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			s.sse.Publish(topic, ssev2.TypeErr, sc.Text())
		}
	}()

	wg.Wait()

	if err := <-proc.errChan; err != nil {
		s.sse.Publish(topic, ssev2.TypeErr, "wait: "+err.Error())
		return
	}

	s.sse.Publish(topic, "done", "done")
}
