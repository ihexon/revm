package main

import (
	"linuxvm/pkg/define"
	"strings"
)

func showVersionAndOSInfo() string {
	var version strings.Builder
	if define.Version != "" {
		version.WriteString(define.Version)
	} else {
		version.WriteString("unknown")
	}

	version.WriteString("-")

	if define.CommitID != "" {
		version.WriteString(define.CommitID)
	} else {
		version.WriteString(" (unknown)")
	}

	return version.String()
}
