package probe

import (
	"encoding/json"
	"testing"
	"time"
)

// Bidirectional probes share one probe ID; forward and reverse rows differ only
// by the reporting agent_id. Aggregation must never blend the two directions
// into a single time bucket, or the panel's direction split misclassifies data.

func TestAggregateTrafficSimDataSeparatesDirections(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// Payloads use the RAW agent wire format: "MOS"/"RFactor" are capitalized
	// (unlike the aggregated output's "mos"/"rFactor") and must still be carried
	// through aggregation.
	mkRow := func(agentID uint, rtt float64, at time.Time) ProbeData {
		payload, _ := json.Marshal(map[string]any{
			"averageRTT":         rtt,
			"minRTT":             rtt - 1,
			"maxRTT":             rtt + 1,
			"totalPackets":       60,
			"lostPackets":        0,
			"MOS":                4.2,
			"RFactor":            85.0,
			"oneWayLatency":      rtt / 2,
			"networkHealthScore": 90.0,
		})
		return ProbeData{
			ProbeID:   42,
			AgentID:   agentID,
			Type:      TypeTrafficSim,
			CreatedAt: at,
			Payload:   payload,
		}
	}

	// Forward direction (agent 1, ~10ms) and reverse (agent 2, ~100ms),
	// interleaved inside the SAME one-minute bucket.
	rows := []ProbeData{
		mkRow(1, 10, base),
		mkRow(2, 100, base.Add(5*time.Second)),
		mkRow(1, 12, base.Add(10*time.Second)),
		mkRow(2, 110, base.Add(15*time.Second)),
	}

	out := aggregateTrafficSimData(rows, time.Minute, 0)
	if len(out) != 2 {
		t.Fatalf("got %d aggregated rows, want 2 (one per direction)", len(out))
	}

	seen := map[uint]bool{}
	for _, r := range out {
		var p TrafficSimPayload
		if err := json.Unmarshal(r.Payload, &p); err != nil {
			t.Fatalf("unmarshal aggregated payload: %v", err)
		}
		seen[r.AgentID] = true
		switch r.AgentID {
		case 1:
			if p.AverageRTT < 9 || p.AverageRTT > 13 {
				t.Errorf("forward bucket avgRTT = %.1f, want ~11 (directions blended?)", p.AverageRTT)
			}
		case 2:
			if p.AverageRTT < 99 || p.AverageRTT > 111 {
				t.Errorf("reverse bucket avgRTT = %.1f, want ~105 (directions blended?)", p.AverageRTT)
			}
		default:
			t.Errorf("aggregated row has unexpected AgentID %d", r.AgentID)
		}

		// VoIP metrics must survive aggregation, including the raw-format
		// capitalized "MOS"/"RFactor" keys.
		if p.MosScore < 4.1 || p.MosScore > 4.3 {
			t.Errorf("aggregated MOS = %.2f, want ~4.2 from agent E-model (not recomputed)", p.MosScore)
		}
		if p.RFactor < 84 || p.RFactor > 86 {
			t.Errorf("aggregated RFactor = %.1f, want ~85", p.RFactor)
		}
		if p.OneWayLatency == 0 {
			t.Error("oneWayLatency dropped by aggregation")
		}
		if p.NetworkHealthScore < 89 || p.NetworkHealthScore > 91 {
			t.Errorf("networkHealthScore = %.1f, want ~90", p.NetworkHealthScore)
		}
	}
	if !seen[1] || !seen[2] {
		t.Errorf("missing a direction in aggregated output: %v", seen)
	}
}

func TestAggregatePingDataSeparatesDirections(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	mkRow := func(agentID uint, rttMs int64, at time.Time) ProbeData {
		payload, _ := json.Marshal(pingAggInputPayload{
			AvgRtt:      rttMs * int64(time.Millisecond),
			MinRtt:      (rttMs - 1) * int64(time.Millisecond),
			MaxRtt:      (rttMs + 1) * int64(time.Millisecond),
			PacketsSent: 60,
			PacketsRecv: 60,
		})
		return ProbeData{
			ProbeID:   42,
			AgentID:   agentID,
			Type:      TypePing,
			CreatedAt: at,
			Payload:   payload,
		}
	}

	rows := []ProbeData{
		mkRow(1, 10, base),
		mkRow(2, 100, base.Add(5*time.Second)),
		mkRow(1, 12, base.Add(10*time.Second)),
		mkRow(2, 110, base.Add(15*time.Second)),
	}

	out := aggregatePingData(rows, time.Minute, 0)
	if len(out) != 2 {
		t.Fatalf("got %d aggregated rows, want 2 (one per direction)", len(out))
	}

	for _, r := range out {
		var p AggregatedPingPayload
		if err := json.Unmarshal(r.Payload, &p); err != nil {
			t.Fatalf("unmarshal aggregated payload: %v", err)
		}
		switch r.AgentID {
		case 1:
			if p.AvgLatency < 9 || p.AvgLatency > 13 {
				t.Errorf("forward bucket avgLatency = %.1f, want ~11 (directions blended?)", p.AvgLatency)
			}
		case 2:
			if p.AvgLatency < 99 || p.AvgLatency > 111 {
				t.Errorf("reverse bucket avgLatency = %.1f, want ~105 (directions blended?)", p.AvgLatency)
			}
		default:
			t.Errorf("aggregated row has unexpected AgentID %d", r.AgentID)
		}
	}
}
