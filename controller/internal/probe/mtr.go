package probe

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"time"

	"netwatcher-controller/internal/alert"
)

func initMtr(db *sql.DB, pg *gorm.DB) {
	Register(NewHandler[mtrPayload](
		TypeMTR,
		func(p mtrPayload) error {
			if len(p.Report.Hops) == 0 {
				return errors.New("no hops")
			}
			return nil
		},
		func(ctx context.Context, data ProbeData, p mtrPayload) error {
			if err := SaveRecordWithAlertEval(ctx, db, pg, data, string(TypeMTR), p); err != nil {
				log.WithError(err).Error("save mtr record (CH)")
				return err
			}

			captureMtrBaseline(ctx, pg, data.ProbeID, p)

			log.Printf("[mtr] probe=%d hops=%d triggered=%v",
				data.ProbeID, len(p.Report.Hops), data.Triggered)
			return nil
		},
	))
}

func captureMtrBaseline(ctx context.Context, pg *gorm.DB, probeID uint, p mtrPayload) {
	if pg == nil {
		return
	}
	payloadJSON, err := json.Marshal(p)
	if err != nil {
		log.WithError(err).Warnf("mtr: failed to marshal payload for baseline capture (probe=%d)", probeID)
		return
	}
	parsed, err := alert.ParseMtrPayload(payloadJSON)
	if err != nil {
		log.WithError(err).Warnf("mtr: failed to parse payload for baseline capture (probe=%d)", probeID)
		return
	}
	fp := alert.GetRouteFingerprint(parsed)
	path := alert.GetRoutePathString(parsed)
	hops := len(parsed.Report.Hops)
	if _, err := alert.EnsureRouteBaseline(ctx, pg, probeID, fp, path, hops); err != nil {
		log.WithError(err).Warnf("mtr: failed to ensure route baseline (probe=%d)", probeID)
		return
	}
	if refreshed, err := alert.RefreshRouteBaselineIfStale(ctx, pg, probeID, fp, path, hops, routeBaselineStaleThreshold); err != nil {
		log.WithError(err).Warnf("mtr: failed to refresh route baseline (probe=%d)", probeID)
	} else if refreshed {
		log.Infof("mtr: refreshed stale route baseline (probe=%d)", probeID)
	}
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
