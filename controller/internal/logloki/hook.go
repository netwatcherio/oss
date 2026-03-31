package logloki

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

type Hook struct {
	url      string
	labels   map[string]string
	host     string
	app      string
	interval time.Duration
	buf      []*logEntry
	maxBuf   int
	mu       sync.Mutex
	client   *http.Client
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

type logEntry struct {
	Timestamp time.Time      `json:"ts"`
	Level     string         `json:"level"`
	Message   string         `json:"msg"`
	Fields    map[string]any `json:"fields,omitempty"`
}

type lokiStream struct {
	Labels  string      `json:"labels"`
	Entries []lokiEntry `json:"entries"`
}

type lokiEntry struct {
	Timestamp string `json:"ts"`
	Line      string `json:"line"`
}

func NewHook(url, app string) *Hook {
	h := &Hook{
		url:      strings.TrimSuffix(url, "/"),
		app:      app,
		labels:   make(map[string]string),
		interval: 3 * time.Second,
		maxBuf:   500,
		buf:      make([]*logEntry, 0, 100),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		stopCh: make(chan struct{}),
	}

	if hostname, err := os.Hostname(); err == nil {
		h.host = hostname
	}

	return h
}

func (h *Hook) SetLabel(key, value string) {
	h.labels[key] = value
}

func (h *Hook) SetInterval(d time.Duration) {
	h.interval = d
}

func (h *Hook) Start() {
	h.wg.Add(1)
	go h.run()
}

func (h *Hook) Stop() {
	close(h.stopCh)
	h.flush()
	h.wg.Wait()
}

func (h *Hook) run() {
	defer h.wg.Done()
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.flush()
		case <-h.stopCh:
			return
		}
	}
}

func (h *Hook) Fire(entry *log.Entry) error {
	if h.url == "" {
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	fields := make(map[string]any)
	for k, v := range entry.Data {
		fields[k] = v
	}

	h.buf = append(h.buf, &logEntry{
		Timestamp: entry.Time.UTC(),
		Level:     strings.ToLower(entry.Level.String()),
		Message:   entry.Message,
		Fields:    fields,
	})

	if len(h.buf) >= h.maxBuf {
		h.flushLocked()
	}

	return nil
}

func (h *Hook) flush() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.flushLocked()
}

func (h *Hook) flushLocked() {
	if len(h.buf) == 0 {
		return
	}

	entries := h.buf
	h.buf = make([]*logEntry, 0, 100)

	go func(entries []*logEntry) {
		if err := h.send(entries); err != nil {
			log.Warnf("loki: flush failed: %v", err)
		}
	}(entries)
}

func (h *Hook) send(entries []*logEntry) error {
	if len(entries) == 0 {
		return nil
	}

	labelStr := h.buildLabels()
	streams := make([]lokiStream, 0, 1)

	lokiEntries := make([]lokiEntry, 0, len(entries))
	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			continue
		}
		lokiEntries = append(lokiEntries, lokiEntry{
			Timestamp: e.Timestamp.Format(time.RFC3339Nano),
			Line:      string(line),
		})
	}

	if len(lokiEntries) == 0 {
		return nil
	}

	streams = append(streams, lokiStream{
		Labels:  labelStr,
		Entries: lokiEntries,
	})

	payload, err := json.Marshal(map[string]any{"streams": streams})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, h.url+"/loki/api/v1/push", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("loki returned %d", resp.StatusCode)
	}

	return nil
}

func (h *Hook) buildLabels() string {
	labels := make([]string, 0, len(h.labels)+2)
	labels = append(labels, fmt.Sprintf(`app="%s"`, h.app))
	if h.host != "" {
		labels = append(labels, fmt.Sprintf(`host="%s"`, h.host))
	}
	for k, v := range h.labels {
		labels = append(labels, fmt.Sprintf(`%s="%s"`, k, v))
	}
	return "{" + strings.Join(labels, ",") + "}"
}

func (h *Hook) Levels() []log.Level {
	return []log.Level{
		log.PanicLevel,
		log.FatalLevel,
		log.ErrorLevel,
		log.WarnLevel,
		log.InfoLevel,
	}
}
