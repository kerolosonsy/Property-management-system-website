package extract

import (
	"archive/zip"
	"bytes"
)

// zipWriter is a thin helper around zip.Writer used by the sniffer tests.
// Kept here, separate from the tests, so the test file reads top-to-bottom.
func zipWriter(w *bytes.Buffer) *zip.Writer {
	return zip.NewWriter(w)
}
