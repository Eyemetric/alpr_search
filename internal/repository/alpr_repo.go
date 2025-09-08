package repository

import (
	"context"

	"github.com/Eyemetric/alpr_service/internal/db"
)

type ALPRRepository interface {
	IngestPlateRead(ctx context.Context, doc []byte) (string, error)
	AddHotlist(ctx context.Context, hotlist []byte) (int32, error)
	ScheduleSuccess(ctx context.Context, id int64) error
	ScheduleFailure(ctx context.Context, failureParams db.ScheduleFailureParams) error
	ClaimDue(ctx context.Context, claimDueParams db.ClaimDueParams) ([]db.ClaimDueRow, error)
	GetPlateHit(ctx context.Context, plateHitParams db.GetPlateHitParams) ([]db.GetPlateHitRow, error)
}
