package probe

import (
	"context"
	"database/sql"
	"time"

	log "github.com/sirupsen/logrus"
)

func initTrafficSim(db *sql.DB) {
	Register(NewHandler[TrafficSimResult](
		TypeTrafficSim,
		nil,
		func(ctx context.Context, data ProbeData, p TrafficSimResult) error {
			if err := SaveRecordCH(ctx, db, data, string(TypeTrafficSim), p); err != nil {
				log.WithError(err).Error("save trafficsim record (CH)")
				return err
			}

			log.Printf("[trafficsim] pid=%d agent=%d loss=%.2f%% avgRTT=%.2fms",
				data.ProbeID, data.AgentID, p.LossPercentage, p.AverageRTT)
			return nil
		},
	))
}

// TrafficSimResult represents the cycle-based statistics from TrafficSim UDP probes
type TrafficSimResult struct {
	// Packet statistics
	LostPackets      int     `json:"lostPackets"`
	LossPercentage   float64 `json:"lossPercentage"`
	OutOfSequence    int     `json:"outOfSequence"`
	DuplicatePackets int     `json:"duplicatePackets"`
	TotalPackets     int     `json:"totalPackets"`

	// RTT statistics (in milliseconds)
	AverageRTT float64 `json:"averageRTT"`
	MinRTT     int64   `json:"minRTT"`
	MaxRTT     int64   `json:"maxRTT"`
	StdDevRTT  float64 `json:"stdDevRTT"`

	// Cycle range (sequence numbers)
	SequenceRange string `json:"sequenceRange,omitempty"`

	// Flow-level statistics
	Flows map[string]interface{} `json:"flows,omitempty"`

	// Timestamps
	ReportTime time.Time `json:"reportTime"`
	Timestamp  time.Time `json:"timestamp"`
}
