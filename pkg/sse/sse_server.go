//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package sse

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	gosse "github.com/tmaxmax/go-sse"
)

// EventType identifies the kind of SSE event sent to clients.
type EventType string

const (
	Stdout EventType = "out"
	Stderr EventType = "error"
	Done   EventType = "done"
)

// Server creates and manages SSE sessions. It owns the underlying
// pub-sub provider and is responsible for shutting it down on exit.
type Server struct {
	inner *gosse.Server
}

// NewServer creates an SSE server. Sessions are created via BeginSession;
// the server itself is not an http.Handler.
func NewServer() *Server {
	return &Server{
		inner: &gosse.Server{
			OnSession: func(w http.ResponseWriter, r *http.Request) ([]string, bool) {
				topic, ok := r.Context().Value(internalTopicKey).(string)
				if !ok || topic == "" {
					w.WriteHeader(http.StatusForbidden)
					return nil, false
				}
				return []string{topic}, true
			},
		},
	}
}

// Shutdown closes the SSE provider, stopping all active subscriptions.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.inner.Shutdown(ctx)
}

// Session represents a single SSE client connection tied to a unique topic.
// Each session publishes events on its own topic, isolating streams between clients.
type Session struct {
	server *Server
	topic  string
}

// BeginSession creates a new SSE session with a unique topic.
func (s *Server) BeginSession() *Session {
	return &Session{
		server: s,
		topic:  "sess-" + uuid.NewString(),
	}
}

// Topic returns the session's unique identifier, useful for logging.
func (sess *Session) Topic() string {
	return sess.topic
}

// ServeHTTP upgrades the HTTP request into an SSE connection for this session.
func (sess *Session) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := context.WithValue(r.Context(), internalTopicKey, sess.topic) //nolint:staticcheck
	sess.server.inner.ServeHTTP(w, r.WithContext(ctx))
}

// Publish sends an event of the given type with the provided data to this session's clients.
func (sess *Session) Publish(typ EventType, data string) error {
	msg := &gosse.Message{}
	msg.AppendData(data)
	msg.Type = gosse.Type(string(typ))
	return sess.server.inner.Publish(msg, sess.topic)
}

type contextKeyType struct{}

var internalTopicKey = contextKeyType{}