// router_test.go — the router picks a family and a state for each accepted
// type. The interesting cases are the missing-tool ones: they must come
// back as not_eligible with a reason, never as failed or pending.
//
// Constitution V allows no automated test gate; this file is one of the
// three the brief permits because routing a kind to the wrong family is
// silent (research.md D-005, D-012).

package extract

import (
	"context"
	"testing"
)

// All-tools-present is the easy case; the router picks the right family.
func TestRouteAllToolsPresent(t *testing.T) {
	caps := Capabilities{
		TesseractBinary: true, TesseractHasArabic: true,
		PdftotextBinary: true, PdftoppmBinary: true, PdfinfoBinary: true,
	}
	cases := []struct {
		kind  Kind
		want  Family
		state string
	}{
		{KindOOXML, FamilyDirect, "pending"},
		{KindImage, FamilyRecognize, "pending"},
		{KindPDF, FamilyPDFMixed, "pending"},
		{KindOLE, FamilyNotEligible, "not_eligible"},
	}
	for _, c := range cases {
		d := Route(context.Background(), c.kind, caps)
		if d.Family != c.want {
			t.Errorf("%s: family=%s want=%s", c.kind, d.Family, c.want)
		}
		if d.State != c.state {
			t.Errorf("%s: state=%s want=%s", c.kind, d.State, c.state)
		}
	}
}

// Missing tesseract degrades image and PDF to not_eligible, never failed.
func TestRouteMissingTesseract(t *testing.T) {
	caps := Capabilities{
		TesseractBinary: false,
		PdftotextBinary: true, PdftoppmBinary: true, PdfinfoBinary: true,
	}
	if d := Route(context.Background(), KindImage, caps); d.State != "not_eligible" {
		t.Errorf("image without tesseract: state=%s want=not_eligible", d.State)
	}
	if d := Route(context.Background(), KindPDF, caps); d.State != "not_eligible" {
		t.Errorf("pdf without tesseract: state=%s want=not_eligible", d.State)
	}
}

// Missing Arabic pack degrades to not_eligible with the right reason.
func TestRouteMissingArabicPack(t *testing.T) {
	caps := Capabilities{
		TesseractBinary: true, TesseractHasArabic: false,
		PdftotextBinary: true, PdftoppmBinary: true, PdfinfoBinary: true,
	}
	d := Route(context.Background(), KindImage, caps)
	if d.State != "not_eligible" {
		t.Fatalf("image without ara pack: state=%s want=not_eligible", d.State)
	}
	if d.Reason != "tesseract-no-arabic" {
		t.Errorf("image without ara pack: reason=%s want=tesseract-no-arabic", d.Reason)
	}
}

// Missing poppler tools degrade PDF to not_eligible.
func TestRouteMissingPoppler(t *testing.T) {
	caps := Capabilities{
		TesseractBinary: true, TesseractHasArabic: true,
		PdftotextBinary: false, PdftoppmBinary: false, PdfinfoBinary: false,
	}
	d := Route(context.Background(), KindPDF, caps)
	if d.State != "not_eligible" {
		t.Fatalf("pdf without poppler: state=%s want=not_eligible", d.State)
	}
	if d.Reason != "poppler-missing" {
		t.Errorf("pdf without poppler: reason=%s want=poppler-missing", d.Reason)
	}
}

// Legacy Office formats are always not_eligible — never failed — because
// nothing is wrong with the document.
func TestRouteOLEIsAlwaysNotEligible(t *testing.T) {
	caps := Capabilities{
		TesseractBinary: true, TesseractHasArabic: true,
		PdftotextBinary: true, PdftoppmBinary: true, PdfinfoBinary: true,
	}
	d := Route(context.Background(), KindOLE, caps)
	if d.State != "not_eligible" {
		t.Errorf("ole: state=%s want=not_eligible", d.State)
	}
	if d.Family != FamilyNotEligible {
		t.Errorf("ole: family=%s want=not_eligible", d.Family)
	}
}

// Unknown kinds are not_eligible, never failed.
func TestRouteUnknownKind(t *testing.T) {
	caps := Capabilities{
		TesseractBinary: true, TesseractHasArabic: true,
		PdftotextBinary: true, PdftoppmBinary: true, PdfinfoBinary: true,
	}
	d := Route(context.Background(), KindUnknown, caps)
	if d.State != "not_eligible" {
		t.Errorf("unknown: state=%s want=not_eligible", d.State)
	}
}
