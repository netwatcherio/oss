// Package whois provides WHOIS lookups using the system whois command.
package whois

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// -------------------- Configuration --------------------

// DefaultTimeout is the default timeout for WHOIS lookups.
// Can be overridden via WHOIS_TIMEOUT_SECONDS environment variable.
var DefaultTimeout = 5 * time.Second

func init() {
	if envTimeout := os.Getenv("WHOIS_TIMEOUT_SECONDS"); envTimeout != "" {
		if secs, err := strconv.Atoi(envTimeout); err == nil && secs > 0 {
			DefaultTimeout = time.Duration(secs) * time.Second
			log.Infof("WHOIS timeout set to %v from environment", DefaultTimeout)
		}
	}
}

// -------------------- Errors --------------------

var (
	ErrInvalidIP      = errors.New("invalid IP address")
	ErrInvalidDomain  = errors.New("invalid domain name")
	ErrLookupFailed   = errors.New("whois lookup failed")
	ErrCommandTimeout = errors.New("whois command timed out")
	ErrWhoisNotFound  = errors.New("whois command not found")
)

// -------------------- Result Types --------------------

// Result contains parsed WHOIS information.
type Result struct {
	Query      string            `json:"query"`
	RawOutput  string            `json:"raw_output"`
	Parsed     map[string]string `json:"parsed,omitempty"`
	LookupTime time.Duration     `json:"lookup_time_ms"`
	Error      string            `json:"error,omitempty"`
	Server     string            `json:"server,omitempty"` // WHOIS server used
}

// -------------------- Validation --------------------

// domainRegex validates domain names (simple check)
var domainRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z]{2,})+$`)

// ValidateIP checks if the input is a valid IP address.
func ValidateIP(input string) (string, error) {
	ip := net.ParseIP(strings.TrimSpace(input))
	if ip == nil {
		return "", ErrInvalidIP
	}
	return ip.String(), nil
}

// ValidateDomain checks if the input is a valid domain name.
func ValidateDomain(input string) (string, error) {
	input = strings.TrimSpace(strings.ToLower(input))
	if !domainRegex.MatchString(input) {
		return "", ErrInvalidDomain
	}
	return input, nil
}

// ValidateQuery validates and sanitizes input for WHOIS lookup.
// Accepts IP addresses and domain names.
func ValidateQuery(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("empty query")
	}

	// Try IP first
	if ip, err := ValidateIP(input); err == nil {
		return ip, nil
	}

	// Try domain
	if domain, err := ValidateDomain(input); err == nil {
		return domain, nil
	}

	return "", errors.New("invalid IP or domain")
}

// -------------------- WHOIS Servers --------------------

// whoisServers maps RIR regions to their WHOIS servers for IP lookups.
// This can help when the default server selection is slow.
var whoisServers = map[string]string{
	"arin":    "whois.arin.net",
	"ripe":    "whois.ripe.net",
	"apnic":   "whois.apnic.net",
	"lacnic":  "whois.lacnic.net",
	"afrinic": "whois.afrinic.net",
}

// getWhoisServer returns an appropriate WHOIS server for the query type.
// Returns empty string to use system default.
func getWhoisServer(query string) string {
	// For now, let the whois command determine the server
	// Could be enhanced to detect IP ranges and route to specific RIRs
	return ""
}

// -------------------- Lookup --------------------

// Lookup performs a WHOIS query using the system whois command.
func Lookup(ctx context.Context, query string) (*Result, error) {
	startTime := time.Now()

	// Validate and sanitize input
	sanitized, err := ValidateQuery(query)
	if err != nil {
		return nil, err
	}

	// Check if whois command exists
	whoisPath, err := exec.LookPath("whois")
	if err != nil {
		return nil, ErrWhoisNotFound
	}

	// Build command args
	args := []string{sanitized}

	// Optionally specify a server for faster lookups
	if server := getWhoisServer(sanitized); server != "" {
		args = append([]string{"-h", server}, args...)
	}

	// Create command with context for timeout
	cmd := exec.CommandContext(ctx, whoisPath, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	duration := time.Since(startTime)

	// Check for timeout first
	if ctx.Err() == context.DeadlineExceeded {
		return &Result{
			Query:      sanitized,
			LookupTime: duration / time.Millisecond,
			Error:      "lookup timed out",
			Parsed:     make(map[string]string),
		}, ErrCommandTimeout
	}

	result := &Result{
		Query:      sanitized,
		RawOutput:  stdout.String(),
		LookupTime: duration / time.Millisecond,
		Parsed:     make(map[string]string),
	}

	if err != nil {
		errMsg := stderr.String()
		if errMsg == "" {
			errMsg = err.Error()
		}
		result.Error = errMsg
		// Don't return error - we still have partial result
		log.WithField("query", sanitized).WithField("error", errMsg).Debug("WHOIS lookup error")
	}

	// Only parse if we got output
	if stdout.Len() > 0 {
		result.Parsed = parseWhoisOutput(stdout.String())
	}

	return result, nil
}

// LookupWithTimeout performs a WHOIS lookup with a specified timeout.
func LookupWithTimeout(query string, timeout time.Duration) (*Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return Lookup(ctx, query)
}

// QuickLookup performs a WHOIS lookup with the default (shorter) timeout.
func QuickLookup(query string) (*Result, error) {
	return LookupWithTimeout(query, DefaultTimeout)
}

// -------------------- Parsing --------------------

// parseWhoisOutput extracts common fields from WHOIS output.
func parseWhoisOutput(output string) map[string]string {
	parsed := make(map[string]string)
	lines := strings.Split(output, "\n")

	// Common field mappings (case-insensitive)
	fieldMappings := map[string][]string{
		"netname":      {"NetName", "netname", "network-name"},
		"netrange":     {"NetRange", "inetnum", "CIDR"},
		"organization": {"OrgName", "Organization", "org-name", "descr"},
		"country":      {"Country", "country"},
		"registrar":    {"Registrar", "registrar"},
		"created":      {"Created", "RegDate", "created"},
		"updated":      {"Updated", "changed", "last-modified"},
		"abuse_email":  {"OrgAbuseEmail", "abuse-mailbox", "e-mail"},
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "%") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Check against known field mappings
		for fieldName, aliases := range fieldMappings {
			for _, alias := range aliases {
				if strings.EqualFold(key, alias) {
					// Only set if not already set (first match wins)
					if _, exists := parsed[fieldName]; !exists {
						parsed[fieldName] = value
					}
					break
				}
			}
		}
	}

	return parsed
}
