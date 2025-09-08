package repository

import (
	"context"
	"fmt"

	"github.com/Eyemetric/alpr_service/internal/db"
	_ "github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PgxAlprRepo struct {
	dbpool  *pgxpool.Pool
	queries *db.Queries
}

func NewPgxAlprRepo(pool *pgxpool.Pool) *PgxAlprRepo {
	queries := db.New(pool)
	return &PgxAlprRepo{
		dbpool:  pool,
		queries: queries,
	}
}

func (a *PgxAlprRepo) IngestPlateRead(ctx context.Context, doc []byte) (string, error) {
	res, err := a.queries.IngestALPR(ctx, doc)
	if err != nil {
		return "", fmt.Errorf("failed to ingest plate read: %w", err)
	}
	fmt.Println("failures go in a deadletter table")
	fmt.Printf("ingest result: %+v\n", res)
	return "", nil
}

func (a *PgxAlprRepo) AddHotlist(ctx context.Context, hotlist []byte) (int32, error) {
	add_cnt, err := a.queries.InsertHotlist(ctx, hotlist)
	if err != nil {
		return 0, fmt.Errorf("failed to add hotlist: %w", err)
	}
	return add_cnt, nil
}

// TODO: not in use yet. it's kind of useless wrapper just to implement the repo interface
func (a *PgxAlprRepo) ScheduleSuccess(ctx context.Context, id int64) error {

	err := a.queries.ScheduleSuccess(ctx, id)
	if err != nil {
		return err
	}

	return nil

}

func (a *PgxAlprRepo) ScheduleFailure(ctx context.Context, failureParams db.ScheduleFailureParams) error {

	err := a.queries.ScheduleFailure(ctx, failureParams)
	if err != nil {
		return err
	}

	return nil

}

func (a *PgxAlprRepo) ClaimDue(ctx context.Context, claimDueParams db.ClaimDueParams) ([]db.ClaimDueRow, error) {
	claimsDue, err := a.queries.ClaimDue(ctx, claimDueParams)
	if err != nil {
		return nil, err
	}
	return claimsDue, nil
}

func (a *PgxAlprRepo) GetPlateHit(ctx context.Context, plateHitParams db.GetPlateHitParams) ([]db.GetPlateHitRow, error) {
	hits, err := a.queries.GetPlateHit(ctx, plateHitParams)
	if err != nil {
		return nil, err
	}

	return hits, nil
}
