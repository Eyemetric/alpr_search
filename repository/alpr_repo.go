package repository

import (
	"context"
)

type ALPRRepository interface {
	IngestPlateRead(ctx context.Context, doc []byte) (string, error)
}
