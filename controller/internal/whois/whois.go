// Package whois provides WHOIS lookups using the system whois command.
package whois

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// -------------------- Errors --------------------

var (
	ErrInvalidIP      = errors.New("invalid IP address")
	ErrInvalidDomain  = errors.New("invalid domain name")
	ErrLookupFailed   = errors.New("whois lookup failed")
	ErrCommandTimeout = errors.New("whois command timed out")
)

// -------------------- Result Types --------------------

// Result contains parsed WHOIS information.
type Result struct {
	Query      string            `json:"query"`
	RawOutput  string            `json:"raw_output"`
	Parsed     map[string]string `json:"parsed,omitempty"`
	LookupTime time.Duration     `json:"lookup_time_ms"`
	Error      string            `json:"error,omitempty"`
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

// -------------------- Lookup --------------------

// Lookup performs a WHOIS query using the system whois command.
func Lookup(ctx context.Context, query string) (*Result, error) {
	startTime := time.Now()

	// Validate and sanitize input
	sanitized, err := ValidateQuery(query)
	if err != nil {
		return nil, err
	}

	// Create command with context for timeout
	cmd := exec.CommandContext(ctx, "whois", sanitized)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	duration := time.Since(startTime)

	if ctx.Err() == context.DeadlineExceeded {
		return nil, ErrCommandTimeout
	}

	result := &Result{
		Query:      sanitized,
		RawOutput:  stdout.String(),
		LookupTime: duration / time.Millisecond,
		Parsed:     make(map[string]string),
	}

	if err != nil {
		result.Error = stderr.String()
		if result.Error == "" {
			result.Error = err.Error()
		}
	}

	// Parse common fields
	result.Parsed = parseWhoisOutput(stdout.String())

	return result, nil
}

// LookupWithTimeout performs a WHOIS lookup with a specified timeout.
func LookupWithTimeout(query string, timeout time.Duration) (*Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return Lookup(ctx, query)
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
