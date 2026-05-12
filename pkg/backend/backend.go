package backend

import (
	"context"
)

type Backend interface {
	Start(ctx context.Context) error
	Stop() error
}
