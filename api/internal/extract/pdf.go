// extract/pdf.go — read text out of a PDF, deciding per page.
//
// A PDF is two different problems at once. Pages carrying a text layer must be
// read exactly; pages that are scans must be rasterised and recognised. The
// common document here — an Egyptian registry file — is both: a generated
// cover page followed by scanned annexes. Choosing per file rather than per
// page loses one half or the other, so this routes per page (research.md
// D-005, FR-017).
//
// Every poppler and tesseract invocation reads from stdin and writes to
// stdout. No command line here contains a path, so no plaintext byte reaches
// disk (research.md D-004, FR-011).

package extract

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"unicode"
)

// maxPDFPages bounds the work one document can demand. Beyond it the
// attachment is reported too large to process rather than occupying the
// worker indefinitely (FR-019's `too_large`).
const maxPDFPages = 200

// ErrTooManyPages reports a PDF with more pages than maxPDFPages.
var ErrTooManyPages = fmt.Errorf("pdf has more pages than the processing limit")

// minPageTextRunes is the threshold below which a page's text layer is treated
// as absent. A scanned page often carries a few stray characters — a header
// stamp, a page number in a footer — and treating those as "this page has
// text" would skip recognition and lose the page's actual content.
const minPageTextRunes = 16

// textLayerIsReliable reports whether a page's text layer can be trusted.
//
// A PDF can carry a text layer whose character mapping is broken — an embedded
// Arabic font with a symbolic encoding and no usable ToUnicode CMap. pdftotext
// then returns the glyph codes reinterpreted through the wrong table, which
// comes out as Greek and Coptic letters: "ΩϳϘϟ΍ϡϗέ" where the page plainly
// reads "رقم القيد". The text extracts without error, is long enough to look
// legitimate, and is entirely useless — both to a reader and to search.
//
// Recognition reads the same page correctly, so an unreliable layer must fall
// through to it. The test is deliberately narrow: characters in the Greek and
// Cyrillic blocks are the signature of this failure, and this application's
// documents are Arabic and Latin (Constitution II), so their presence in bulk
// means the mapping is wrong rather than that the document is Greek.
func textLayerIsReliable(text string) bool {
	var arabic, suspicious, letters int
	for _, r := range text {
		switch {
		case (r >= 0x0600 && r <= 0x06FF) || (r >= 0xFB50 && r <= 0xFDFF) || (r >= 0xFE70 && r <= 0xFEFF):
			arabic++
			letters++
		case (r >= 0x0370 && r <= 0x03FF) || (r >= 0x1F00 && r <= 0x1FFF) || (r >= 0x0400 && r <= 0x04FF):
			suspicious++
			letters++
		case unicode.IsLetter(r):
			letters++
		}
	}
	if letters == 0 {
		return true // nothing to judge; the emptiness check decides
	}
	// Broken mappings produce suspicious characters in bulk and almost no real
	// Arabic. A page with more suspicious characters than Arabic ones, where
	// they are a meaningful share of the text, is not trustworthy.
	if suspicious > arabic && suspicious*20 >= letters {
		return false
	}
	return true
}

// PDFText reads a PDF page by page, using its text layer where one exists and
// recognition where it does not, and returns the concatenation.
func PDFText(ctx context.Context, src io.Reader) (string, error) {
	body, err := io.ReadAll(src)
	if err != nil {
		return "", fmt.Errorf("pdf: read: %w", err)
	}
	if len(body) == 0 {
		return "", nil
	}

	pages, err := pdfPageCount(ctx, body)
	if err != nil {
		return "", err
	}
	if pages > maxPDFPages {
		return "", ErrTooManyPages
	}
	if pages < 1 {
		pages = 1
	}

	var out strings.Builder
	for page := 1; page <= pages; page++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		text, err := runPDFTextLayerPage(ctx, body, page)
		if err != nil {
			return "", err
		}

		if len([]rune(strings.TrimSpace(text))) < minPageTextRunes || !textLayerIsReliable(text) {
			// No usable text layer on this page: rasterise just this page and
			// recognise it. A failure to recognise one page must not discard
			// the pages that did read, so it is recorded and the loop goes on.
			png, rerr := runPDFRasterizePage(ctx, body, page)
			if rerr == nil {
				var recErr error
				text, recErr = ImageText(ctx, bytes.NewReader(png))
				if recErr != nil {
					// Recognition unavailable or failing. Keep whatever the
					// text layer gave, even if it was below the threshold.
					text, _ = runPDFTextLayerPage(ctx, body, page)
				}
			} else if rerr == ErrNoPoppler {
				return "", rerr
			}
		}

		if s := strings.TrimSpace(text); s != "" {
			if out.Len() > 0 {
				out.WriteString("\n\n")
			}
			out.WriteString(s)
		}
	}
	return out.String(), nil
}

// pdfPageCount asks poppler how many pages the document has. pdfinfo ships
// with the same package as pdftotext and pdftoppm, so it adds no new
// prerequisite — and using it avoids taking a Go PDF-parsing dependency
// purely to count pages (Constitution VI).
func pdfPageCount(ctx context.Context, body []byte) (int, error) {
	cmd := exec.CommandContext(ctx, "pdfinfo", "-")
	cmd.Stdin = bytes.NewReader(body)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if _, lookErr := exec.LookPath("pdfinfo"); lookErr != nil {
			return 0, ErrNoPoppler
		}
		return 0, fmt.Errorf("pdfinfo: %w (%s)", err, stderr.String())
	}
	for _, line := range strings.Split(stdout.String(), "\n") {
		if rest, ok := strings.CutPrefix(line, "Pages:"); ok {
			n, err := strconv.Atoi(strings.TrimSpace(rest))
			if err != nil {
				return 0, fmt.Errorf("pdfinfo: unreadable page count %q", strings.TrimSpace(rest))
			}
			return n, nil
		}
	}
	return 0, fmt.Errorf("pdfinfo: no page count in output")
}

func runPDFTextLayerPage(ctx context.Context, body []byte, page int) (string, error) {
	p := strconv.Itoa(page)
	cmd := exec.CommandContext(ctx, "pdftotext", "-enc", "UTF-8", "-f", p, "-l", p, "-", "-")
	cmd.Stdin = bytes.NewReader(body)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if _, lookErr := exec.LookPath("pdftotext"); lookErr != nil {
			return "", ErrNoPoppler
		}
		return "", fmt.Errorf("pdftotext: %w (%s)", err, stderr.String())
	}
	return stdout.String(), nil
}

func runPDFRasterizePage(ctx context.Context, body []byte, page int) ([]byte, error) {
	p := strconv.Itoa(page)
	// The output root MUST be omitted for pdftoppm to write to stdout. Passing
	// "-" as the root does NOT mean stdout — poppler treats it as a filename
	// prefix and writes "-.png" into the process's working directory, which is
	// a plaintext render of a decrypted document on disk and a direct breach of
	// Principle VII. Verified against poppler 25.x: `pdftoppm -singlefile -png -`
	// streams the image, `pdftoppm -singlefile -png - -` silently writes a file
	// and emits nothing.
	cmd := exec.CommandContext(ctx, "pdftoppm", "-singlefile", "-f", p, "-l", p, "-r", "300", "-png", "-")
	cmd.Stdin = bytes.NewReader(body)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if _, lookErr := exec.LookPath("pdftoppm"); lookErr != nil {
			return nil, ErrNoPoppler
		}
		return nil, fmt.Errorf("pdftoppm: %w (%s)", err, stderr.String())
	}
	return stdout.Bytes(), nil
}
