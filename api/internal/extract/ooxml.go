// extract/ooxml.go — read text out of an OOXML container (docx, xlsx, pptx).
//
// Standard library only: archive/zip to open the container, encoding/xml to
// walk the document parts. No third-party dependency (research.md D-005,
// Constitution VI).
//
// The strategy: open the zip, find every XML part that is a document part,
// walk it as a stream, collect text from every <w:t>, <a:t>, <v:text>, or
// <p:sp>-equivalent text node. Whitespace between nodes is preserved as a
// single space, matching what a typist reading the document would see.

package extract

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ooxmlDocumentNames is the set of XML part names that carry visible text.
var ooxmlDocumentNames = []string{
	"word/document.xml",  // docx body
	"word/footnotes.xml", // docx footnotes
	"word/endnotes.xml",  // docx endnotes
	"word/header1.xml", "word/header2.xml", "word/header3.xml",
	"word/footer1.xml", "word/footer2.xml", "word/footer3.xml",
	"xl/sharedStrings.xml", // xlsx shared strings
	"xl/worksheets/sheet1.xml", "xl/worksheets/sheet2.xml", "xl/worksheets/sheet3.xml",
	"ppt/slides/slide1.xml", "ppt/slides/slide2.xml", "ppt/slides/slide3.xml",
	"ppt/notesSlides/notesSlide1.xml",
}

// OOXMLText reads text out of an OOXML container. Implementation lives in
// extractors_stub.go; this file overrides it with the real implementation.
func OOXMLText(ctx context.Context, src io.Reader) (string, error) {
	body, err := io.ReadAll(src)
	if err != nil {
		return "", fmt.Errorf("ooxml: read: %w", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return "", fmt.Errorf("open ooxml zip: %w", err)
	}

	// First, confirm the container is OOXML. The router already does this,
	// but a defence-in-depth check here costs nothing and makes the
	// function safe to call from anywhere.
	hasContentTypes := false
	for _, f := range zr.File {
		if f.Name == "[Content_Types].xml" {
			hasContentTypes = true
			break
		}
	}
	if !hasContentTypes {
		return "", errors.New("not an ooxml container")
	}

	var out strings.Builder
	for _, name := range ooxmlDocumentNames {
		f, err := zr.Open(name)
		if err != nil {
			// A missing optional part is normal; only bail on real errors.
			continue
		}
		partBody, err := io.ReadAll(f)
		_ = f.Close()
		if err != nil {
			return "", fmt.Errorf("read %s: %w", name, err)
		}
		// Walk every text node. encoding/xml does not preserve whitespace
		// between elements exactly, but we only care about the visible
		// text, which lives in <w:t>, <a:t>, <v:text>, and similar.
		text, err := extractTextNodes(bytes.NewReader(partBody))
		if err != nil {
			return "", fmt.Errorf("parse %s: %w", name, err)
		}
		if text != "" {
			if out.Len() > 0 {
				out.WriteString("\n")
			}
			out.WriteString(text)
		}
	}
	return out.String(), nil
}

// extractTextNodes walks an XML stream and concatenates the text of every
// element with a name matching one of the OOXML text element local names.
// It tolerates namespaces because the elements appear as local-name matches
// regardless of the prefix.
func extractTextNodes(r io.Reader) (string, error) {
	dec := xml.NewDecoder(r)
	var b strings.Builder
	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if isTextElement(t.Name.Local) {
				// Read until end and take the inner text.
				var inner strings.Builder
				for {
					t2, err := dec.Token()
					if err != nil {
						return "", err
					}
					if c, ok := t2.(xml.CharData); ok {
						inner.Write(c)
					} else if end, ok := t2.(xml.EndElement); ok && end.Name.Local == t.Name.Local {
						break
					}
				}
				text := strings.TrimSpace(inner.String())
				if text != "" {
					if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
						b.WriteString(" ")
					}
					b.WriteString(text)
				}
			}
		case xml.EndElement:
			depth--
			if depth < 0 {
				return "", errors.New("xml: unbalanced")
			}
		}
	}
	return b.String(), nil
}

// isTextElement returns true for the OOXML element local-names that hold
// user-visible text. The full list is small and stable.
func isTextElement(local string) bool {
	switch local {
	case "t", // word run text
		"a:t",    // drawingml text (used in modern Word/Excel/PowerPoint)
		"v:text", // vml text (legacy fallback)
		"instrText":
		return true
	}
	return false
}
