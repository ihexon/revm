package interfaces

import (
	"context"
)

type VMMProvider interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}
