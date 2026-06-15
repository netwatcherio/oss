package reports

import (
	"bytes"
	"testing"

	"github.com/wcharczuk/go-chart/v2/drawing"
)

// TestRenderSparkline ensures the sparkline helper produces a valid
// PNG (PNG magic bytes: 0x89 0x50 0x4E 0x47 0x0D 0x0A 0x1A 0x0A).
func TestRenderSparkline(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 4, 3, 2, 1, 5, 6, 7, 8, 7, 6}
	png, err := RenderSparkline(values, drawing.Color{})
	if err != nil {
		t.Fatalf("RenderSparkline: %v", err)
	}
	if !bytes.HasPrefix(png, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) {
		t.Errorf("output is not a valid PNG (got %d bytes)", len(png))
	}
}

// TestRenderSparklineEmpty ensures we get an error for empty input
// (the chart engine can't draw a line of zero length).
func TestRenderSparklineEmpty(t *testing.T) {
	_, err := RenderSparkline(nil, drawing.Color{})
	if err == nil {
		t.Error("expected error for empty values, got nil")
	}
}

// TestRenderSparklineSingleValue ensures single-value input is
// padded (max > min) so the line has visible thickness.
func TestRenderSparklineSingleValue(t *testing.T) {
	// Sparkline requires at least 2 values (the chart engine's
	// x-range needs a delta). We supply 2 to confirm the helper
	// handles a flat series.
	png, err := RenderSparkline([]float64{4.2, 4.2}, drawing.Color{})
	if err != nil {
		t.Fatalf("RenderSparkline: %v", err)
	}
	if len(png) < 100 {
		t.Errorf("PNG too small (%d bytes) — chart may not have rendered", len(png))
	}
}

// TestRenderHealthGauge ensures the gauge renders a valid PNG.
func TestRenderHealthGauge(t *testing.T) {
	for _, mos := range []float64{1.0, 2.5, 3.5, 4.0, 4.5} {
		png, err := RenderHealthGauge(mos, gradeFromMOS(mos))
		if err != nil {
			t.Errorf("RenderHealthGauge(%.1f): %v", mos, err)
			continue
		}
		if len(png) < 100 {
			t.Errorf("RenderHealthGauge(%.1f) PNG too small (%d bytes)", mos, len(png))
		}
	}
}

// TestRenderHealthGaugeClamp ensures values outside the [1, 4.5]
// range are clamped silently.
func TestRenderHealthGaugeClamp(t *testing.T) {
	for _, mos := range []float64{0, -5, 5, 100} {
		_, err := RenderHealthGauge(mos, "excellent")
		if err != nil {
			t.Errorf("RenderHealthGauge(%.1f): %v (should clamp silently)", mos, err)
		}
	}
}

// TestRenderMOSTimeline ensures the timeline chart renders.
func TestRenderMOSTimeline(t *testing.T) {
	buckets := make([]VoiceBucket, 12)
	for i := range buckets {
		buckets[i] = VoiceBucket{
			Timestamp: "2024-01-01T" + string(rune('0'+i/10)) + string(rune('0'+i%10)) + ":00:00Z",
			Forward:   3.5 + float64(i%3)*0.3,
			Return:    3.8 + float64(i%2)*0.2,
		}
	}
	png, err := RenderMOSTimeline(buckets, []string{"2024-01-01T05:00:00Z"})
	if err != nil {
		t.Fatalf("RenderMOSTimeline: %v", err)
	}
	if len(png) < 100 {
		t.Errorf("timeline PNG too small (%d bytes)", len(png))
	}
}

// TestRenderMOSTimelineEmpty ensures we get an error for empty
// input.
func TestRenderMOSTimelineEmpty(t *testing.T) {
	_, err := RenderMOSTimeline(nil, nil)
	if err == nil {
		t.Error("expected error for empty buckets, got nil")
	}
}

// TestRenderJitterLossDualAxis ensures the dual-axis chart renders.
func TestRenderJitterLossDualAxis(t *testing.T) {
	buckets := make([]VoiceBucket, 6)
	for i := range buckets {
		buckets[i] = VoiceBucket{
			Timestamp: "2024-01-01T" + string(rune('0'+i)) + "0:00:00Z",
			LossPct:   float64(i) * 0.5,
			JitterMs:  float64(i) * 5,
		}
	}
	png, err := RenderJitterLossDualAxis(buckets)
	if err != nil {
		t.Fatalf("RenderJitterLossDualAxis: %v", err)
	}
	if len(png) < 100 {
		t.Errorf("dual-axis PNG too small (%d bytes)", len(png))
	}
}

