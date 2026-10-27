//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package network

import (
	"fmt"
	"net/url"
)

func ParseUnixAddr(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if u.Scheme != "unix" && u.Scheme != "unixgram" {
		return nil, fmt.Errorf("invalid scheme %q, expected unix:// or unixgram://", u.Scheme)
	}
	if u.Host != "" {
		return nil, fmt.Errorf("unix socket address must not contain a host")
	}
	if u.Path == "" {
		return nil, fmt.Errorf("missing path")
	}

	return u, nil
}
