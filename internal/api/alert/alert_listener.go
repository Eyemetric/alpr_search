package alert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Eyemetric/alpr_service/internal/api/wasabi"
	"github.com/Eyemetric/alpr_service/internal/db"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Job struct {
	ID        int64
	PlateID   int64
	HotlistID int64
}

type PlateHits struct {
	Plates []PlateHit `json:"plateHits,omitzero"`
}

type PlateHit struct {
	ID               string    `json:"ID"`
	EventID          string    `json:"eventID"`
	EventDateTime    time.Time `json:"eventDateTime"`
	PlateNumber      string    `json:"plateNumber"`
	PlateSt          string    `json:"plateSt"`
	PlateNumber2     string    `json:"plateNumber2"`
	Confidence       string    `json:"confidence"`
	VehicleMake      string    `json:"vehicleMake"`
	VehicleModel     string    `json:"vehicleModel"`
	VehicleColor     string    `json:"vehicleColor"`
	VehicleSize      string    `json:"vehicleSize"`
	VehicleType      string    `json:"vehicleType"`
	CameraID         string    `json:"cameraID"`
	CameraName       string    `json:"cameraName"`
	CameraType       string    `json:"cameraType"`
	Agency           string    `json:"agency"`
	Ori              string    `json:"ori"`
	Latitude         float64   `json:"latitude"`
	Longitude        float64   `json:"longitude"`
	Direction        string    `json:"direction"`
	ImageVehicle     string    `json:"imageVehicle"`
	ImagePlate       string    `json:"imagePlate"`
	AdditionalImage1 string    `json:"additionalImage1"`
	AdditionalImage2 string    `json:"additionalImage2"`
	ImageID          string    `json:"-"`
	SourceID         string    `json:"-"`
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

func toString(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	s := t.String
	return s
}
func toTime(ts pgtype.Timestamp) (time.Time, bool) {
	if ts.InfinityModifier != pgtype.Finite {
		return time.Time{}, false
	}
	if !ts.Valid {
		return time.Time{}, false
	} // or ts.Status != pgtype.Present
	return ts.Time, true
}

func buildHitDoc(ctx context.Context, job Job, q *db.Queries, w *wasabi.Wasabi) error {

	params := db.GetPlateHitParams{
		PlateID:   job.PlateID,
		HotlistID: job.HotlistID,
	}

	hits, err := q.GetPlateHit(ctx, params)
	if err != nil {
		return err
	}

	var plateHits = make([]PlateHit, 0, len(hits))
	for _, hit := range hits {
		phit := PlateHit{
			ID:               hit.ID,
			EventID:          hit.Eventid,
			PlateNumber:      hit.Platenumber,
			PlateSt:          toString(hit.Platest),
			PlateNumber2:     hit.Platenumber2,
			Confidence:       hit.Confidence,
			VehicleMake:      toString(hit.Vehiclemake),
			VehicleModel:     hit.Vehiclemodel,
			VehicleColor:     toString(hit.Vehiclecolor),
			VehicleSize:      hit.Vehiclesize,
			VehicleType:      toString(hit.Vehicletype),
			CameraID:         hit.Cameraid,
			CameraName:       toString(hit.Cameraname),
			CameraType:       hit.Cameratype,
			Agency:           hit.Agency,
			Ori:              hit.Ori,
			Latitude:         hit.Latitude.(float64),
			Longitude:        hit.Longitude.(float64),
			Direction:        hit.Direction,
			ImageVehicle:     hit.Imagevehicle,
			ImagePlate:       hit.Imageplate,
			AdditionalImage1: hit.Additionalimage1,
			AdditionalImage2: hit.Additionalimage2,
			ImageID:          toString(hit.ImageID),
			SourceID:         hit.SourceID.(string),
		}

		//date time, always fun.
		if t, ok := toTime(hit.Eventdatetime); ok {
			phit.EventDateTime = t
		}

		//build wasabi image links.
		phit = createImageLinks(phit, w)
		plateHits = append(plateHits, phit)

	}

	ph := PlateHits{Plates: plateHits}
	bytes, err := json.Marshal(ph)
	if err != nil {
		fmt.Printf("err : %v\n", err)
	}
	fmt.Println(string(bytes))

	return nil
}

func createImageLinks(plateHit PlateHit, w *wasabi.Wasabi) PlateHit {

	vehicle_img := fmt.Sprintf("alpr/%s/%s", plateHit.SourceID, plateHit.ImageID)
	vehicle_url, err := w.PresignUrl("njsnap", vehicle_img)
	if err == nil {
		plateHit.ImageVehicle = vehicle_url
	}

	plate_img := fmt.Sprintf("alpr-plate/%s/%s", plateHit.SourceID, plateHit.ImageID)
	plate_url, err := w.PresignUrl("njsnap", plate_img)
	if err == nil {
		plateHit.ImagePlate = plate_url
	}

	return plateHit
}

// A simple database poll to check for plate hits every 5 seconds.
// func StartAlertListener(ctx context.Context, pool *pgxpool.Pool, s Sender) error {
func StartAlertListener(ctx context.Context, pool *pgxpool.Pool, wasabi *wasabi.Wasabi, s Sender) error {
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
				_ = buildHitDoc(ctx, hitJob, q, wasabi)
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

		//start polling.
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
	}() //start go routing

	return nil
}
