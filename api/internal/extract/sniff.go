// Package extract decides what an attachment's bytes are and reads text out
// of them. Subprocesses are fed over pipes (research.md D-004); content type
// is decided from bytes, not filename (research.md D-003); missing tools
// produce not_eligible, not failed (research.md D-012).
//
// The package splits into three families by content kind:
//   - direct read for OOXML (archive/zip + encoding/xml, stdlib only)
//   - direct read for PDF pages with a text layer, rasterise-and-recognise
//     for PDF pages that are scans (tesseract over pipes)
//   - recognition for images (tesseract over pipes)
//
// A fourth family — legacy binary Office formats — is marked not_eligible
// rather than failed; nothing is wrong with the document (research.md D-005).
package extract

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Kind is what the sniffer decides a file is. The router in router.go maps
// each kind to a family and to the right tool for the job. Kind is distinct
// from the content-type string the API returns: pdf and docx are kinds,
// "application/pdf" and the OOXML sniff are the underlying signals.
type Kind string

const (
	KindPDF     Kind = "pdf"
	KindOOXML   Kind = "ooxml"
	KindOLE     Kind = "ole" // legacy binary Office — accepted, not eligible
	KindImage   Kind = "image"
	KindUnknown Kind = "unknown"
)

// Family is the extraction strategy the router chose. Image family and the
// image-with-text-layer route both end up at tesseract, just through
// different code paths.
type Family string

const (
	FamilyDirect      Family = "direct"
	FamilyRecognize   Family = "recognize"
	FamilyPDFMixed    Family = "pdf_mixed" // per-page choice
	FamilyNotEligible Family = "not_eligible"
)

// magicBytes is the list of leading-byte patterns we recognise beyond what
// http.DetectContentType gives us. The order matters: a PDF is checked before
// the image sniffer because the PDF header overlaps with some image
// detection heuristics.
var (
	pdfMagic  = []byte("%PDF")
	zipMagic  = []byte{'P', 'K', 0x03, 0x04}
	oleMagic  = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}
	pngMagic  = []byte{0x89, 'P', 'N', 'G'}
	jpegMagic = []byte{0xFF, 0xD8, 0xFF}
	gifMagic  = []byte("GIF8")
	bmpMagic  = []byte("BM")
	tiffLE    = []byte("II*\x00")
	tiffBE    = []byte("MM\x00*")
)

// Sniff inspects at most the first 8 KiB and decides the kind. It deliberately
// ignores filename hints: an executable renamed deed.pdf is the rejection
// case FR-004a names (research.md D-003).
//
// The returned content-type is what the API reports and is what the
// browser will see. It is the sniffed type, never the filename extension.
func Sniff(r io.Reader) (Kind, string, error) {
	// Read enough to cover every magic we care about plus http.DetectContentType.
	head := make([]byte, 8192)
	n, err := io.ReadFull(r, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", "", fmt.Errorf("sniff: read: %w", err)
	}
	head = head[:n]

	if bytes.HasPrefix(head, pdfMagic) {
		return KindPDF, "application/pdf", nil
	}
	if bytes.HasPrefix(head, oleMagic) {
		return KindOLE, "application/x-ole-storage", nil
	}
	if bytes.HasPrefix(head, zipMagic) {
		// A ZIP that is not OOXML is a refusal — research.md D-003. We have
		// to look inside far enough to decide. OpenReader on a byte slice
		// works without a temp file (zip.NewReader is the io.Reader form).
		if isOOXML(head) {
			return KindOOXML, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", nil
		}
		return KindUnknown, "application/zip", nil
	}
	if bytes.HasPrefix(head, pngMagic) {
		return KindImage, "image/png", nil
	}
	if bytes.HasPrefix(head, jpegMagic) {
		return KindImage, "image/jpeg", nil
	}
	if bytes.HasPrefix(head, gifMagic) {
		return KindImage, "image/gif", nil
	}
	if bytes.HasPrefix(head, bmpMagic) {
		return KindImage, "image/bmp", nil
	}
	if bytes.HasPrefix(head, tiffLE) || bytes.HasPrefix(head, tiffBE) {
		return KindImage, "image/tiff", nil
	}

	// Fallback: trust the standard library's heuristic. If it reports pdf
	// from magic-less input we still refuse — only our explicit magic check
	// is allowed to admit pdf. But for plain text or other types the
	// library's call is good enough.
	detected := http.DetectContentType(head)
	if strings.HasPrefix(detected, "image/") {
		return KindImage, detected, nil
	}
	return KindUnknown, detected, nil
}

// isOOXML opens a ZIP central directory far enough to see whether
// [Content_Types].xml is present. Done in-process so the sniffer never has
// to write a temp file (Constitution VII).
func isOOXML(head []byte) bool {
	zr, err := zip.NewReader(bytes.NewReader(head), int64(len(head)))
	if err != nil {
		return false
	}
	for _, f := range zr.File {
		if f.Name == "[Content_Types].xml" {
			return true
		}
	}
	return false
}

// ErrRefused is returned when a file is of an accepted family but the sniffer
// refused it. The message is an Arabic user-facing phrase and never carries
// file content.
type ErrRefused struct {
	Reason string
	Detail string
}

func (e *ErrRefused) Error() string { return e.Reason + ": " + e.Detail }

// IsRefused is a small helper for callers that need to branch on the kind
// of error without importing errors.As in every place.
func IsRefused(err error) bool {
	var r *ErrRefused
	return errors.As(err, &r)
}

// KindForContentType maps a stored content type back to its extraction family.
//
// The sniffer decides a Kind at upload, but only the content type is persisted,
// so the worker has to recover the Kind when it picks the row up later. Casting
// the stored MIME string straight to Kind does not work — Kind values are short
// tokens like "pdf", not MIME types — and silently routes every attachment to
// the unsupported-type branch.
func KindForContentType(contentType string) Kind {
	switch {
	case contentType == "application/pdf":
		return KindPDF
	case strings.HasPrefix(contentType, "image/"):
		return KindImage
	case contentType == "application/x-ole-storage",
		contentType == "application/msword",
		contentType == "application/vnd.ms-excel",
		contentType == "application/vnd.ms-powerpoint":
		return KindOLE
	case strings.HasPrefix(contentType, "application/vnd.openxmlformats-officedocument."):
		return KindOOXML
	default:
		return KindUnknown
	}
}
