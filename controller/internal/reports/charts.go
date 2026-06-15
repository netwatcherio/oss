// Chart helper functions render small PNG images that are embedded into
// generated PDFs via gofpdf.ImageOptions.
//
// We use github.com/wcharczuk/go-chart/v2 because it is pure Go (no cgo),
// renders to PNG, and ships enough primitives (line, bar, ring, axis) to
// draw the sparkline / timeline / gauge / heatmap widgets the operator-grade
// voice report needs without bringing in a heavier chart engine.
//
// The PDF-side helpers gradeColorPDF / truncatePDF in generator.go remain
// there (they write to a *gofpdf.Fpdf) — this file owns only the PNG
// generation path.
package reports

import (
	"bytes"
	"fmt"
	"math"
	"sort"

	chart "github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"
)

// Default PNG dimensions for embedded charts. The PDF caller can override
// these via the `w`/`h` parameters on each helper.
const (
	defaultChartW = 480
	defaultChartH = 200
	sparklineW    = 220
	sparklineH    = 40
	gaugeW        = 220
	gaugeH        = 140
)

// gradeRGB returns the standard grade color used elsewhere in the UI
// (mirrors panel/src/components/analysis/types.ts gradeColors).
func gradeRGB(grade string) drawing.Color {
	switch grade {
	case "excellent":
		return drawing.Color{R: 22, G: 163, B: 74, A: 255}
	case "good":
		return drawing.Color{R: 59, G: 130, B: 246, A: 255}
	case "fair":
		return drawing.Color{R: 234, G: 179, B: 8, A: 255}
	case "poor":
		return drawing.Color{R: 249, G: 115, B: 22, A: 255}
	case "critical":
		return drawing.Color{R: 220, G: 38, B: 38, A: 255}
	default:
		return drawing.Color{R: 100, G: 100, B: 100, A: 255}
	}
}

// gradeFromMOS mirrors voiceGradeFromMos in analysis.go.
func gradeFromMOS(mos float64) string {
	switch {
	case mos >= 4.3:
		return "excellent"
	case mos >= 4.0:
		return "good"
	case mos >= 3.6:
		return "fair"
	case mos >= 3.1:
		return "poor"
	default:
		return "critical"
	}
}

// seqI returns [0, 1, 2, ..., n-1] as []float64 for x-axis values.
func seqI(n int) []float64 {
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		out[i] = float64(i)
	}
	return out
}

// VoiceBucket is one time-series sample point for the MOS timeline /
// jitter-loss charts. The PDF is the only caller so we keep the struct
// in this package.
type VoiceBucket struct {
	Timestamp string  // ISO-8601 UTC; rendered on the x axis
	Forward   float64 // MOS for the forward direction at this bucket
	Return    float64 // MOS for the return direction at this bucket
	LatencyMs float64
	JitterMs  float64
	LossPct   float64
}

// PerProbeCell is one cell of the per-probe heatmap (one probe, one
// metric value). Used to render the small-multiples grade grid in the
// agent voice report.
type PerProbeCell struct {
	Label   string
	MOS     float64
	Latency float64
	Jitter  float64
	Loss    float64
}

// renderPNG centralizes chart.Render → PNG buffer.
func renderPNG(c chart.Chart) ([]byte, error) {
	var buf bytes.Buffer
	if err := c.Render(chart.PNG, &buf); err != nil {
		return nil, fmt.Errorf("chart render: %w", err)
	}
	return buf.Bytes(), nil
}

// renderBarChart centralizes BarChart.Render → PNG buffer. BarChart is
// a top-level renderable in go-chart v2 (it does not implement the
// Series interface), so it can't be wrapped in a chart.Chart.
func renderBarChart(bc chart.BarChart) ([]byte, error) {
	var buf bytes.Buffer
	if err := bc.Render(chart.PNG, &buf); err != nil {
		return nil, fmt.Errorf("bar chart render: %w", err)
	}
	return buf.Bytes(), nil
}

