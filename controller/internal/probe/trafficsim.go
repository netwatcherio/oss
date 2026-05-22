package probe

import (
	"context"
	"database/sql"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func initTrafficSim(db *sql.DB, pg *gorm.DB) {
	Register(NewHandler[TrafficSimResult](
		TypeTrafficSim,
		nil,
		func(ctx context.Context, data ProbeData, p TrafficSimResult) error {
			log.Infof("[trafficsim] RAW payload bytes: %s", string(data.Payload))
			log.Infof("[trafficsim] Parsed TrafficSimResult: %+v", p)

			if err := SaveRecordWithAlertEval(ctx, db, pg, data, string(TypeTrafficSim), p); err != nil {
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
	LostPacketsRaw   int     `json:"lostPacketsRaw,omitempty"`
	LossPercentage   float64 `json:"lossPercentage"`
	OutOfSequence    int     `json:"outOfSequence"`
	DuplicatePackets int     `json:"duplicatePackets"`
	TotalPackets     int     `json:"totalPackets"`

	// RTT statistics (in milliseconds)
	AverageRTT float64 `json:"averageRTT"`
	MedianRTT  float64 `json:"medianRTT,omitempty"`
	P95RTT     float64 `json:"p95RTT,omitempty"`
	P99RTT     float64 `json:"p99RTT,omitempty"`
	MinRTT     int64   `json:"minRTT"`
	MaxRTT     int64   `json:"maxRTT"`
	StdDevRTT  float64 `json:"stdDevRTT"`

	// Jitter statistics (RFC 3550 style - inter-packet delay variation in milliseconds)
	JitterAvg float64 `json:"jitterAvg"` // stddev of RTTs (backwards compatible with stdDevRTT)
	JitterMed float64 `json:"jitterMedian,omitempty"`
	JitterP95 float64 `json:"jitterP95,omitempty"`

	// Cycle range (sequence numbers)
	SequenceRange string `json:"sequenceRange,omitempty"`

	// Flow-level statistics
	Flows map[string]interface{} `json:"flows,omitempty"`

	// Timestamps
	Timestamp time.Time `json:"timestamp"`
}
