//go:build linux

package main

import (
	"bytes"
	"encoding/binary"
	"net/http"
	"testing"
)

func TestProbeRecognizesFramedPodmanResponse(t *testing.T) {
	// An exposed endpoint must be detected even when HTTP headers cross frames.
	response := []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK")
	var wire bytes.Buffer
	for _, b := range response {
		binary.Write(&wire, binary.BigEndian, uint32(1))
		wire.WriteByte(b)
	}
	binary.Write(&wire, binary.BigEndian, uint32(0))
	got, err := readPodmanResponse(&wire)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Body.Close()
	if got.StatusCode != http.StatusOK {
		t.Fatalf("exposed HTTP service was not recognized: %s", got.Status)
	}
}
