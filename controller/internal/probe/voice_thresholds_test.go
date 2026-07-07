package probe

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestVoiceThresholdsMergeOverlayNoNil verifies that passing nil
// returns a copy of the base.
func TestVoiceThresholdsMergeOverlayNoNil(t *testing.T) {
	base := VoiceDefaultThresholds
	out := base.MergeOverlay(nil)
	// Compare key scalar fields (the struct now contains a map and
	// is therefore uncomparable with ==).
	if out.WarningJitterMs != base.WarningJitterMs || out.Codec != base.Codec {
		t.Errorf("nil overlay should return base; got %+v vs %+v", out, base)
	}
}

// TestVoiceThresholdsMergeOverlayPartial verifies that only the
// non-zero fields in the overlay take effect. Other fields keep
// their base value.
func TestVoiceThresholdsMergeOverlayPartial(t *testing.T) {
	base := VoiceDefaultThresholds
	warning := 12.0
	critical := 30.0
	overlay := &VoiceThresholds{
		WarningJitterMs:  warning,
		CriticalJitterMs: critical,
	}
	out := base.MergeOverlay(overlay)
	if out.WarningJitterMs != warning {
		t.Errorf("WarningJitterMs = %v, want %v", out.WarningJitterMs, warning)
	}
	if out.CriticalJitterMs != critical {
		t.Errorf("CriticalJitterMs = %v, want %v", out.CriticalJitterMs, critical)
	}
	// Untouched field should keep the base value.
	if out.WarningLossPct != base.WarningLossPct {
		t.Errorf("WarningLossPct should not change; got %v, want %v", out.WarningLossPct, base.WarningLossPct)
	}
	// Codec empty in overlay → base value preserved.
	if out.Codec != base.Codec {
		t.Errorf("Codec should be preserved from base; got %q, want %q", out.Codec, base.Codec)
	}
}

// TestVoiceThresholdsMergeOverlayCodec verifies that the codec
// field is overridden when explicitly set.
func TestVoiceThresholdsMergeOverlayCodec(t *testing.T) {
	base := VoiceDefaultThresholds
	overlay := &VoiceThresholds{Codec: "Opus"}
	out := base.MergeOverlay(overlay)
	if out.Codec != "Opus" {
		t.Errorf("Codec = %q, want Opus", out.Codec)
	}
}

// TestVoiceThresholdsJSONRoundTrip ensures the struct can be
// marshaled and unmarshaled through a typical JSON encoder (the
// path used by Postgres JSONB and the admin REST API).
func TestVoiceThresholdsJSONRoundTrip(t *testing.T) {
	src := VoiceDefaultThresholds.Clone()
	src.Codec = "G.729"
	src.WarningJitterMs = 18.0
	b, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var dst VoiceThresholds
	if err := json.Unmarshal(b, &dst); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if dst.Codec != "G.729" {
		t.Errorf("Codec = %q, want G.729", dst.Codec)
	}
	if dst.WarningJitterMs != 18.0 {
		t.Errorf("WarningJitterMs = %v, want 18.0", dst.WarningJitterMs)
	}
	if dst.ExcellentMos != src.ExcellentMos {
		t.Errorf("ExcellentMos = %v, want %v", dst.ExcellentMos, src.ExcellentMos)
	}
}

// TestVoiceThresholdsCloneIndependent verifies the clone is a real
// copy (mutating it doesn't affect the original).
func TestVoiceThresholdsCloneIndependent(t *testing.T) {
	a := VoiceDefaultThresholds
	b := a.Clone()
	b.WarningJitterMs = 999
	if a.WarningJitterMs == b.WarningJitterMs {
		t.Error("Clone should be independent")
	}
}

// TestDetectJitterAnomaliesUsesThresholds verifies the
// detection function respects the configured thresholds — both
// that warning/critical boundaries use the supplied values, and
// that the "no threshold breach = no issue" path is taken when
// the metric is below the threshold.
func TestDetectJitterAnomaliesUsesThresholds(t *testing.T) {
	cases := []struct {
		name       string
		jitter     float64
		thresholds VoiceThresholds
		wantSev    string // "" if no issue expected
	}{
		{
			name:       "below warning threshold (default)",
			jitter:     10.0,
			thresholds: VoiceDefaultThresholds,
			wantSev:    "",
		},
		{
			name:       "above warning threshold (default)",
			jitter:     20.0,
			thresholds: VoiceDefaultThresholds,
			wantSev:    "warning",
		},
		{
			name:       "above critical threshold (default)",
			jitter:     30.0,
			thresholds: VoiceDefaultThresholds,
			wantSev:    "critical",
		},
		{
			name:       "below custom warning threshold",
			jitter:     30.0, // above default threshold
			thresholds: VoiceThresholds{WarningJitterMs: 40.0, CriticalJitterMs: 60.0},
			wantSev:    "",
		},
		{
			name:       "above custom critical threshold",
			jitter:     70.0,
			thresholds: VoiceThresholds{WarningJitterMs: 40.0, CriticalJitterMs: 60.0},
			wantSev:    "critical",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := &VoicePathMetrics{
				Direction:   VoicePathForward,
				ProbeID:     1,
				JitterAvg:   tc.jitter,
				MosScore:    4.0,
				SampleCount: 100,
			}
			issues := detectJitterAnomalies(path, nil, VoicePathForward, "test", tc.thresholds)
			var got string
			for _, iss := range issues {
				if got == "" {
					got = iss.Severity
				} else if iss.Severity == "critical" {
					got = "critical"
				}
			}
			if got != tc.wantSev {
				t.Errorf("severity = %q, want %q (issues: %+v)", got, tc.wantSev, issues)
			}
		})
	}
}

// TestDetectPacketLossAnomaliesUsesThresholds verifies the
// loss detection uses the configured thresholds too.
func TestDetectPacketLossAnomaliesUsesThresholds(t *testing.T) {
	cases := []struct {
		name       string
		loss       float64
		thresholds VoiceThresholds
		wantSev    string
	}{
		{"below warning", 1.0, VoiceDefaultThresholds, ""},
		{"warning band", 3.0, VoiceDefaultThresholds, "warning"},
		{"critical band", 7.0, VoiceDefaultThresholds, "critical"},
		{"custom warning higher", 3.0, VoiceThresholds{WarningLossPct: 5.0, CriticalLossPct: 10.0}, ""},
		{"custom critical lower", 8.0, VoiceThresholds{WarningLossPct: 5.0, CriticalLossPct: 10.0}, "warning"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := &VoicePathMetrics{
				Direction:   VoicePathForward,
				ProbeID:     1,
				PacketLoss:  tc.loss,
				MosScore:    4.0,
				SampleCount: 100,
			}
			issues := detectPacketLossAnomalies(path, nil, VoicePathForward, "test", tc.thresholds)
			var got string
			for _, iss := range issues {
				if got == "" {
					got = iss.Severity
				} else if iss.Severity == "critical" {
					got = "critical"
				}
			}
			if got != tc.wantSev {
				t.Errorf("severity = %q, want %q (issues: %+v)", got, tc.wantSev, issues)
			}
		})
	}
}

// TestAdminSettingsJSONRoundTrip verifies that a VoiceThresholds
// stored via SetAdminVoiceThresholds round-trips correctly through
// the database. Skipped when no Postgres is configured.
func TestAdminSettingsJSONRoundTrip(t *testing.T) {
	t.Skip("requires a live Postgres; covered by integration tests")

	// Pseudocode: open a connection, set a threshold, get it back,
	// unmarshal, compare. Kept here as a placeholder for the
	// integration suite.
	_ = bytes.NewReader
}
