package probe

import (
	"context"
	"database/sql"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type HTTPPayload struct {
	StartTimestamp    time.Time         `json:"start_timestamp"`
	StopTimestamp     time.Time         `json:"stop_timestamp"`
	DNSLookupMs       float64           `json:"dns_lookup_ms"`
	TCPConnectMs      float64           `json:"tcp_connect_ms"`
	TLSHandshakeMs    float64           `json:"tls_handshake_ms"`
	FirstByteMs       float64           `json:"first_byte_ms"`
	TotalMs           float64           `json:"total_ms"`
	URL               string            `json:"url"`
	StatusCode        int               `json:"status_code"`
	StatusText        string            `json:"status_text"`
	Headers           map[string]string `json:"headers"`
	BodySize          int64             `json:"body_size"`
	ContentType       string            `json:"content_type"`
	RemoteAddr        string            `json:"remote_addr"`
	Protocol          string            `json:"protocol"`
	TLSVersion        string            `json:"tls_version,omitempty"`
	TLSCipherSuite    string            `json:"tls_cipher_suite,omitempty"`
	CertificateInfo   *CertInfo         `json:"certificate_info,omitempty"`
	ContentMatch      bool              `json:"content_match"`
	ContentMatchFound string            `json:"content_match_found,omitempty"`
	Error             string            `json:"error,omitempty"`
}

type CertInfo struct {
	Subject         string    `json:"subject"`
	Issuer          string    `json:"issuer"`
	NotBefore       time.Time `json:"not_before"`
	NotAfter        time.Time `json:"not_after"`
	DaysUntilExpiry int       `json:"days_until_expiry"`
	SANs            []string  `json:"sans"`
}

func initHTTP(ch *sql.DB, pg *gorm.DB) {
	Register(NewHandler[HTTPPayload](
		TypeHTTP,
		func(p HTTPPayload) error {
			return nil
		},
		func(ctx context.Context, data ProbeData, p HTTPPayload) error {
			if err := SaveRecordWithAlertEval(ctx, ch, pg, data, string(TypeHTTP), p); err != nil {
				log.WithError(err).Error("save HTTP record (CH)")
				return err
			}

			target := p.URL
			if target == "" {
				target = data.Target
			}

			certInfo := ""
			if p.CertificateInfo != nil {
				certInfo = p.CertificateInfo.Subject
			}

			log.Printf("[http] pid=%d url=%s status=%d time=%.2fms tls=%s cipher=%s cert=%s",
				data.ProbeID, target, p.StatusCode, p.TotalMs, p.TLSVersion, p.TLSCipherSuite, certInfo)
			return nil
		},
	))
}
