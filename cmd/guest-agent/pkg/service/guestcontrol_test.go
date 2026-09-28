package service

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"linuxvm/pkg/protocol"
)

func TestGuestControlExecStreamsOutputAndExitCode(t *testing.T) {
	body, err := json.Marshal(protocol.GuestControlRequest{
		SchemaVersion: protocol.GuestControlVersion,
		Bin:           "/bin/sh",
		Args:          []string{"-c", "printf out; printf err >&2; exit 7"},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/exec", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handleGuestControl(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	dec := json.NewDecoder(rec.Body)
	var stdout, stderr []byte
	var exit int
	for {
		var frame protocol.GuestControlFrame
		err := dec.Decode(&frame)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch frame.Type {
		case protocol.GuestControlStdout:
			stdout = append(stdout, frame.Data...)
		case protocol.GuestControlStderr:
			stderr = append(stderr, frame.Data...)
		case protocol.GuestControlExit:
			exit = *frame.ExitCode
		}
	}
	if string(stdout) != "out" || string(stderr) != "err" || exit != 7 {
		t.Fatalf("stdout=%q stderr=%q exit=%d", stdout, stderr, exit)
	}
}
