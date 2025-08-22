package repository

import (
	"context"
)

type ALPRRepository interface {
	IngestPlateRead(ctx context.Context, doc []byte) (string, error)
	AddHotlist(ctx context.Context, hotlist []byte) (int32, error)
}