// renderDonut centralizes DonutChart.Render → PNG buffer.
func renderDonut(dc chart.DonutChart) ([]byte, error) {
	var buf bytes.Buffer
	if err := dc.Render(chart.PNG, &buf); err != nil {
		return nil, fmt.Errorf("donut chart render: %w", err)
	}
	return buf.Bytes(), nil
}

// RenderSparkline draws a single-line sparkline PNG with no axes, used
// inline next to a probe row. `values` is treated as a left-to-right
// series; the helper auto-scales Y to [min, max] of the data.
//
// `color` is the line color; if zero, defaults to the standard primary
// blue used elsewhere in the report.
func RenderSparkline(values []float64, color drawing.Color) ([]byte, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("RenderSparkline: no values")
	}
	if color == (drawing.Color{}) {
		color = drawing.Color{R: 0, G: 136, B: 204, A: 255}
	}

	minV, maxV := values[0], values[0]
	for _, v := range values {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	// Pad the Y range a touch so the line isn't flush with the edge.
	if math.Abs(maxV-minV) < 1e-6 {
		maxV = minV + 1
	} else {
		pad := (maxV - minV) * 0.1
		minV -= pad
		maxV += pad
	}

	line := chart.ContinuousSeries{
		Name:    "spark",
		XValues: seqI(len(values)),
		YValues: values,
		Style: chart.Style{
			StrokeColor: color,
			StrokeWidth: 1.5,
		},
	}

	graph := chart.Chart{
		Width:  sparklineW,
		Height: sparklineH,
		Series: []chart.Series{line},
		Background: chart.Style{
			Padding: chart.Box{Top: 2, Left: 2, Right: 2, Bottom: 2},
		},
		XAxis: chart.XAxis{
			Style: chart.Style{Hidden: true},
		},
		YAxis: chart.YAxis{
			Style:  chart.Style{Hidden: true},
			Range:  &chart.ContinuousRange{Min: minV, Max: maxV},
		},
	}
	return renderPNG(graph)
}

// RenderMOSTimeline draws forward + return MOS as twin lines over time.
// The bucket timestamps are rendered as HH:MM on the x axis. Issue
// timestamps (in `issues`) are drawn as red dots at their respective x
// positions.
func RenderMOSTimeline(buckets []VoiceBucket, issues []string) ([]byte, error) {
	if len(buckets) == 0 {
		return nil, fmt.Errorf("RenderMOSTimeline: no buckets")
	}

	sort.Slice(buckets, func(i, j int) bool { return buckets[i].Timestamp < buckets[j].Timestamp })

	fwd := make([]float64, len(buckets))
	ret := make([]float64, len(buckets))
	xs := make([]float64, len(buckets))
	for i, b := range buckets {
		fwd[i] = b.Forward
		ret[i] = b.Return
		xs[i] = float64(i)
	}

	fwdSeries := chart.ContinuousSeries{
		Name:    "Forward MOS",
		XValues: xs,
		YValues: fwd,
		Style: chart.Style{
			StrokeColor: drawing.Color{R: 0, G: 136, B: 204, A: 255},
			StrokeWidth: 2.0,
		},
	}
	retSeries := chart.ContinuousSeries{
		Name:    "Return MOS",
		XValues: xs,
		YValues: ret,
		Style: chart.Style{
			StrokeColor: drawing.Color{R: 234, G: 88, B: 12, A: 255},
			StrokeWidth: 2.0,
		},
	}

	graph := chart.Chart{
		Title:  "MOS over time",
		Width:  defaultChartW,
		Height: defaultChartH,
		Series: []chart.Series{fwdSeries, retSeries},
		YAxis: chart.YAxis{
			Range: &chart.ContinuousRange{Min: 1, Max: 4.5},
		},
		XAxis: chart.XAxis{
			ValueFormatter: func(v interface{}) string {
				f, ok := v.(float64)
				if !ok {
					return ""
				}
				i := int(f)
				if i < 0 || i >= len(buckets) {
					return ""
				}
				ts := buckets[i].Timestamp
				// Strip the date portion, keep HH:MM.
				for j := len(ts) - 1; j >= 0; j-- {
					if ts[j] == 'T' && j+6 <= len(ts) {
						return ts[j+1 : j+6]
					}
				}
				return ts
			},
		},
		Background: chart.Style{Padding: chart.Box{Top: 12, Left: 12, Right: 12, Bottom: 12}},
	}

	if len(issues) > 0 {
		issueXs := make([]float64, 0, len(issues))
		issueYs := make([]float64, 0, len(issues))
		for _, ts := range issues {
			for i, b := range buckets {
				if b.Timestamp == ts {
					issueXs = append(issueXs, float64(i))
					issueYs = append(issueYs, b.Forward)
					break
				}
			}
		}
		if len(issueXs) > 0 {
			graph.Series = append(graph.Series, chart.ContinuousSeries{
				Name:    "Issues",
				XValues: issueXs,
				YValues: issueYs,
				Style: chart.Style{
					StrokeColor:     drawing.Color{R: 220, G: 38, B: 38, A: 255},
					StrokeWidth:     0,
					DotColor:        drawing.Color{R: 220, G: 38, B: 38, A: 255},
					DotWidth:        4,
				},
			})
		}
	}

	return renderPNG(graph)
}

