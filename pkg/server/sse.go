package server

import (
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
