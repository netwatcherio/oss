package oui

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// Config holds OUI store configuration.
type Config struct {
	Path string // Path to oui.txt file
}

// LoadConfigFromEnv loads OUI configuration from environment variables.
func LoadConfigFromEnv() Config {
	return Config{
		Path: os.Getenv("OUI_PATH"),
	}
}

// Entry represents a single OUI entry.
type Entry struct {
	OUI         string `json:"oui"`    // e.g., "00:1C:42"
	Vendor      string `json:"vendor"` // e.g., "Parallels, Inc."
	Address     string `json:"address,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
}

// Store provides OUI lookup functionality.
type Store struct {
	mu          sync.RWMutex
	entries     map[string]Entry // Key: normalized OUI (uppercase, no separators)
	path        string
	sourcePath  string
	loaded      bool
	loadedAt    time.Time
	parseErrors int
}

// NewStore creates a new OUI store from config.
func NewStore(cfg Config) *Store {
	return &Store{
		entries: make(map[string]Entry),
		path:    cfg.Path,
	}
}

// Load loads OUI data from the configured file path.
// Returns nil if path is not configured (OUI lookup will be disabled).
func (s *Store) Load() error {
	if s.path == "" {
		log.Info("OUI_PATH not configured, MAC vendor lookup disabled")
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.loadFromFile()
}

// loadFromFile loads OUI data from the configured file path.
func (s *Store) loadFromFile() error {
	info, err := os.Stat(s.path)
	if err != nil {
		return fmt.Errorf("OUI file not found at %s: %w", s.path, err)
	}

	abs, err := filepath.Abs(s.path)
	if err != nil {
		abs = s.path
	}
	s.sourcePath = abs

	file, err := os.Open(s.path)
	if err != nil {
		return fmt.Errorf("could not open OUI file: %w", err)
	}
	defer file.Close()

	count, errs := s.parseOUIData(file)
	if count == 0 {
		return fmt.Errorf("no OUI entries found in %s", s.path)
	}

	s.parseErrors = errs
	s.loaded = true
	s.loadedAt = info.ModTime()
	log.Infof("Loaded %d OUI entries from %s (%d parse errors)", count, s.path, errs)
	return nil
}

// parseOUIData parses IEEE OUI format.
// Format: "00-1C-42   (hex)		Parallels, Inc."
func (s *Store) parseOUIData(r io.Reader) (int, int) {
	// Pattern matches: "XX-XX-XX   (hex)		Vendor Name"
	hexPattern := regexp.MustCompile(`^([0-9A-Fa-f]{2}-[0-9A-Fa-f]{2}-[0-9A-Fa-f]{2})\s+\(hex\)\s+(.+)$`)

	scanner := bufio.NewScanner(r)
	count := 0
	parseErrors := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		matches := hexPattern.FindStringSubmatch(line)
		if matches == nil {
			// Count as a parse error only if the line looks like it should
			// have been an entry (i.e. contains "("), otherwise it's just
			// a header/separator line in the IEEE file format.
			if strings.Contains(line, "(") {
				parseErrors++
			}
			continue
		}

		oui := strings.ToUpper(strings.ReplaceAll(matches[1], "-", ""))
		vendor := strings.TrimSpace(matches[2])

		s.entries[oui] = Entry{
			OUI:    matches[1],
			Vendor: vendor,
		}
		count++
	}

	return count, parseErrors
}

// Lookup looks up a MAC address and returns the vendor.
// Accepts various MAC formats: 00:1C:42:XX:XX:XX, 00-1C-42-XX-XX-XX, 001C42XXXXXX.
// Returns a non-nil error when the input is not a valid 6- or 12-hex-char MAC
// (after URL-decoding and separator-stripping), so the handler can return 400.
func (s *Store) Lookup(mac string) (*Entry, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.loaded {
		return nil, false, nil
	}

	normalized, err := normalizeMAC(mac)
	if err != nil {
		return nil, false, fmt.Errorf("invalid MAC %q: %w", mac, err)
	}
	if len(normalized) != 6 && len(normalized) != 12 {
		return nil, false, fmt.Errorf("invalid MAC %q: expected 6 or 12 hex chars, got %d", mac, len(normalized))
	}
	if !isAllHex(normalized) {
		return nil, false, fmt.Errorf("invalid MAC %q: non-hex characters after normalization", mac)
	}

	// Take first 6 characters (3 bytes = OUI)
	oui := normalized[:6]

	entry, ok := s.entries[oui]
	if !ok {
		return nil, false, nil
	}

	return &entry, true, nil
}

// LookupBulk looks up multiple MAC addresses. Per-MAC parse errors are
// returned as the third map value so a single bad input does not sink
// the rest of the batch.
func (s *Store) LookupBulk(macs []string) (map[string]*Entry, map[string]error) {
	hits := make(map[string]*Entry)
	errs := make(map[string]error)
	for _, mac := range macs {
		if entry, ok, err := s.Lookup(mac); err != nil {
			errs[mac] = err
		} else if ok {
			hits[mac] = entry
		}
	}
	return hits, errs
}

// IsLoaded returns true if the store has loaded OUI data.
func (s *Store) IsLoaded() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loaded
}

// EntryCount returns the number of loaded OUI entries.
func (s *Store) EntryCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

// ParseErrors returns the number of lines in the source file that looked
// like entries but failed to parse. Zero in a healthy install.
func (s *Store) ParseErrors() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.parseErrors
}

// SourcePath returns the absolute path the loaded OUI file was read from.
func (s *Store) SourcePath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sourcePath
}

// LoadedAt returns the modification time of the OUI file at the moment it
// was loaded into memory. Useful for verifying freshness in /status.
func (s *Store) LoadedAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadedAt
}

// normalizeMAC URL-decodes the input (so clients can send `%3A` etc.) and
// strips common separators. Returns an error if the input cannot be
// percent-decoded.
func normalizeMAC(mac string) (string, error) {
	decoded, err := url.PathUnescape(mac)
	if err != nil {
		return "", err
	}
	cleaned := strings.ToUpper(decoded)
	cleaned = strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(cleaned)
	return cleaned, nil
}

// isAllHex returns true if s is non-empty and every rune is a hex digit.
func isAllHex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}
