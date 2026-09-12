package extract

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildMixedPDF returns a two-page PDF: page one carries a real text layer,
// page two carries none. It is built by hand so the test needs no fixture file
// and no tooling beyond poppler.
func buildMixedPDF() []byte {
	obj := func(n int, body []byte) []byte {
		return append(append([]byte{}, []byte(itoa(n)+" 0 obj\n")...), append(body, []byte("\nendobj\n")...)...)
	}
	text := []byte("BT /F1 14 Tf 72 700 Td (Registry Number AB-12345 Cover Page) Tj ET")
	stream := append([]byte("<< /Length "+itoa(len(text))+" >>\nstream\n"), append(text, []byte("\nendstream")...)...)
	blank := []byte("BT ET")
	stream2 := append([]byte("<< /Length "+itoa(len(blank))+" >>\nstream\n"), append(blank, []byte("\nendstream")...)...)

	objs := [][]byte{
		obj(1, []byte("<< /Type /Catalog /Pages 2 0 R >>")),
		obj(2, []byte("<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>")),
		obj(3, []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 7 0 R >> >> >>")),
		obj(4, stream),
		obj(5, []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 6 0 R /Resources << >> >>")),
		obj(6, stream2),
		obj(7, []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")),
	}
	out := []byte("%PDF-1.4\n")
	offs := make([]int, 0, len(objs))
	for _, o := range objs {
		offs = append(offs, len(out))
		out = append(out, o...)
	}
	xref := len(out)
	out = append(out, []byte("xref\n0 "+itoa(len(objs)+1)+"\n0000000000 65535 f \n")...)
	for _, off := range offs {
		out = append(out, []byte(pad10(off)+" 00000 n \n")...)
	}
	out = append(out, []byte("trailer\n<< /Size "+itoa(len(objs)+1)+" /Root 1 0 R >>\nstartxref\n"+itoa(xref)+"\n%%EOF\n")...)
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func pad10(n int) string {
	s := itoa(n)
	for len(s) < 10 {
		s = "0" + s
	}
	return s
}

func popplerAvailable() bool {
	c := Probe(context.Background())
	return c.PdftotextBinary && c.PdftoppmBinary && c.PdfinfoBinary
}

// TestPDFTextReadsTextLayerPerPage confirms the per-page router reads a page
// that has a text layer exactly, rather than rasterising the whole document.
func TestPDFTextReadsTextLayerPerPage(t *testing.T) {
	if !popplerAvailable() {
		t.Skip("poppler not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	got, err := PDFText(ctx, bytes.NewReader(buildMixedPDF()))
	if err != nil {
		t.Fatalf("PDFText: %v", err)
	}
	if !strings.Contains(got, "AB-12345") {
		t.Fatalf("text layer not read; got %q", got)
	}
}

// TestPDFTextWritesNothingToDisk is the regression guard for a real defect:
// `pdftoppm -png - -` does NOT write to stdout. Poppler treats the second "-"
// as a filename prefix and silently writes "-.png" into the working directory,
// which is a plaintext render of a decrypted document on disk — a breach of
// Principle VII that produces no error and no output. The output root must be
// omitted entirely.
func TestPDFTextWritesNothingToDisk(t *testing.T) {
	if !popplerAvailable() {
		t.Skip("poppler not installed")
	}
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(orig)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := PDFText(ctx, bytes.NewReader(buildMixedPDF())); err != nil {
		t.Fatalf("PDFText: %v", err)
	}

	entries, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("extraction wrote %d file(s) to disk: %v — plaintext must never be written (Principle VII)", len(entries), entries)
	}
}

// TestTextLayerReliability guards the defect where a PDF's text layer carries a
// broken character mapping: pdftotext returns Greek/Coptic mojibake instead of
// Arabic, long enough to pass the emptiness check, so the page was stored and
// indexed as garbage instead of falling through to recognition.
func TestTextLayerReliability(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{"real arabic", "رقم القيد ١٢٣ جمهورية مصر العربية", true},
		{"latin only", "AD/BTH-NC/0002603/2019 155996756", true},
		{"mojibake arabic", "ΔϳΑέόϟ΍έλϣΔϳέϭϬϤΟ ϪΗϳγϧΟ ΩϳϘϟ΍ϡϗέ", false},
		{"mixed latin and mojibake", "AD/BTH-NC/0002603/2019\n155996756\nΩϳϘϟ΍ϡϗέ", false},
		{"empty", "", true},
		{"arabic with a stray greek word", "رقم القيد جمهورية مصر العربية والحمد لله رب العالمين alpha", true},
	}
	for _, c := range cases {
		if got := textLayerIsReliable(c.text); got != c.want {
			t.Errorf("%s: textLayerIsReliable = %v, want %v", c.name, got, c.want)
		}
	}
}
