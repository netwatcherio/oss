package oui

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	// IEEE OUI database URL
	IEEEOUIURL = "https://standards-oui.ieee.org/oui/oui.txt"

	// Default cache file path
	DefaultCachePath = "/tmp/oui.txt"
)

// Entry represents a single OUI entry.
type Entry struct {
	OUI         string `json:"oui"`    // e.g., "00:1C:42"
	Vendor      string `json:"vendor"` // e.g., "Parallels, Inc."
	Address     string `json:"address,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
}

// Store provides OUI lookup functionality.
type Store struct {
	mu        sync.RWMutex
	entries   map[string]Entry // Key: normalized OUI (uppercase, no separators)
	cachePath string
	loaded    bool
	loadedAt  time.Time
}

// NewStore creates a new OUI store.
func NewStore(cachePath string) *Store {
	if cachePath == "" {
		cachePath = DefaultCachePath
	}
	return &Store{
		entries:   make(map[string]Entry),
		cachePath: cachePath,
	}
}

// Load loads OUI data from cache or downloads from IEEE.
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Try cache first
	if s.loadFromCache() {
		return nil
	}

	// Download from IEEE
	return s.downloadAndParse()
}

// loadFromCache attempts to load from local cache file.
func (s *Store) loadFromCache() bool {
	info, err := os.Stat(s.cachePath)
	if err != nil {
		return false
	}

	// Cache expires after 30 days
	if time.Since(info.ModTime()) > 30*24*time.Hour {
		log.Info("OUI cache expired, will re-download")
		return false
	}

	file, err := os.Open(s.cachePath)
	if err != nil {
		return false
	}
	defer file.Close()

	count := s.parseOUIData(file)
	if count > 0 {
		s.loaded = true
		s.loadedAt = info.ModTime()
		log.Infof("Loaded %d OUI entries from cache", count)
		return true
	}
	return false
}

// downloadAndParse downloads OUI data from IEEE and parses it.
func (s *Store) downloadAndParse() error {
	log.Info("Downloading OUI database from IEEE...")

	resp, err := http.Get(IEEEOUIURL)
	if err != nil {
		return fmt.Errorf("download OUI data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download OUI data: status %d", resp.StatusCode)
	}

	// Save to cache file while parsing
	cacheFile, err := os.Create(s.cachePath)
	if err != nil {
		log.Warnf("Could not create cache file: %v", err)
		// Parse from response body directly
		count := s.parseOUIData(resp.Body)
		s.loaded = true
		s.loadedAt = time.Now()
		log.Infof("Loaded %d OUI entries (no cache)", count)
		return nil
	}
	defer cacheFile.Close()

	// Tee to both cache and parser
	reader := io.TeeReader(resp.Body, cacheFile)
	count := s.parseOUIData(reader)
	s.loaded = true
	s.loadedAt = time.Now()
	log.Infof("Loaded %d OUI entries from IEEE (cached)", count)
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
