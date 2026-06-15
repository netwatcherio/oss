package reports

import (
	"bytes"
	"fmt"

	"github.com/jung-kurt/gofpdf"
)

// PDF page dimensions we work with. A4 portrait at 15mm margins
// leaves 180mm horizontal × 267mm vertical for content.
const (
	pdfPageW  = 210.0
	pdfPageH  = 297.0
	pdfMargin = 15.0
	// usableH is the y-coordinate below which we should call AddPage
	// before rendering more content. We keep ~25mm footer clearance
	// for the page number / generation timestamp.
	pdfUsableH = 270.0
)

// ensureRoom pushes a new page if the next block of `needed` mm
// wouldn't fit. Returns the Y position after any auto-page-break so
// callers can chain `pdf.SetY(room)` and start drawing. Use this
// instead of sprinkling `pdf.AddPage()` calls — it makes layout
// decisions local and testable.
func ensureRoom(pdf *gofpdf.Fpdf, needed float64) float64 {
	if pdf.GetY()+needed > pdfUsableH {
		pdf.AddPage()
		return pdfMargin
	}
	return pdf.GetY()
}

// sectionHeader writes a consistent, page-break-aware section title
// and reserves space. Returns the Y position after the header so
// callers can continue drawing immediately.
func sectionHeader(pdf *gofpdf.Fpdf, title string) float64 {
	pdf.Ln(4)
	y := ensureRoom(pdf, 12)
	pdf.SetY(y)
	pdf.SetFont("Arial", "B", 14)
	pdf.SetTextColor(26, 54, 93)
	pdf.Cell(0, 8, title)
	pdf.Ln(8)
	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(50, 50, 50)
	return pdf.GetY()
}

// chipText writes a small colored "chip" — a colored block followed
// by the label text in the chip's contrast color. Used for severity
// badges and metric labels in the new layout. The block is
// `width` mm wide and 6mm tall.
func chipText(pdf *gofpdf.Fpdf, label, text string, r, g, b int) {
	pdf.SetFont("Arial", "B", 8)
	pdf.SetFillColor(r, g, b)
	pdf.SetTextColor(255, 255, 255)
	pdf.CellFormat(0, 6, " "+label+" ", "", 0, "L", true, 0, "")
	pdf.SetTextColor(50, 50, 50)
	pdf.SetFont("Arial", "", 10)
	pdf.CellFormat(0, 6, "  "+text, "", 0, "L", false, 0, "")
	pdf.Ln(6)
	pdf.SetFont("Arial", "", 10)
}

// metricRow writes a `label: value` line, like the old cover-page
// executive metric format, but tab-aligned and with bold label.
func metricRow(pdf *gofpdf.Fpdf, label, value string) {
	pdf.SetFont("Arial", "B", 10)
	pdf.Cell(60, 6, label+":")
	pdf.SetFont("Arial", "", 10)
	pdf.Cell(0, 6, value)
	pdf.Ln(6)
}

// pageFooter is the optional page footer (page number + generator
// tag). Wire up with `pdf.AliasNbPages("")` and call from
// pdf.SetFooterFunc on the generator.
func pageFooter(pdf *gofpdf.Fpdf) {
	pdf.SetY(-12)
	pdf.SetFont("Arial", "I", 8)
	pdf.SetTextColor(120, 120, 120)
	pdf.CellFormat(0, 8, fmt.Sprintf("NetWatcher Voice Quality Report  •  Page %d/{nb}", pdf.PageNo()), "", 0, "C", false, 0, "")
}

// imageBytesFromPNG registers a PNG byte slice with gofpdf and
// returns the registered image-info pointer for the caller to
// position via pdf.ImageOptions. We keep this in a small helper so
// the render code stays declarative (line, "x, y, w, h, ...").
//
// gofpdf's PNG registration returns nil if the PNG is invalid; the
// render callers log+skip on error so the report still generates
// when a chart fails to render.
func imageBytesFromPNG(pdf *gofpdf.Fpdf, name string, png []byte) (*gofpdf.ImageInfoType, error) {
	imgOpts := gofpdf.ImageOptions{ImageType: "PNG", ReadDpi: false}
	info := pdf.RegisterImageOptionsReader(name, imgOpts, bytes.NewReader(png))
	if info == nil {
		return nil, errImageNotRegistered
	}
	return info, nil
}

// errImageNotRegistered is returned when gofpdf fails to parse a PNG
// payload. Render callers log this and skip the chart rather than
// aborting the whole report.
var errImageNotRegistered = simpleErr("gofpdf could not register PNG")

type simpleErr string

func (e simpleErr) Error() string { return string(e) }
