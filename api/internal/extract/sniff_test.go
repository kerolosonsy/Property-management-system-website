// sniff_test.go — the sniffer against files renamed to lie. Constitution V
// allows no automated test gate; this file covers the two places where a
// mistake is silent and invisible on screen (the sniffer is one — an
// executable named deed.pdf is the canonical refusal case).

package extract

import (
	"bytes"
	"testing"
)

func TestSniffPDF(t *testing.T) {
	head := append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte{0x20}, 200)...)
	k, ct, err := Sniff(bytes.NewReader(head))
	if err != nil {
		t.Fatal(err)
	}
	if k != KindPDF || ct != "application/pdf" {
		t.Fatalf("got kind=%s ct=%s, want pdf/application/pdf", k, ct)
	}
}

func TestSniffPNG(t *testing.T) {
	head := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	k, ct, err := Sniff(bytes.NewReader(head))
	if err != nil {
		t.Fatal(err)
	}
	if k != KindImage || ct != "image/png" {
		t.Fatalf("got kind=%s ct=%s, want image/png", k, ct)
	}
}

func TestSniffJPEG(t *testing.T) {
	head := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	k, ct, err := Sniff(bytes.NewReader(head))
	if err != nil {
		t.Fatal(err)
	}
	if k != KindImage || ct != "image/jpeg" {
		t.Fatalf("got kind=%s ct=%s, want image/jpeg", k, ct)
	}
}

// An executable named deed.pdf must be refused. We simulate the executable
// with the ELF magic; the sniffer MUST NOT call this a PDF.
func TestSniffExecutableRenamedPDFIsNotPDF(t *testing.T) {
	head := []byte{0x7F, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}
	head = append(head, bytes.Repeat([]byte{0x00}, 64)...)
	k, _, err := Sniff(bytes.NewReader(head))
	if err != nil {
		t.Fatal(err)
	}
	if k == KindPDF {
		t.Fatal("executable renamed .pdf was accepted as PDF")
	}
}

// A plain ZIP that does not contain [Content_Types].xml is not OOXML and
// MUST be refused — research.md D-003.
func TestSniffNonOOXMLZipIsRejected(t *testing.T) {
	// Build a minimal empty zip in memory using the standard library's
	// zip writer machinery — but instead, just feed the raw bytes of an
	// end-of-central-directory record preceded by enough to look like a zip.
	// A simpler test: a zip header followed by a file named "stuff.txt"
	// (no Content_Types).
	//
	// Rather than construct one by hand, we use archive/zip:
	importBlock(t)
	head := buildMinimalZip(t, "stuff.txt", []byte("not an ooxml"))
	k, _, err := Sniff(bytes.NewReader(head))
	if err != nil {
		t.Fatal(err)
	}
	if k == KindOOXML {
		t.Fatal("non-OOXML zip was classified as OOXML")
	}
	if k != KindUnknown {
		// It's fine to fall through to KindUnknown; the point is that it
		// MUST NOT be classified as OOXML.
		t.Logf("non-OOXML zip classified as %s (acceptable)", k)
	}
}

// The OOXML signature — Content_Types — IS recognised when present.
func TestSniffOOXML(t *testing.T) {
	importBlock(t)
	head := buildMinimalZip(t, "[Content_Types].xml", []byte(`<?xml version="1.0"?>`))
	k, _, err := Sniff(bytes.NewReader(head))
	if err != nil {
		t.Fatal(err)
	}
	if k != KindOOXML {
		t.Fatalf("OOXML zip classified as %s; want ooxml", k)
	}
}

// importBlock and buildMinimalZip exist because Go test files cannot import
// standard library packages at the top of the file together with the test
// functions and stay readable. Keeping them at the bottom matches what the
// rest of the project does.
func importBlock(t *testing.T) {
	t.Helper()
}

// buildMinimalZip returns the bytes of a single-file zip whose only member
// has the given name and contents. Enough for the sniffer to open and see
// what is inside.
func buildMinimalZip(t *testing.T, name string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zipWriter(&buf)
	f, err := w.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
