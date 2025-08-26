package alert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Eyemetric/alpr_service/internal/db"
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
	fmt.Printf("send alert id=%d (plateid=%d, hotlist=%d)\n", j.ID, j.PlateID, j.HotlistID)
	return nil
}

// reconnectWithBackoff attempts to re-establish a LISTEN connection with exponential backoff
func reconnectWithBackoff(ctx context.Context, relisten func() error) bool {
	backoff := 200 * time.Millisecond
	maxBackoff := 10 * time.Second
	attempts := 5

	for i := 0; i < attempts && ctx.Err() == nil; i++ {
		if i > 0 {
			time.Sleep(backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}

		err := relisten()
		if err == nil {
			log.Printf("reconnected successfully")
			return true
		}
		log.Printf("relisten attempt %d/%d failed: %v\n", i+1, attempts, err)

	}
	return false
}

// NOTE: Listen/Notify turned out to be pretty complex! using a much
// simpler db poll until this strategy becomes necessary
func StartAlertListenerX(ctx context.Context, pool *pgxpool.Pool, s Sender) error {
	const (
		channel    = "alerts_new"
		hbEvery    = 10 * time.Second // DB should NOTIFY at least this often (heartbeat or real work)
		missAfter  = 3 * hbEvery      // tolerate a few missed beats
		minSleep   = 50 * time.Millisecond
		reqTimeout = 60 * time.Second
	)

	q := db.New(pool)
	var ln *pgxpool.Conn
	lastSeen := time.Now()

	relisten := func() error {
		if ln != nil {
			ln.Release()
			ln = nil
		}
		var err error
		ln, err = pool.Acquire(ctx)
		if err != nil {
			return err
		}
		if _, err = ln.Exec(ctx, "LISTEN "+channel); err != nil {
			ln.Release()
			ln = nil
			return err
		}
		lastSeen = time.Now()
		return nil
	}

	if err := relisten(); err != nil {
		return err
	}

	// Worker goroutine
	go func() {
		defer func() {
			if ln != nil {
				ln.Release()
			}
		}()

		// drain items from queue with specified batch size
		drainQueue := func(batchSize int32) bool {
			rows, err := q.ClaimDue(ctx, db.ClaimDueParams{Batch: batchSize, WorkerID: "worker-1"})
			if err != nil {
				log.Printf("claim error: %v", err)
				return false
			}
			if len(rows) == 0 {
				return false
			}

			// Process all claimed rows
			for _, row := range rows {
				hitJob := Job{ID: row.ID, PlateID: row.PlateID, HotlistID: row.HotlistID}
				sendCtx, cancel := context.WithTimeout(ctx, reqTimeout)
				err = s.Send(sendCtx, hitJob)
				cancel()

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

			return true
		}

		for ctx.Err() == nil {
			// Drain all currently due items, one at a time
			for drainQueue(1) && ctx.Err() == nil {
			}

			// Compute next wake (fallback now+500ms)
			next, err := q.NextWake(ctx)
			if err != nil {
				next = time.Now().Add(500 * time.Millisecond)
			}
			nextT, ok := next.(time.Time)
			if !ok {
				nextT = time.Now().Add(500 * time.Millisecond)
			}

			wait := time.Until(nextT)
			if wait < minSleep {
				wait = minSleep
			}
			if wait > hbEvery {
				// Bound wait so we can check heartbeat regularly
				wait = hbEvery
			}

			// Wait for NOTIFY (bounded)
			waitCtx, cancel := context.WithTimeout(ctx, wait)
			n, werr := ln.Conn().WaitForNotification(waitCtx)
			cancel()

			switch {
			case werr == nil:
				// Got a NOTIFY: log + parse payload (optional)
				lastSeen = time.Now()
				log.Printf("notify: channel=%s pid=%d payload=%s", n.Channel, n.PID, n.Payload)
				// Example payloads: {"bulk":"drain"} or {"type":"hb"}
				var msg struct {
					Type string `json:"type"`
					Bulk string `json:"bulk"`
				}
				if err := json.Unmarshal([]byte(n.Payload), &msg); err == nil {
					if msg.Bulk == "drain" {
						// immediately try to drain more work
						continue
					}
					// treat any message as activity (heartbeat or real work)
				}

			case errors.Is(werr, context.DeadlineExceeded):
				// No NOTIFY in this window; check heartbeat SLA
				if time.Since(lastSeen) > missAfter {
					log.Printf("missed heartbeat for %v; reconnecting", time.Since(lastSeen))
					if err := relisten(); err != nil {
						log.Printf("relisten failed: %v", err)
						return
					}
				}

			case errors.Is(werr, context.Canceled):
				// parent ctx canceled; exit
				return

			default:
				// Hard error (conn dropped, etc.) -> reconnect with backoff
				log.Printf("listener error: %v; reconnecting", werr)
				if !reconnectWithBackoff(ctx, relisten) {
					log.Printf("failed to reconnect after 5 attempts; exiting")
					return
				}
			}
		}
	}()

	return nil
}

// A simple database poll to check for plate hits every 5 seconds.
func StartListener(ctx context.Context, pool *pgxpool.Pool, s Sender) error {
	const (
		pollInterval = 5 * time.Second
		sendTimeout  = 60 * time.Second
		batchSize    = 1
	)

	q := db.New(pool)

	go func() {
		drainQueue := func() bool {
			rows, err := q.ClaimDue(ctx, db.ClaimDueParams{Batch: batchSize, WorkerID: "worker-1"})
			if err != nil {
				log.Printf("claim error: %v", err)
				return false
			}
			//nothing to claim
			if len(rows) == 0 {
				return false
			}

			for _, row := range rows {
				hitJob := Job{ID: row.ID, PlateID: row.PlateID, HotlistID: row.HotlistID}
				sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
				err = s.Send(sendCtx, hitJob)
				cancel()

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

			return true
		}

		//the polling
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		//initial drain
		for drainQueue() && ctx.Err() == nil {
		}

		for ctx.Err() == nil {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for drainQueue() && ctx.Err() == nil {
				}
			}
		}
	}()

	return nil
}
