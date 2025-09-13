package server

import (
	"net/http"

	"github.com/sirupsen/logrus"
	"github.com/tmaxmax/go-sse"
)

const (
	TypeOut = "out"
	TypeErr = "error"

	topicKey = "topicKey"
)

var (
	sseServer *sse.Server
)

func createSSEMsgAndPublish(msgType, msg string, sseServer *sse.Server, topic string) {
	m := &sse.Message{}
	m.AppendData(msg)
	m.Type = sse.Type(msgType)
	if err := sseServer.Publish(m, topic); err != nil {
		logrus.Warnf("Error publishing message: %v", err)
	}
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
