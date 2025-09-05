package probe_data

import (
	"context"
	log "github.com/sirupsen/logrus"
	"netwatcher-controller/internal/probe"
	"time"
)

func initNetInfo() {
	Register(NewHandler[netInfoPayload](
		probe.TypeNetInfo,
		func(p netInfoPayload) error {
			return nil
		},
		func(ctx context.Context, data ProbeData, p netInfoPayload) error {
			// Store to DB / compute / alert as needed:
			log.Printf("[netinfo] id=%d probe=%d hops=%d triggered=%v",
				data.ID, data.ProbeID, len(p.Lat), data.Triggered)
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
