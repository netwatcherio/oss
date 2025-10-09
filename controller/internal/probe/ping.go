package probe

import (
	"context"
	"database/sql"
	log "github.com/sirupsen/logrus"
	"time"
)

func initPing(db *sql.DB) {
	Register(NewHandler[PingPayload](
		TypePing,
		func(p PingPayload) error {
			return nil
		},
		func(ctx context.Context, data ProbeData, p PingPayload) error {
			if err := SaveRecordCH(ctx, db, data, string(TypePing), p); err != nil {
				log.WithError(err).Error("save ping record (CH)")
				return err
			}

			// Store to DB / compute / alert as needed:
			log.Printf("[ping] pid=%d ploss=%v rtt=%d",
				data.ProbeID, p.PacketLoss, p.AvgRtt)
			return nil
		},
	))
}

type PingPayload struct {
	StartTimestamp        time.Time     `json:"start_timestamp" bson:"start_timestamp"`
	StopTimestamp         time.Time     `json:"stop_timestamp" bson:"stop_timestamp"`
	PacketsRecv           int           `json:"packets_recv" bson:"packets_recv"`
	PacketsSent           int           `json:"packets_sent" bson:"packets_sent"`
	PacketsRecvDuplicates int           `json:"packets_recv_duplicates" bson:"packets_recv_duplicates"`
	PacketLoss            float64       `json:"packet_loss" bson:"packet_loss"`
	Addr                  string        `json:"addr" bson:"addr"`
	MinRtt                time.Duration `json:"min_rtt" bson:"min_rtt"`
	MaxRtt                time.Duration `json:"max_rtt" bson:"max_rtt"`
	AvgRtt                time.Duration `json:"avg_rtt" bson:"avg_rtt"`
	StdDevRtt             time.Duration `json:"std_dev_rtt" bson:"std_dev_rtt"`
}
