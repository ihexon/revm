//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package httpserver

import (
	"net/http"

	"github.com/sirupsen/logrus"
	"github.com/tmaxmax/go-sse"
)

// SSE message types
const (
	sseTypeOut = "out"
	sseTypeErr = "error"

	sseTopicKey = "sseTopicKey"
)

// sseServer wraps the SSE server with helper methods.
type sseServer struct {
	server *sse.Server
}

func newSSEServer() *sseServer {
	return &sseServer{
		server: &sse.Server{
			OnSession: func(w http.ResponseWriter, r *http.Request) ([]string, bool) {
				topic, ok := r.Context().Value(sseTopicKey).(string)
				if !ok || topic == "" {
					logrus.Warn("sse: empty topic in session")
					return nil, false
				}
				return []string{topic}, true
			},
		},
	}
}

func (s *sseServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.server.ServeHTTP(w, r)
}

func (s *sseServer) publish(topic, msgType, data string) {
	msg := &sse.Message{}
	msg.AppendData(data)
	msg.Type = sse.Type(msgType)

	if err := s.server.Publish(msg, topic); err != nil {
		logrus.Warnf("sse: failed to publish message: %v", err)
	}
}
