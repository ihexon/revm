package define

import "sync"

// Readiness tracks the lifecycle signals of a single VM instance.
// Each Machine holds its own *Readiness, so multiple VMs can coexist.
type Readiness struct {
	SSHReady       chan struct{}
	PodmanReady    chan struct{}
	VNetHostReady  chan struct{}
	VNetGuestReady chan struct{}

	sshOnce       sync.Once
	podmanOnce    sync.Once
	vNetHostOnce  sync.Once
	vNetGuestOnce sync.Once
}

func NewReadiness() *Readiness {
	return &Readiness{
		SSHReady:       make(chan struct{}, 1),
		PodmanReady:    make(chan struct{}, 1),
		VNetHostReady:  make(chan struct{}, 1),
		VNetGuestReady: make(chan struct{}, 1),
	}
}

// SignalSSHReady closes SSHReady exactly once. Returns true if this was the first call.
func (r *Readiness) SignalSSHReady() bool {
	first := false
	r.sshOnce.Do(func() { close(r.SSHReady); first = true })
	return first
}

// SignalPodmanAPIProxyReady closes PodmanReady exactly once. Returns true if this was the first call.
func (r *Readiness) SignalPodmanAPIProxyReady() bool {
	first := false
	r.podmanOnce.Do(func() { close(r.PodmanReady); first = true })
	return first
}

// SignalVNetHostReady closes VNetHostReady exactly once. Returns true if this was the first call.
func (r *Readiness) SignalVNetHostReady() bool {
	first := false
	r.vNetHostOnce.Do(func() { close(r.VNetHostReady); first = true })
	return first
}

// SignalVNetGuestReady closes VNetGuestReady exactly once. Returns true if this was the first call.
func (r *Readiness) SignalVNetGuestReady() bool {
	first := false
	r.vNetGuestOnce.Do(func() { close(r.VNetGuestReady); first = true })
	return first
}
