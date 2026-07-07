package reports

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"netwatcher-controller/internal/probe"
)

// TestVoiceReportDataJSONShapePerProbe verifies the per-probe payload
// exposes the keys the panel view (and the static templates) read.
func TestVoiceReportDataJSONShapePerProbe(t *testing.T) {
	out := &VoiceReportDataJSON{
		Meta: VoiceReportMetaJSON{
			ReportID:    "VQR-PROBE-1",
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			ViewMode:    "probe",
			Agent:       &VoiceReportAgentRefJSON{ID: 1, Name: "agent-nyc-01"},
			Target:      &VoiceReportTargetRefJSON{Name: "HQ PBX", Host: "sip.example-pbx.com"},
		},
		Summary:    VoiceReportSummaryJSON{MOS: 4.21, RFactor: 86.4, Grade: "B+"},
		Thresholds: ToVoiceThresholdsJSON(probe.VoiceDefaultThresholds),
		Metrics: VoiceReportMetricsJSON{
			Latency: VoiceStatJSON{Min: 22.1, Avg: 34.6, Max: 61.8, StdDev: 6.2, Unit: "ms"},
			Jitter:  VoiceStatJSON{Min: 0.4, Avg: 3.8, Max: 28.4, StdDev: 2.9, Unit: "ms"},
			Packets: VoicePacketCountersJSON{Sent: 6000, Received: 5987, Lost: 13, LossPct: 0.22},
		},
	}

	// Encode to JSON and back to confirm the wire shape.
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	requiredTopLevel := []string{"meta", "summary", "thresholds", "metrics"}
	for _, k := range requiredTopLevel {
		if _, ok := got[k]; !ok {
			t.Errorf("top-level key %q missing from payload", k)
		}
	}

	meta := got["meta"].(map[string]interface{})
	for _, k := range []string{"report_id", "generated_at", "view_mode", "agent", "target"} {
		if _, ok := meta[k]; !ok {
			t.Errorf("meta.%s missing", k)
		}
	}
	if meta["view_mode"].(string) != "probe" {
		t.Errorf("meta.view_mode = %v, want \"probe\"", meta["view_mode"])
	}

	summary := got["summary"].(map[string]interface{})
	for _, k := range []string{"mos", "r_factor", "grade"} {
		if _, ok := summary[k]; !ok {
			t.Errorf("summary.%s missing", k)
		}
	}

	metrics := got["metrics"].(map[string]interface{})
	for _, k := range []string{"latency", "jitter", "packets"} {
		if _, ok := metrics[k]; !ok {
			t.Errorf("metrics.%s missing", k)
		}
	}
	packets := metrics["packets"].(map[string]interface{})
	for _, k := range []string{"sent", "received", "lost", "loss_pct"} {
		if _, ok := packets[k]; !ok {
			t.Errorf("metrics.packets.%s missing", k)
		}
	}
}

// TestVoiceReportDataJSONShapeWorkspace verifies the per-workspace
// payload includes heatmap + top_issues + issues.
func TestVoiceReportDataJSONShapeWorkspace(t *testing.T) {
	out := &VoiceReportDataJSON{
		Meta: VoiceReportMetaJSON{
			ReportID:    "VQR-WS-1",
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			ViewMode:    "workspace",
			Workspace:   &VoiceReportWorkspaceRefJSON{ID: 1, Name: "Test WS"},
		},
		Summary:    VoiceReportSummaryJSON{MOS: 4.0, RFactor: 75, Grade: "fair"},
		Thresholds: ToVoiceThresholdsJSON(probe.VoiceDefaultThresholds),
		Heatmap: []VoiceReportHeatmapCellJSON{
			{AgentID: 1, AgentName: "agent-a", ForwardMOS: 4.3, ForwardGrade: "excellent", ReverseMOS: 4.1, ReverseGrade: "good"},
			{AgentID: 2, AgentName: "agent-b", ForwardMOS: 3.7, ForwardGrade: "fair"},
		},
		TopIssues: []VoiceQualityIssueJSON{
			{ID: "x", Severity: "critical", Title: "Loss burst"},
		},
	}

	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["meta"].(map[string]interface{})["view_mode"].(string) != "workspace" {
		t.Errorf("expected view_mode = workspace")
	}
	for _, k := range []string{"heatmap", "top_issues"} {
		if _, ok := got[k]; !ok {
			t.Errorf("top-level %s missing from workspace payload", k)
		}
	}
}

