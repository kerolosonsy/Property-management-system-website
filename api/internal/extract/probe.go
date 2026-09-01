// extract/probe.go — detect at startup which external tools are present.
//
// Both tesseract and the poppler tools may be absent on a developer machine
// (research.md D-012); the server still has to start and uploads still have
// to succeed. Missing tools degrade extraction to not_eligible rather than
// failing uploads. The probe records what is available once per process so
// the router can consult a simple boolean instead of forking a subprocess
// for every attachment.
package extract

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Capabilities is what the probe discovered. All fields are set once at
// startup and treated as read-only thereafter.
type Capabilities struct {
	TesseractBinary    bool // binary on PATH
	TesseractHasArabic bool // the ara language pack installed
	PdftotextBinary    bool
	PdftoppmBinary     bool
	PdfinfoBinary      bool
}

// Probe looks for the binaries and, where relevant, the language packs. It
// is intentionally tolerant of errors: a missing tool is not an error, only
// a "no" for the corresponding boolean. The timeout prevents the probe from
// blocking startup when a tool is slow to respond.
func Probe(ctx context.Context) Capabilities {
	c := Capabilities{}
	c.TesseractBinary = hasBinary(ctx, "tesseract")
	if c.TesseractBinary {
		c.TesseractHasArabic = tesseractHasArabic(ctx)
	}
	c.PdftotextBinary = hasBinary(ctx, "pdftotext")
	c.PdftoppmBinary = hasBinary(ctx, "pdftoppm")
	c.PdfinfoBinary = hasBinary(ctx, "pdfinfo")
	return c
}

func hasBinary(ctx context.Context, name string) bool {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	// `which` is portable enough for the local-only deployment this project
	// targets; using exec.LookPath avoids spawning a child.
	if _, err := exec.LookPath(name); err == nil {
		return true
	}
	// Fallback: actually invoke the binary with --version so a misnamed
	// binary on a developer's machine still gets a fair check. The timeout
	// keeps this bounded.
	err := exec.CommandContext(cctx, name, "--version").Run()
	return err == nil
}

// tesseractHasArabic runs `tesseract --list-langs` and checks for "ara" in
// the output. Tesseract prints one language per line; "ara" is the ISO code
// for Arabic. The command exits non-zero if the binary itself fails, which
// is fine — we treat that as "no".
func tesseractHasArabic(ctx context.Context) bool {
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, "tesseract", "--list-langs").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == "ara" {
			return true
		}
	}
	return false
}

// String renders a one-line summary for startup logging. The summary names
// each tool and what is missing; it never carries file content or key
// material.
func (c Capabilities) String() string {
	var b strings.Builder
	b.WriteString("extraction capabilities: ")
	if c.TesseractBinary {
		if c.TesseractHasArabic {
			b.WriteString("tesseract(ara+eng) ")
		} else {
			b.WriteString("tesseract(no-ara) ")
		}
	} else {
		b.WriteString("tesseract(missing) ")
	}
	if c.PdftotextBinary {
		b.WriteString("pdftotext ")
	} else {
		b.WriteString("pdftotext(missing) ")
	}
	if c.PdftoppmBinary {
		b.WriteString("pdftoppm ")
	} else {
		b.WriteString("pdftoppm(missing) ")
	}
	return strings.TrimSpace(b.String())
}

// ErrNoTesseract is the structured error returned by image.go when the
// tesseract binary is missing. The router converts this into a not_eligible
// state rather than a failure (research.md D-012).
var ErrNoTesseract = fmt.Errorf("tesseract not installed")

// ErrNoPoppler is the structured error returned by pdf.go when one of the
// poppler tools is missing.
var ErrNoPoppler = fmt.Errorf("poppler tools not installed")

// ErrNoArabicPack is returned when tesseract is installed but the Arabic
// language pack is not.
var ErrNoArabicPack = fmt.Errorf("tesseract ara language pack not installed")
