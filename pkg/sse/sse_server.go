//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package sse

import (
	"net/http"

	"github.com/sirupsen/logrus"
	"github.com/tmaxmax/go-sse"
)

// SSE message types
const (
	TypeOut = "out"
	TypeErr = "error"

	TopicKey = "sseTopicKey"
)

// Server wraps the SSE server with helper methods.
type Server struct {
	server *sse.Server
}

func NewSSEServer() *Server {
	return &Server{
		server: &sse.Server{
			OnSession: func(w http.ResponseWriter, r *http.Request) ([]string, bool) {
				topic, ok := r.Context().Value(TopicKey).(string)
				if !ok || topic == "" {
					logrus.Warn("sse: empty topic in session")
					return nil, false
				}
				return []string{topic}, true
			},
		},
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.server.ServeHTTP(w, r)
}

func (s *Server) Publish(topic, msgType, data string) {
	msg := &sse.Message{}
	msg.AppendData(data)
	msg.Type = sse.Type(msgType)

	if err := s.server.Publish(msg, topic); err != nil {
		logrus.Warnf("sse: failed to publish message: %v", err)
	}
}
