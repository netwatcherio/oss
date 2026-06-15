package reports

import (
	"bytes"
	"testing"

	"github.com/jung-kurt/gofpdf"
)

// newTestPDF returns a fresh gofpdf PDF with margins set up. Tests
// that build a PDF section-by-section use this so the boilerplate
// is in one place.
func newTestPDF(t *testing.T) *gofpdf.Fpdf {
	t.Helper()
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(pdfMargin, pdfMargin, pdfMargin)
	pdf.AddPage()
	return pdf
}

// pdfBytes returns the rendered PDF bytes. Tests use this to assert
// on output (e.g., that the magic header is present).
func pdfBytes(t *testing.T, pdf *gofpdf.Fpdf) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("pdf.Output: %v", err)
	}
	return buf.Bytes()
}

// startsWith is a tiny helper to keep the assertions readable.
func startsWith(haystack, prefix []byte) bool {
	if len(haystack) < len(prefix) {
		return false
	}
	for i, b := range prefix {
		if haystack[i] != b {
			return false
		}
	}
	return true
}

func prefix(b []byte, n int) string {
	if n > len(b) {
		n = len(b)
	}
	return string(b[:n])
}
