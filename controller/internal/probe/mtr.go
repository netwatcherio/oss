package probe

import (
	"context"
	"database/sql"
	"errors"
	log "github.com/sirupsen/logrus"
	"time"
)

func initMtr(db *sql.DB) {
	Register(NewHandler[mtrPayload](
		TypeMTR,
		func(p mtrPayload) error {
			if len(p.Report.Hops) == 0 {
				return errors.New("no hops")
			}
			return nil
		},
		func(ctx context.Context, data ProbeData, p mtrPayload) error {
			if err := SaveRecordCH(ctx, db, data, string(TypeMTR), p); err != nil {
				log.WithError(err).Error("save mtr record (CH)")
				return err
			}

			// Store to DB / compute / alert as needed:
			log.Printf("[mtr] probe=%d hops=%d triggered=%v",
				data.ProbeID, len(p.Report.Hops), data.Triggered)
			return nil
		},
	))
}

type mtrPayload struct {
	StartTimestamp time.Time `json:"start_timestamp" bson:"start_timestamp"`
	StopTimestamp  time.Time `json:"stop_timestamp" bson:"stop_timestamp"`
	Report         struct {
		Info struct {
			Target struct {
				IP       string `json:"ip" bson:"ip"`
				Hostname string `json:"hostname" bson:"hostname"`
			} `json:"target" bson:"target"`
		} `json:"info" bson:"info"`
		Hops []struct {
			TTL   int `json:"ttl" bson:"ttl"`
			Hosts []struct {
				IP       string `json:"ip" bson:"ip"`
				Hostname string `json:"hostname" bson:"hostname"`
			} `json:"hosts" bson:"hosts"`
			Extensions []string `json:"extensions" bson:"extensions"`
			LossPct    string   `json:"loss_pct" bson:"loss_pct"`
			Sent       int      `json:"sent" bson:"sent"`
			Last       string   `json:"last" bson:"last"`
			Recv       int      `json:"recv" bson:"recv"`
			Avg        string   `json:"avg" bson:"avg"`
			Best       string   `json:"best" bson:"best"`
			Worst      string   `json:"worst" bson:"worst"`
			StdDev     string   `json:"stddev" bson:"stddev"`
		} `json:"hops" bson:"hops"`
	} `json:"report" bson:"report"`
}
