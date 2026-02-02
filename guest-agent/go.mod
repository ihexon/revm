module guestAgent

go 1.25.5

require (
	github.com/insomniacslk/dhcp v0.0.0-20251020182700-175e84fbb167
	github.com/mdlayher/vsock v1.2.1
	github.com/sirupsen/logrus v1.9.5-0.20260121091959-524506f8912c
	github.com/urfave/cli/v3 v3.6.1
	github.com/vishvananda/netlink v1.3.1
	golang.org/x/sync v0.19.0
	linuxvm v0.0.0
)

require (
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/josharian/native v1.1.0 // indirect
	github.com/jsimonetti/rtnetlink v1.3.5 // indirect
	github.com/mdlayher/netlink v1.7.2 // indirect
	github.com/mdlayher/packet v1.1.2 // indirect
	github.com/mdlayher/socket v0.5.1 // indirect
	github.com/pierrec/lz4/v4 v4.1.23 // indirect
	github.com/u-root/uio v0.0.0-20240224005618-d2acac8f3701 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
)

replace linuxvm => ../
