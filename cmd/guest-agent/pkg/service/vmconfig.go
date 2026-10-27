package service

import (
	"encoding/json"
	"fmt"
	"linuxvm/pkg/define"
	"linuxvm/pkg/protocol"
	"os"
)

func GetVMConfig() (*protocol.GuestSpec, error) {
	data, err := os.ReadFile(define.VMConfigFilePathInGuest)
	if err != nil {
		return nil, err
	}
	var spec protocol.GuestSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, err
	}
	if spec.SchemaVersion != protocol.GuestSpecVersion {
		return nil, fmt.Errorf("unsupported guest spec schema version: %d", spec.SchemaVersion)
	}
	return &spec, nil
}