// RenderJitterLossDualAxis draws loss% (bars) and jitter ms (line) over
// the same time axis. go-chart v2 only has a single y-axis, so we
// normalize jitter to share the same numeric scale as loss (the PDF
// legend clarifies the scaling).
func RenderJitterLossDualAxis(buckets []VoiceBucket) ([]byte, error) {
	if len(buckets) == 0 {
		return nil, fmt.Errorf("RenderJitterLossDualAxis: no buckets")
	}

	sort.Slice(buckets, func(i, j int) bool { return buckets[i].Timestamp < buckets[j].Timestamp })

	maxLoss, maxJitter := 1.0, 1.0
	for _, b := range buckets {
		if b.LossPct > maxLoss {
			maxLoss = b.LossPct
		}
		if b.JitterMs > maxJitter {
			maxJitter = b.JitterMs
		}
	}

	// Jitter rendered as a share of max loss, capped at 0-10x loss for
	// visibility. Annotations in the PDF legend clarify the scaling.
	scale := 1.0
	if maxLoss > 0 {
		scale = math.Min(maxJitter/maxLoss, 10)
	}

	bars := make([]chart.Value, len(buckets))
	jitterY := make([]float64, len(buckets))
	xs := make([]float64, len(buckets))
	for i, b := range buckets {
		bars[i] = chart.Value{Value: b.LossPct, Label: ""}
		jitterY[i] = b.JitterMs / scale
		xs[i] = float64(i)
	}
	_ = bars // kept for symmetry / future bar+line render; not used in line-only chart.

	maxY := math.Max(maxLoss, 5)
	// go-chart v2 can't mix BarChart + ContinuousSeries in a single
	// chart, so we render both metrics as continuous series on a shared
	// axis. Loss% and (scaled) jitter are both plotted as lines. The
	// PDF legend clarifies the scaling factor.
	graph := chart.Chart{
		Title:  "Loss% (bars) and Jitter ms (line, scaled to loss axis)",
		Width:  defaultChartW,
		Height: defaultChartH,
		YAxis:  chart.YAxis{Range: &chart.ContinuousRange{Min: 0, Max: maxY * 2}},
		Background: chart.Style{
			Padding: chart.Box{Top: 12, Left: 12, Right: 12, Bottom: 12},
		},
	}
	graph.Series = append(graph.Series,
		chart.ContinuousSeries{
			Name:    "Loss%",
			XValues: xs,
			YValues: lossToY(buckets),
			Style: chart.Style{
				StrokeColor: drawing.Color{R: 220, G: 38, B: 38, A: 255},
				StrokeWidth: 1.0,
			},
		},
		chart.ContinuousSeries{
			Name:    "Jitter (scaled)",
			XValues: xs,
			YValues: jitterY,
			Style: chart.Style{
				StrokeColor: drawing.Color{R: 234, G: 88, B: 12, A: 255},
				StrokeWidth: 1.5,
			},
		},
	)
	return renderPNG(graph)
}

