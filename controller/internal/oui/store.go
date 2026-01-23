package oui

import (
	"bufio"
	"fmt"
	"io"
	"os"
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
	mu       sync.RWMutex
	entries  map[string]Entry // Key: normalized OUI (uppercase, no separators)
	path     string
	loaded   bool
	loadedAt time.Time
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

	file, err := os.Open(s.path)
	if err != nil {
		return fmt.Errorf("could not open OUI file: %w", err)
	}
	defer file.Close()

	count := s.parseOUIData(file)
	if count == 0 {
		return fmt.Errorf("no OUI entries found in %s", s.path)
	}

	s.loaded = true
	s.loadedAt = info.ModTime()
	log.Infof("Loaded %d OUI entries from %s", count, s.path)
	return nil
}

// parseOUIData parses IEEE OUI format.
// Format: "00-1C-42   (hex)		Parallels, Inc."
func (s *Store) parseOUIData(r io.Reader) int {
	// Pattern matches: "XX-XX-XX   (hex)		Vendor Name"
	hexPattern := regexp.MustCompile(`^([0-9A-Fa-f]{2}-[0-9A-Fa-f]{2}-[0-9A-Fa-f]{2})\s+\(hex\)\s+(.+)$`)

	scanner := bufio.NewScanner(r)
	count := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		matches := hexPattern.FindStringSubmatch(line)
		if matches == nil {
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

	return count
}

// Lookup looks up a MAC address and returns the vendor.
// Accepts various MAC formats: 00:1C:42:XX:XX:XX, 00-1C-42-XX-XX-XX, 001C42XXXXXX
func (s *Store) Lookup(mac string) (*Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.loaded {
		return nil, false
	}

	oui := normalizeMAC(mac)
	if len(oui) < 6 {
		return nil, false
	}

	// Take first 6 characters (3 bytes = OUI)
	oui = oui[:6]

	entry, ok := s.entries[oui]
	if !ok {
		return nil, false
	}

	return &entry, true
}

// LookupBulk looks up multiple MAC addresses.
func (s *Store) LookupBulk(macs []string) map[string]*Entry {
	result := make(map[string]*Entry)
	for _, mac := range macs {
		if entry, ok := s.Lookup(mac); ok {
			result[mac] = entry
		}
	}
	return result
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

// normalizeMAC removes separators and converts to uppercase.
func normalizeMAC(mac string) string {
	mac = strings.ToUpper(mac)
	mac = strings.ReplaceAll(mac, ":", "")
	mac = strings.ReplaceAll(mac, "-", "")
	mac = strings.ReplaceAll(mac, ".", "")
	return mac
}
