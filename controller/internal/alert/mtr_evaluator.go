package alert

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

// MtrHost represents a hop host in MTR data
type MtrHost struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
}

// MtrHop represents a single hop in MTR data
type MtrHop struct {
	TTL     int       `json:"ttl"`
	Hosts   []MtrHost `json:"hosts"`
	LossPct string    `json:"loss_pct"`
	Avg     string    `json:"avg"`
	Worst   string    `json:"worst"`
	Best    string    `json:"best"`
	Sent    int       `json:"sent"`
	Recv    int       `json:"recv"`
}

// MtrReport represents the report structure in MTR data
type MtrReport struct {
	Hops []MtrHop `json:"hops"`
}

// MtrPayload represents the full MTR payload structure
type MtrPayload struct {
	StartTimestamp string `json:"start_timestamp"`
	StopTimestamp  string `json:"stop_timestamp"`
	Report         struct {
		Info struct {
			Target struct {
				IP       string `json:"ip"`
				Hostname string `json:"hostname"`
			} `json:"target"`
		} `json:"info"`
		Hops []MtrHop `json:"hops"`
	} `json:"report"`
}

// MtrMetrics holds extracted metrics from MTR data
type MtrMetrics struct {
	EndHopLoss    float64 // Packet loss at final destination
	EndHopLatency float64 // Avg latency at final destination
	WorstHopLoss  float64 // Worst packet loss on any hop
	WorstHopIndex int     // Index of hop with worst loss
	HopCount      int     // Total number of hops
}

// ParseMtrPayload parses raw MTR JSON payload
func ParseMtrPayload(payloadJSON []byte) (*MtrPayload, error) {
	var mtr MtrPayload
	if err := json.Unmarshal(payloadJSON, &mtr); err != nil {
		return nil, err
	}
	return &mtr, nil
}

// ExtractMtrMetrics extracts alertable metrics from MTR data
func ExtractMtrMetrics(mtr *MtrPayload) *MtrMetrics {
	hops := mtr.Report.Hops
	if len(hops) == 0 {
		return &MtrMetrics{}
	}

	metrics := &MtrMetrics{
		HopCount: len(hops),
	}

	// Extract end hop (final destination) metrics
	lastHop := hops[len(hops)-1]
	metrics.EndHopLoss, _ = strconv.ParseFloat(lastHop.LossPct, 64)
	metrics.EndHopLatency, _ = strconv.ParseFloat(lastHop.Avg, 64)

	// Find worst hop loss (excluding destination - that's end_hop_loss)
	for i, hop := range hops {
		loss, _ := strconv.ParseFloat(hop.LossPct, 64)
		if loss > metrics.WorstHopLoss {
			metrics.WorstHopLoss = loss
			metrics.WorstHopIndex = i
		}
	}

	return metrics
}

// GetRouteFingerprint generates a deterministic hash of the route path
// This is used to detect route changes between MTR runs
func GetRouteFingerprint(mtr *MtrPayload) string {
	var ips []string

	for _, hop := range mtr.Report.Hops {
		if len(hop.Hosts) > 0 {
			// Use primary host IP for fingerprint
			// Handle * (timeout) hops by using placeholder
			ip := hop.Hosts[0].IP
			if ip == "" || ip == "*" {
				ip = "*"
			}
			ips = append(ips, ip)
		} else {
			ips = append(ips, "*")
		}
	}

	// Sort is NOT applied - order matters for routes
	// Create deterministic hash
	routePath := strings.Join(ips, "->")
	hash := sha256.Sum256([]byte(routePath))
	return hex.EncodeToString(hash[:16]) // Use first 16 bytes for shorter fingerprint
}

// GetRoutePathString returns human-readable route path
func GetRoutePathString(mtr *MtrPayload) string {
	var ips []string
	for _, hop := range mtr.Report.Hops {
		if len(hop.Hosts) > 0 {
			ip := hop.Hosts[0].IP
			if ip == "" {
				ip = "*"
			}
			ips = append(ips, ip)
		} else {
			ips = append(ips, "*")
		}
	}
	return strings.Join(ips, " -> ")
}

// CompareRoutes checks if two route fingerprints are different
func CompareRoutes(baseline, current string) bool {
	return baseline != current
}

// GetMtrMetricValue extracts a specific metric value from MTR data
func GetMtrMetricValue(metrics *MtrMetrics, metric Metric) *float64 {
	switch metric {
	case MetricEndHopLoss:
		return &metrics.EndHopLoss
	case MetricEndHopLatency:
		return &metrics.EndHopLatency
	case MetricWorstHopLoss:
		return &metrics.WorstHopLoss
	default:
		return nil
	}
}

// Helper to sort strings for consistent ordering (used in some contexts)
func sortedCopy(s []string) []string {
	c := make([]string, len(s))
	copy(c, s)
	sort.Strings(c)
	return c
}
