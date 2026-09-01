// extract/image.go — recognise text from images via tesseract.
//
// The subprocess is invoked as `tesseract stdin stdout -l ara+eng` with the
// image bytes on stdin and the recognised text on stdout. No file path is
// ever part of the command line; this is research.md D-004's binding rule.

package extract

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
)

// ImageText implements recognition from pixels. The plaintext image bytes
// arrive on src; the recognised text is returned as a single string.
func ImageText(ctx context.Context, src io.Reader) (string, error) {
	body, err := io.ReadAll(src)
	if err != nil {
		return "", fmt.Errorf("image: read: %w", err)
	}
	if len(body) == 0 {
		return "", nil
	}
	// "stdin stdout" — tesseract reads the image from stdin and writes text
	// to stdout. The dash is the literal argument, not a path.
	cmd := exec.CommandContext(ctx, "tesseract", "stdin", "stdout", "-l", "ara+eng")
	cmd.Stdin = bytes.NewReader(body)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Distinguish "not installed" so the router can map it to
		// not_eligible. exec.LookPath gives the same answer but is
		// cheaper; we try it first.
		if _, lookErr := exec.LookPath("tesseract"); lookErr != nil {
			return "", ErrNoTesseract
		}
		// Missing language pack produces a specific stderr line that
		// includes "could not create TXT output" or "Error, could not
		// create TXT output file". We treat any failure as a tool error
		// and let the worker's classifier decide between ara-missing and
		// tool-error.
		if bytes.Contains(stderr.Bytes(), []byte("could not create TXT output")) ||
			bytes.Contains(stderr.Bytes(), []byte("language")) {
			return "", ErrNoArabicPack
		}
		return "", fmt.Errorf("tesseract: %w (%s)", err, stderr.String())
	}
	return stdout.String(), nil
}
