package alert

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Eyemetric/alpr_service/internal/db"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Job struct {
	ID        int64
	PlateID   int64
	HotlistID int64
}

type Sender interface {
	Send(ctx context.Context, j Job) error
}

type SimSender struct{ FailureOnOddPlate bool }

func (s SimSender) Send(ctx context.Context, j Job) error {

	if s.FailureOnOddPlate && (j.PlateID%2 == 1) {
		return errors.New("simulated vendor failure")
	}
	fmt.Printf("send alert id=%d (plateid=%d, hotlist=%d\n", j.ID, j.PlateID, j.HotlistID)
	return nil
}

func StartAlertListener(ctx context.Context, pool *pgxpool.Pool, s Sender) error {

	ln, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	if _, err := ln.Exec(ctx, `listen alerts_new`); err != nil {
		ln.Release()
		return err
	}

	q := db.New(pool)
	req_timeout := 60 * time.Second

	//a light thread that listens for events, sends hotlist hits, updates queue.
	go func() {
		defer ln.Release()
		//convert go type to postgres
		workerID := pgtype.Text{String: "worker-1", Valid: true}

		for ctx.Err() == nil {
			//drain the jobs
			for {
				rows, err := q.ClaimDue(ctx, db.ClaimDueParams{Batch: 1, WorkerID: workerID})
				if err != nil {
					log.Printf("claim error: %v", err)
					break
				}
				if len(rows) == 0 {
					break
				}
				row := rows[0]
				hitJob := Job{ID: row.ID, PlateID: row.PlateID, HotlistID: row.HotlistID}

				sendCtx, cancel := context.WithTimeout(ctx, req_timeout)
				//TODO: We actually need to build a json doc to send
				err = s.Send(sendCtx, hitJob)
				cancel()

				//NOTE: if Schedule fails for some reason we could have some queue issues build up. Think about this.
				if err == nil {
					if err := q.ScheduleSuccess(ctx, hitJob.ID); err != nil {
						log.Printf("success hook error: %v", err)
					}
				} else {
					if err := q.ScheduleFailure(ctx, db.ScheduleFailureParams{ID: hitJob.ID, Err: err.Error()}); err != nil {
						log.Printf("failure hook error: %v", err)

					}
				}
			}

			//Sleep until the next due time (or wake on a Notify from the db.)
			next, err := q.NextWake(ctx)
			if err != nil {
				next = time.Now().Add(500 * time.Millisecond)
			}

			wait := max(time.Until(next.(time.Time)), 50*time.Millisecond)
			waitCtx, cancel := context.WithTimeout(ctx, wait)
			_, _ = ln.Conn().WaitForNotification(waitCtx)
			cancel()

		}
	}()

	return nil

}
