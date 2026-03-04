package system

import (
	"slices"
	"sync"

	"github.com/sirupsen/logrus"
)

type CleanupCallback struct {
	noCopy noCopy
	Funcs  []func() error
	mu     sync.Mutex
}

// noCopy is used by go vet's copylocks checker to detect accidental value copies.
type noCopy struct{}

func (*noCopy) Lock() {}

func (c *CleanupCallback) CleanIfErr(err *error) {
	if err == nil {
		return
	}
	if *err == nil {
		return
	}
	c.clean()
}

func (c *CleanupCallback) DoClean() {
	c.clean()
}

func (c *CleanupCallback) clean() {
	c.mu.Lock()
	// Claim exclusive usage by copy and resetting to nil
	funcs := c.Funcs
	c.Funcs = nil
	c.mu.Unlock()

	// Already claimed or none set
	if funcs == nil {
		return
	}

	// Cleanup functions run in LIFO order so teardown mirrors setup.
	slices.Reverse(funcs)
	for _, cleanfunc := range funcs {
		if err := cleanfunc(); err != nil {
			logrus.Warnf("%v", err.Error())
		}
	}
}

func NewCleanUp() *CleanupCallback {
	return &CleanupCallback{
		Funcs: []func() error{},
	}
}

func (c *CleanupCallback) Add(anotherFunc func() error) {
	if anotherFunc == nil {
		return
	}
	c.mu.Lock()
	c.Funcs = append(c.Funcs, anotherFunc)
	c.mu.Unlock()
}

// AddFunc adds a cleanup callback that does not return an error.
func (c *CleanupCallback) AddFunc(fn func()) {
	if fn == nil {
		return
	}
	c.Add(func() error {
		fn()
		return nil
	})
}
