// extract/router.go — given a sniffer result and the available capabilities,
// pick the family and the concrete extractor function. The router is the
// only place that knows about the full set of families; image.go, pdf.go,
// and ooxml.go are family-specific and never call each other.
//
// A missing tool produces not_eligible rather than failed (research.md
// D-012). The router is what enforces that: every tool absence is mapped
// to not_eligible with a reason code that the worker records verbatim and
// the UI shows in Arabic.

package extract

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// Decision is what the router returns: which family to use and which
// extractor to call, or why it is not eligible. The worker writes
// `State` and `Reason` directly to the database; `Family` is informational
// and lives in logs.
type Decision struct {
	Family    Family
	State     string    // "pending" or "not_eligible"
	Reason    string    // empty when State == "pending"; a reason code otherwise
	Extractor Extractor // nil when State == "not_eligible"
}

// Extractor pulls text out of one attachment. The input is the plaintext
// stream; the output is whatever the extractor could read, possibly empty.
// Implementations live in image.go, pdf.go, and ooxml.go.
type Extractor func(ctx context.Context, src io.Reader) (string, error)

// Route picks the family for one attachment. kind comes from the sniffer;
// caps from the startup probe. The Decision's Extractor closure receives
// the plaintext stream and is responsible for piping it to its tool
// without ever writing the bytes to disk (research.md D-004).
func Route(ctx context.Context, kind Kind, caps Capabilities) Decision {
	switch kind {
	case KindOOXML:
		return Decision{Family: FamilyDirect, State: "pending", Extractor: OOXMLText}
	case KindImage:
		if !caps.TesseractBinary {
			return notEligible(FamilyRecognize, "tesseract-missing")
		}
		if !caps.TesseractHasArabic {
			return notEligible(FamilyRecognize, "tesseract-no-arabic")
		}
		return Decision{Family: FamilyRecognize, State: "pending", Extractor: ImageText}
	case KindPDF:
		// pdfinfo is needed too: the per-page router asks it for the page count
		// before deciding anything (pdf.go).
		if !caps.PdftotextBinary || !caps.PdftoppmBinary || !caps.PdfinfoBinary {
			return notEligible(FamilyPDFMixed, "poppler-missing")
		}
		if !caps.TesseractBinary {
			return notEligible(FamilyPDFMixed, "tesseract-missing")
		}
		if !caps.TesseractHasArabic {
			return notEligible(FamilyPDFMixed, "tesseract-no-arabic")
		}
		return Decision{Family: FamilyPDFMixed, State: "pending", Extractor: PDFText}
	case KindOLE:
		return notEligible(FamilyNotEligible, "legacy-office-format")
	default:
		return notEligible(FamilyNotEligible, "unsupported-type")
	}
}

func notEligible(family Family, reason string) Decision {
	return Decision{Family: family, State: "not_eligible", Reason: reason}
}

// IsNotEligible is the predicate the worker uses to map a router result to
// the schema's enum without string-matching on every comparison.
func IsNotEligible(d Decision) bool {
	return d.State == "not_eligible"
}

// ErrNoExtractor is returned by a misuse: someone called d.Extractor when
// the router had decided not_eligible. The worker guards against this, but
// the panic message would be the only other signal.
var ErrNoExtractor = errors.New("router returned no extractor")

// Run invokes the chosen extractor. The caller supplies the plaintext
// stream; Run returns the extracted text or an error. Run is the single
// place the worker calls.
func (d Decision) Run(ctx context.Context, src io.Reader) (string, error) {
	if d.Extractor == nil {
		return "", ErrNoExtractor
	}
	out, err := d.Extractor(ctx, src)
	if err != nil {
		return "", fmt.Errorf("extract: %w", err)
	}
	return out, nil
}
