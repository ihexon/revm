package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	yamux "github.com/libp2p/go-yamux/v5"
)

const ProtocolVersion = 3

type Request struct {
	Version    int      `json:"version"`
	InstanceID string   `json:"instance_id"`
	Argv       []string `json:"argv"`
	Env        []string `json:"env,omitempty"`
	WorkDir    string   `json:"cwd,omitempty"`
	TTY        bool     `json:"tty"`
	Term       string   `json:"term,omitempty"`
	Rows       uint16   `json:"rows,omitempty"`
	Cols       uint16   `json:"cols,omitempty"`
}

type Message struct {
	Type   string  `json:"type"`
	Rows   uint16  `json:"rows,omitempty"`
	Cols   uint16  `json:"cols,omitempty"`
	Result *Result `json:"result,omitempty"`
	Error  string  `json:"error,omitempty"`
}

type Streams struct {
	Session                        *yamux.Session
	Control, Stdin, Stdout, Stderr *yamux.Stream
	writeMu                        sync.Mutex
	Decoder                        *json.Decoder
}

func streams(ctx context.Context, conn net.Conn, server bool) (*Streams, error) {
	cfg := yamux.DefaultConfig()
	cfg.MaxStreamWindowSize = 256 * 1024
	var s *yamux.Session
	var err error
	if server {
		s, err = yamux.Server(conn, cfg, nil)
	} else {
		s, err = yamux.Client(conn, cfg, nil)
	}
	if err != nil {
		conn.Close()
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { s.Close() })
	defer stop()
	out := &Streams{Session: s}
	for _, slot := range []**yamux.Stream{&out.Control, &out.Stdin, &out.Stdout, &out.Stderr} {
		if server {
			*slot, err = s.AcceptStream()
		} else {
			*slot, err = s.OpenStream(ctx)
		}
		if err != nil {
			s.Close()
			return nil, err
		}
	}
	out.Decoder = json.NewDecoder(out.Control)
	return out, nil
}

func (s *Streams) Send(m Message) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_ = s.Control.SetWriteDeadline(time.Now().Add(time.Second))
	defer s.Control.SetWriteDeadline(time.Time{})
	return json.NewEncoder(s.Control).Encode(m)
}

func Open(ctx context.Context, conn net.Conn, req Request) (*Streams, error) {
	s, err := streams(ctx, conn, false)
	if err != nil {
		return nil, err
	}
	_ = s.Control.SetDeadline(time.Now().Add(10 * time.Second))
	defer s.Control.SetDeadline(time.Time{})
	if err := json.NewEncoder(s.Control).Encode(req); err != nil {
		s.Session.Close()
		return nil, err
	}
	var started Message
	if err := s.Decoder.Decode(&started); err != nil {
		s.Session.Close()
		return nil, err
	}
	if started.Type != "started" {
		s.Session.Close()
		return nil, fmt.Errorf("start execution: %s", started.Error)
	}
	return s, nil
}