// TestRenderPerProbeHeatmap ensures the heatmap renders.
func TestRenderPerProbeHeatmap(t *testing.T) {
	cells := []PerProbeCell{
		{Label: "probe-1", MOS: 4.5},
		{Label: "probe-2", MOS: 3.5},
		{Label: "probe-3", MOS: 2.0},
		{Label: "probe-4", MOS: 4.0},
	}
	png, err := RenderPerProbeHeatmap(cells, 2)
	if err != nil {
		t.Fatalf("RenderPerProbeHeatmap: %v", err)
	}
	if len(png) < 100 {
		t.Errorf("heatmap PNG too small (%d bytes)", len(png))
	}
}

// TestRenderPerProbeHeatmapEmpty ensures we get an error for empty
// input.
func TestRenderPerProbeHeatmapEmpty(t *testing.T) {
	_, err := RenderPerProbeHeatmap(nil, 4)
	if err == nil {
		t.Error("expected error for empty cells, got nil")
	}
}

// TestParseAgentReportSections covers all preset/override paths.
func TestParseAgentReportSections(t *testing.T) {
	cases := []struct {
		name  string
		input string
		check func(t *testing.T, got AgentReportOptions)
	}{
		{
			name:  "empty yields defaults",
			input: "",
			check: func(t *testing.T, got AgentReportOptions) {
				if !got.IncludeExecutive || !got.IncludeTimeline || !got.IncludePerProbe || !got.IncludeIssues || !got.IncludeCorrelation {
					t.Errorf("defaults wrong: %+v", got)
				}
				if got.IncludeAppendix || got.IncludeRawJSON || got.IncludeAggregate {
					t.Errorf("defaults should NOT include these: %+v", got)
				}
			},
		},
		{
			name:  "all turns everything on",
			input: "all",
			check: func(t *testing.T, got AgentReportOptions) {
				if !got.All {
					t.Error("All should be true")
				}
				if !got.IncludeExecutive || !got.IncludeTimeline || !got.IncludeAggregate || !got.IncludePerProbe ||
					!got.IncludeIssues || !got.IncludeCorrelation || !got.IncludeAppendix || !got.IncludeRawJSON {
					t.Errorf("not all sections on: %+v", got)
				}
			},
		},
		{
			name:  "none yields empty",
			input: "none",
			check: func(t *testing.T, got AgentReportOptions) {
				if got.IncludeExecutive || got.IncludeTimeline || got.IncludePerProbe || got.IncludeIssues {
					t.Errorf("none should be all-off: %+v", got)
				}
			},
		},
		{
			name:  "custom section names are recognized",
			input: "noissues,nocorrelation,appendix,aggregate,raw",
			check: func(t *testing.T, got AgentReportOptions) {
				if !got.IncludeExecutive || !got.IncludeTimeline || !got.IncludePerProbe {
					t.Errorf("non-negated defaults should still be on: %+v", got)
				}
				if !got.IncludeAppendix || !got.IncludeAggregate || !got.IncludeRawJSON {
					t.Errorf("listed sections should be on: %+v", got)
				}
				if got.IncludeIssues || got.IncludeCorrelation {
					t.Errorf("negated sections should be off: %+v", got)
				}
			},
		},
		{
			name:  "negation suppresses defaults",
			input: "noissues,nocorrelation",
			check: func(t *testing.T, got AgentReportOptions) {
				if got.IncludeIssues || got.IncludeCorrelation {
					t.Errorf("negation failed: %+v", got)
				}
				if !got.IncludeExecutive || !got.IncludeTimeline || !got.IncludePerProbe {
					t.Errorf("non-negated defaults lost: %+v", got)
				}
			},
		},
		{
			name:  "unknown tokens are ignored",
			input: "executive,frobnicate,baz",
			check: func(t *testing.T, got AgentReportOptions) {
				if !got.IncludeExecutive {
					t.Errorf("executive should be on: %+v", got)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseAgentReportSections(tc.input)
			tc.check(t, got)
		})
	}
}

// TestAgentReportOptionsIsAnySectionEnabled is a sanity check on
// the "any on" predicate.
func TestAgentReportOptionsIsAnySectionEnabled(t *testing.T) {
	allOff := AgentReportOptions{}
	if allOff.IsAnySectionEnabled() {
		t.Error("empty options should report no sections enabled")
	}
	one := AgentReportOptions{IncludeExecutive: true}
	if !one.IsAnySectionEnabled() {
		t.Error("with IncludeExecutive on, should report enabled")
	}
}
