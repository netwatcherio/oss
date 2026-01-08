// internal/email/smtp.go
package email

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"os"
	"strconv"
	"strings"
)

// SMTPConfig holds SMTP configuration
type SMTPConfig struct {
	Host       string
	Port       int
	User       string
	Password   string
	FromEmail  string
	FromName   string
	UseTLS     bool
	SkipVerify bool
}

// LoadSMTPConfigFromEnv loads SMTP configuration from environment variables
func LoadSMTPConfigFromEnv() *SMTPConfig {
	port := 587
	if p := os.Getenv("SMTP_PORT"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil {
			port = parsed
		}
	}

	useTLS := true
	if t := os.Getenv("SMTP_TLS"); t != "" {
		useTLS = strings.ToLower(t) == "true" || t == "1"
	}

	skipVerify := false
	if s := os.Getenv("SMTP_SKIP_VERIFY"); s != "" {
		skipVerify = strings.ToLower(s) == "true" || s == "1"
	}

	return &SMTPConfig{
		Host:       os.Getenv("SMTP_HOST"),
		Port:       port,
		User:       os.Getenv("SMTP_USER"),
		Password:   os.Getenv("SMTP_PASSWORD"),
		FromEmail:  getenvDefault("SMTP_FROM_EMAIL", "noreply@netwatcher.io"),
		FromName:   getenvDefault("SMTP_FROM_NAME", "NetWatcher"),
		UseTLS:     useTLS,
		SkipVerify: skipVerify,
	}
}

// IsConfigured returns true if SMTP is properly configured
func (c *SMTPConfig) IsConfigured() bool {
	return c.Host != "" && c.FromEmail != ""
}

// Sender handles sending emails via SMTP
type Sender struct {
	config *SMTPConfig
}

// NewSender creates a new email sender
func NewSender(config *SMTPConfig) *Sender {
	return &Sender{config: config}
}

// Send sends an email
func (s *Sender) Send(email *EmailQueue) error {
	if !s.config.IsConfigured() {
		return fmt.Errorf("SMTP not configured")
	}

	// Build headers
	from := s.config.FromEmail
	if s.config.FromName != "" {
		from = fmt.Sprintf("%s <%s>", s.config.FromName, s.config.FromEmail)
	}

	to := email.ToEmail
	if email.ToName != "" {
		to = fmt.Sprintf("%s <%s>", email.ToName, email.ToEmail)
	}

	// Determine content type
	var body string
	contentType := "text/plain"
	if email.BodyHTML != "" {
		body = email.BodyHTML
		contentType = "text/html"
	} else {
		body = email.Body
	}

	// Build message
	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: %s; charset=\"utf-8\"\r\n"+
		"\r\n"+
		"%s",
		from, to, email.Subject, contentType, body)

	// Connect and send
	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	var auth smtp.Auth
	if s.config.User != "" && s.config.Password != "" {
		auth = smtp.PlainAuth("", s.config.User, s.config.Password, s.config.Host)
	}

	if s.config.UseTLS {
		return s.sendWithTLS(addr, auth, s.config.FromEmail, []string{email.ToEmail}, []byte(msg))
	}

	return smtp.SendMail(addr, auth, s.config.FromEmail, []string{email.ToEmail}, []byte(msg))
}

// sendWithTLS sends email using STARTTLS
func (s *Sender) sendWithTLS(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	conn, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}
	defer conn.Close()

	// Try STARTTLS
	tlsConfig := &tls.Config{
		ServerName:         s.config.Host,
		InsecureSkipVerify: s.config.SkipVerify,
	}

	if ok, _ := conn.Extension("STARTTLS"); ok {
		if err := conn.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("starttls failed: %w", err)
		}
	}

	// Authenticate if credentials provided
	if auth != nil {
		if ok, _ := conn.Extension("AUTH"); ok {
			if err := conn.Auth(auth); err != nil {
				return fmt.Errorf("auth failed: %w", err)
			}
		}
	}

	// Send
	if err := conn.Mail(from); err != nil {
		return fmt.Errorf("mail from failed: %w", err)
	}

	for _, addr := range to {
		if err := conn.Rcpt(addr); err != nil {
			return fmt.Errorf("rcpt to failed: %w", err)
		}
	}

	w, err := conn.Data()
	if err != nil {
		return fmt.Errorf("data failed: %w", err)
	}

	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("close failed: %w", err)
	}

	return conn.Quit()
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
