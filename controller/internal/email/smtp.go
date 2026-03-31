// internal/email/smtp.go
package email

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
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

	from := s.config.FromEmail
	if s.config.FromName != "" {
		from = fmt.Sprintf("%s <%s>", s.config.FromName, s.config.FromEmail)
	}

	to := email.ToEmail
	if email.ToName != "" {
		to = fmt.Sprintf("%s <%s>", email.ToName, email.ToEmail)
	}

	var msg []byte
	if len(email.AttachmentContent) > 0 {
		msg = s.buildMultipartMessage(from, to, email.Subject, email.BodyHTML, email.Body, email.AttachmentName, email.AttachmentContent)
	} else {
		msg = s.buildSimpleMessage(from, to, email.Subject, email.BodyHTML, email.Body)
	}

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	var auth smtp.Auth
	if s.config.User != "" && s.config.Password != "" {
		auth = smtp.PlainAuth("", s.config.User, s.config.Password, s.config.Host)
	}

	if s.config.UseTLS {
		return s.sendWithTLS(addr, auth, s.config.FromEmail, []string{email.ToEmail}, msg)
	}

	return smtp.SendMail(addr, auth, s.config.FromEmail, []string{email.ToEmail}, msg)
}

func (s *Sender) buildSimpleMessage(from, to, subject, bodyHTML, body string) []byte {
	var contentType string
	var bodyContent string
	if bodyHTML != "" {
		contentType = "text/html; charset=\"utf-8\""
		bodyContent = bodyHTML
	} else {
		contentType = "text/plain; charset=\"utf-8\""
		bodyContent = body
	}

	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: %s\r\n"+
		"Date: %s\r\n"+
		"\r\n"+
		"%s",
		from, to, subject, contentType, time.Now().Format(time.RFC1123Z), bodyContent)
	return []byte(msg)
}

func (s *Sender) buildMultipartMessage(from, to, subject, bodyHTML, body, attachmentName string, attachmentContent []byte) []byte {
	boundary := fmt.Sprintf("==%d==", time.Now().UnixNano())

	var alternativePart string
	if bodyHTML != "" {
		alternativePart = fmt.Sprintf(
			"--%s\r\n"+
				"Content-Type: text/html; charset=\"utf-8\"\r\n"+
				"\r\n"+
				"%s\r\n"+
				"\r\n"+
				"--%s\r\n"+
				"Content-Type: text/plain; charset=\"utf-8\"\r\n"+
				"\r\n"+
				"%s\r\n",
			boundary, bodyHTML, boundary, body)
	} else {
		alternativePart = fmt.Sprintf(
			"--%s\r\n"+
				"Content-Type: text/plain; charset=\"utf-8\"\r\n"+
				"\r\n"+
				"%s\r\n",
			boundary, body)
	}

	encoded := base64.StdEncoding.EncodeToString(attachmentContent)
	encodedLines := s.splitBase64Lines(encoded)

	attachmentPart := fmt.Sprintf(
		"--%s\r\n"+
			"Content-Type: application/pdf; name=\"%s\"\r\n"+
			"Content-Transfer-Encoding: base64\r\n"+
			"Content-Disposition: attachment; filename=\"%s\"\r\n"+
			"\r\n"+
			"%s\r\n",
		boundary, attachmentName, attachmentName, strings.Join(encodedLines, "\r\n"))

	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: multipart/mixed; boundary=\"%s\"\r\n"+
		"Date: %s\r\n"+
		"\r\n"+
		"--%s\r\n"+
		"Content-Type: multipart/alternative; boundary=\"%s\"\r\n"+
		"\r\n"+
		"%s"+
		"--%s--\r\n"+
		"\r\n"+
		"%s"+
		"--%s--\r\n",
		from, to, subject, boundary, time.Now().Format(time.RFC1123Z),
		boundary, boundary, alternativePart,
		boundary, attachmentPart, boundary)

	return []byte(msg)
}

func (s *Sender) splitBase64Lines(encoded string) []string {
	const lineLen = 76
	lines := make([]string, 0, (len(encoded)+lineLen-1)/lineLen)
	for i := 0; i < len(encoded); i += lineLen {
		end := i + lineLen
		if end > len(encoded) {
			end = len(encoded)
		}
		lines = append(lines, encoded[i:end])
	}
	return lines
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