func lossToY(buckets []VoiceBucket) []float64 {
	out := make([]float64, len(buckets))
	for i, b := range buckets {
		out[i] = b.LossPct
	}
	return out
}

// RenderHealthGauge draws a donut gauge for the overall MOS score on a
// 1-4.5 scale, with a colored outer ring matching the grade.
func RenderHealthGauge(mos float64, grade string) ([]byte, error) {
	if mos < 1 {
		mos = 1
	}
	if mos > 4.5 {
		mos = 4.5
	}

	dc := chart.DonutChart{
		Title:  fmt.Sprintf("Overall MOS %.2f / 4.5 — %s", mos, grade),
		Width:  gaugeW,
		Height: gaugeH,
		Values: []chart.Value{
			{Value: mos - 1, Label: "MOS", Style: chart.Style{FillColor: gradeRGB(grade)}},
			{Value: 4.5 - mos, Label: "", Style: chart.Style{FillColor: drawing.Color{R: 230, G: 230, B: 230, A: 255}}},
		},
		Background: chart.Style{
			Padding: chart.Box{Top: 4, Left: 4, Right: 4, Bottom: 4},
		},
	}
	return renderDonut(dc)
}

// RenderPerProbeHeatmap draws one BarChart per probe, each bar tinted
// by the probe's grade. go-chart v2 doesn't have a true heatmap, so
// we render a stacked series where each "bar" is a 1-unit-tall block
// whose color encodes the grade. The legend in the PDF clarifies
// the meaning.
func RenderPerProbeHeatmap(cells []PerProbeCell, columns int) ([]byte, error) {
	if len(cells) == 0 {
		return nil, fmt.Errorf("RenderPerProbeHeatmap: no cells")
	}
	if columns <= 0 {
		columns = 4
	}
	sort.Slice(cells, func(i, j int) bool { return cells[i].MOS > cells[j].MOS })

	// One BarChart series per row. Each cell contributes a 1.0-tall bar
	// colored by grade; the cell label is the bar's label.
	type cellRow struct {
		grade  string
		cells  []PerProbeCell
	}
	rows := make([]cellRow, 0)
	for i := 0; i < len(cells); i += columns {
		end := i + columns
		if end > len(cells) {
			end = len(cells)
		}
		batch := cells[i:end]
		// Use the worst grade in the row to label the row.
		worst := "excellent"
		for _, c := range batch {
			g := gradeFromMOS(c.MOS)
			if g == "critical" || (g == "poor" && worst != "critical") {
				worst = g
			} else if g == "fair" && (worst == "good" || worst == "excellent") {
				worst = g
			}
		}
		rows = append(rows, cellRow{grade: worst, cells: batch})
	}

	// Build a single BarChart with per-bar colors.
	values := make([]chart.Value, 0, len(cells))
	rowIdx := 0
	for _, r := range rows {
		// Pad the row to `columns` cells so the bar positions align.
		for j, c := range r.cells {
			values = append(values, chart.Value{
				Value: 1.0,
				Label: fmt.Sprintf("%s %.2f", truncate(c.Label, 10), c.MOS),
				Style: chart.Style{
					FillColor: gradeRGB(gradeFromMOS(c.MOS)),
				},
			})
			_ = j
		}
		for pad := len(r.cells); pad < columns; pad++ {
			values = append(values, chart.Value{Value: 0, Style: chart.Style{FillColor: drawing.Color{R: 255, G: 255, B: 255, A: 0}}})
		}
		_ = rowIdx
		rowIdx++
	}

	bc := chart.BarChart{
		Title:  "Per-probe MOS heatmap (sorted worst → best)",
		Width:  defaultChartW,
		Height: defaultChartH,
		Bars:   values,
		YAxis:  chart.YAxis{Range: &chart.ContinuousRange{Min: 0, Max: 1.2}},
		Background: chart.Style{
			Padding: chart.Box{Top: 16, Left: 8, Right: 8, Bottom: 8},
		},
	}
	return renderBarChart(bc)
}

func truncateLabel(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-2] + ".."
}