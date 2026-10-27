module guestAgent

go 1.26.3

require (
	github.com/google/uuid v1.6.0
	github.com/mdlayher/vsock v1.3.0
	github.com/moby/sys/mountinfo v0.7.3-0.20250407123443-8d553c09dc5e
	github.com/sirupsen/logrus v1.10.2
	github.com/vishvananda/netlink v1.3.1
	golang.org/x/sync v0.23.0
	linuxvm v0.0.0
)

require (
	github.com/creack/pty v1.1.24 // indirect
	github.com/inetaf/tcpproxy v0.0.0-20260515195445-c159a6051109 // indirect
	github.com/libp2p/go-buffer-pool v0.0.2 // indirect
	github.com/libp2p/go-yamux/v5 v5.1.0 // indirect
	github.com/mdlayher/socket v0.6.0 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace linuxvm => ../../
