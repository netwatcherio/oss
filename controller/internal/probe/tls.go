package probe

import (
	"context"
	"database/sql"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type TLSPayload struct {
	StartTimestamp   time.Time   `json:"start_timestamp"`
	StopTimestamp    time.Time   `json:"stop_timestamp"`
	RemoteAddr       string      `json:"remote_addr"`
	Protocol         string      `json:"protocol"`
	TLSVersion       string      `json:"tls_version"`
	TLSCipherSuite   string      `json:"tls_cipher_suite"`
	Certificate      *ChainCert  `json:"certificate,omitempty"`
	CertificateChain []ChainCert `json:"certificate_chain,omitempty"`
	IsExpired        bool        `json:"is_expired"`
	IsExpiringSoon   bool        `json:"is_expiring_soon"`
	DaysUntilExpiry  int         `json:"days_until_expiry"`
	IssuerOrg        string      `json:"issuer_org"`
	CertType         string      `json:"cert_type"`
	CertFingerprint  string      `json:"cert_fingerprint"`
	Error            string      `json:"error,omitempty"`
}

type ChainCert struct {
	Subject         string    `json:"subject"`
	Issuer          string    `json:"issuer"`
	NotBefore       time.Time `json:"not_before"`
	NotAfter        time.Time `json:"not_after"`
	DaysUntilExpiry int       `json:"days_until_expiry"`
	IssuerOrg       string    `json:"issuer_org"`
	Fingerprint     string    `json:"fingerprint"`
}

func initTLS(ch *sql.DB, pg *gorm.DB) {
	Register(NewHandler[TLSPayload](
		TypeTLS,
		func(p TLSPayload) error {
			return nil
		},
		func(ctx context.Context, data ProbeData, p TLSPayload) error {
			if err := SaveRecordWithAlertEval(ctx, ch, pg, data, string(TypeTLS), p); err != nil {
				log.WithError(err).Error("save TLS record (CH)")
				return err
			}

			certInfo := ""
			if p.Certificate != nil {
				certInfo = p.Certificate.Subject
			}

			log.Printf("[tls] pid=%d target=%s protocol=%s version=%s cipher=%s days=%d expired=%v cert=%s",
				data.ProbeID, data.Target, p.Protocol, p.TLSVersion, p.TLSCipherSuite, p.DaysUntilExpiry, p.IsExpired, certInfo)
			return nil
		},
	))
}
