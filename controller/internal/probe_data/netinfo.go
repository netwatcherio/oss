package probe_data

import (
	"context"
	"database/sql"
	log "github.com/sirupsen/logrus"
	"netwatcher-controller/internal/probe"
	"time"
)

func initNetInfo(db *sql.DB) {
	Register(NewHandler[netInfoPayload](
		probe.TypeNetInfo,
		func(p netInfoPayload) error {
			return nil
		},
		func(ctx context.Context, data ProbeData, p netInfoPayload) error {
			if err := SaveRecordCH(ctx, db, data, string(probe.TypeNetInfo), p); err != nil {
				log.WithError(err).Error("save netinfo record (CH)")
				return err
			}

			// Store to DB / compute / alert as needed:
			log.Printf("[netinfo] id=%d wan=%s lan=%s gw=%s",
				data.ID, p.PublicAddress, p.LocalAddress, p.DefaultGateway)
			return nil
		},
	))
}

type netInfoPayload struct {
	LocalAddress     string    `json:"local_address" bson:"local_address"`
	DefaultGateway   string    `json:"default_gateway" bson:"default_gateway"`
	PublicAddress    string    `json:"public_address" bson:"public_address"`
	InternetProvider string    `json:"internet_provider" bson:"internet_provider"`
	Lat              string    `json:"lat" bson:"lat"`
	Long             string    `json:"long" bson:"long"`
	Timestamp        time.Time `json:"timestamp" bson:"timestamp"`
}