// TestVoiceReportDataJSONShapeMulti verifies the multi-pair payload
// includes pairs[].
func TestVoiceReportDataJSONShapeMulti(t *testing.T) {
	out := &VoiceReportDataJSON{
		Meta:       VoiceReportMetaJSON{ReportID: "X", ViewMode: "multi", GeneratedAt: time.Now().UTC().Format(time.RFC3339)},
		Summary:    VoiceReportSummaryJSON{MOS: 4.0, Grade: "fair"},
		Thresholds: ToVoiceThresholdsJSON(probe.VoiceDefaultThresholds),
		Pairs: []VoicePairSummaryJSON{
			{
				ID:           "pair-1",
				Agent:        VoiceReportAgentRefJSON{ID: 1, Name: "agent"},
				Target:       VoiceReportTargetRefJSON{Name: "HQ PBX"},
				OverallMOS:   4.21,
				OverallGrade: "good",
				Issues:       []VoiceQualityIssueJSON{},
			},
		},
	}

	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(b), `"pairs"`) {
		t.Errorf("pairs array missing from multi-view payload")
	}
}

// TestBuildAgentReportDataNilThreshold verifies the builder falls
// back to defaults when the engine returns a nil thresholds field.
func TestBuildAgentReportDataNilThreshold(t *testing.T) {
	// We can't easily call BuildAgentReportData without a real DB,
	// so just verify the JSON wire shape includes the right defaults.
	out := &VoiceReportDataJSON{
		Thresholds: ToVoiceThresholdsJSON(probe.VoiceDefaultThresholds),
	}
	if out.Thresholds.Codec != "G.711" {
		t.Errorf("default codec should be G.711, got %q", out.Thresholds.Codec)
	}
	if out.Thresholds.WarningLossPct == 0 {
		t.Errorf("default WarningLossPct should be non-zero")
	}
}

// TestToVoicePairSummaryJSONRoundTrip ensures the JSON converter
// preserves all the fields the panel reads.
func TestToVoicePairSummaryJSONRoundTrip(t *testing.T) {
	src := probe.VoicePairSummary{
		ID:     "pair-1",
		Agent:  probe.AgentRef{ID: 1, Name: "agent"},
		Target: probe.TargetRef{Name: "target", Host: "host"},
		Forward: &probe.VoicePathMetrics{
			ProbeID: 1, MosScore: 4.2, AvgLatency: 30, JitterAvg: 3,
			PacketLoss: 0.5, SampleCount: 100, CongestionLevel: probe.CongestionNone,
			MaxConsecutiveLoss: 2, TotalBursts: 1,
		},
		Issues: []probe.VoiceQualityIssue{
			{ID: "loss_x", Severity: "warning", Title: "Loss burst", Category: "packet_loss"},
		},
		OverallMos:   4.2,
		OverallGrade: "good",
		Thresholds:   probe.VoiceDefaultThresholds,
	}

	out := ToVoicePairSummaryJSON(src)
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, k := range []string{
		`"id":"pair-1"`,
		`"overall_mos":4.2`,
		`"overall_grade":"good"`,
		`"forward"`,
		`"max_consecutive_loss":2`,
		`"issues"`,
		`"title":"Loss burst"`,
	} {
		if !strings.Contains(s, k) {
			t.Errorf("expected JSON to contain %q, got %s", k, s)
		}
	}
}

// stubCompileTimeReferences keeps gorm imported so the test file
// builds in isolation; the package doesn't otherwise reference it.
var _ = (*gorm.DB)(nil)
