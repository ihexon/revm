package ssh

import (
	"context"
	"linuxvm/pkg/ssh"
	"testing"
)

func TestSSHClient(t *testing.T) {
	client, err := ssh.NewClient("127.0.0.1", "root", 61343, "/var/folders/dt/wqkv0wf13nl6n8jbf98jggjh0000gn/T/675f5ced/ssh_keypair", "ls", "-alv")
	if err != nil {
		t.Fatalf("failed to create ssh client: %v", err)
	}
	err = client.Run(context.Background())
	if err != nil {
		t.Fatalf("failed to run command: %v", err)
	}
}
