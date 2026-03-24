package probe

import (
	"context"
	"database/sql"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// DNSPayload matches the agent's dns.DNSPayload struct
type DNSPayload struct {
	DNSServer    string      `json:"dns_server"`
	RecordType   string      `json:"record_type"`
	QueryTimeMs  float64     `json:"query_time_ms"`
	ResponseCode string      `json:"response_code"`
	Answers      []DNSAnswer `json:"answers"`
	RawResponse  string      `json:"raw_response"`
	Error        string      `json:"error,omitempty"`
	Protocol     string      `json:"protocol"`
	Target       string      `json:"target"`
}

// DNSAnswer represents a single DNS answer record
type DNSAnswer struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
	TTL   uint32 `json:"ttl"`
}

func initDns(ch *sql.DB, pg *gorm.DB) {
	Register(NewHandler[DNSPayload](
		TypeDNS,
		func(p DNSPayload) error {
			return nil
		},
		func(ctx context.Context, data ProbeData, p DNSPayload) error {
			if err := SaveRecordWithAlertEval(ctx, ch, pg, data, string(TypeDNS), p); err != nil {
				log.WithError(err).Error("save DNS record (CH)")
				return err
			}

			log.Printf("[dns] pid=%d server=%s type=%s target=%s rcode=%s time=%.2fms answers=%d",
				data.ProbeID, p.DNSServer, p.RecordType, p.Target, p.ResponseCode, p.QueryTimeMs, len(p.Answers))
			return nil
		},
	))
}
