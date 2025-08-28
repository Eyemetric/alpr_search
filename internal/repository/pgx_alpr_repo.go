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
