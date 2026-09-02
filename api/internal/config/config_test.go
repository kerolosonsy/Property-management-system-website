package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebDistValidationRequiresExistingAbsoluteDirectory(t *testing.T) {
	t.Run("empty is disabled", func(t *testing.T) {
		if err := validateWebDist(""); err != nil {
			t.Fatalf("validateWebDist: %v", err)
		}
	})

	t.Run("existing absolute directory", func(t *testing.T) {
		if err := validateWebDist(t.TempDir()); err != nil {
			t.Fatalf("validateWebDist: %v", err)
		}
	})

	t.Run("relative path", func(t *testing.T) {
		err := validateWebDist("web/dist")
		if err == nil || !strings.Contains(err.Error(), "must be an absolute path") {
			t.Fatalf("validateWebDist error = %v", err)
		}
	})

	t.Run("missing path", func(t *testing.T) {
		err := validateWebDist(filepath.Join(t.TempDir(), "missing"))
		if err == nil || !strings.Contains(err.Error(), "is not usable") {
			t.Fatalf("validateWebDist error = %v", err)
		}
	})

	t.Run("file", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "bundle")
		if err := os.WriteFile(file, []byte("not a directory"), 0o600); err != nil {
			t.Fatal(err)
		}
		err := validateWebDist(file)
		if err == nil || !strings.Contains(err.Error(), "must be a directory") {
			t.Fatalf("validateWebDist error = %v", err)
		}
	})
}

func TestWebDistCannotOverlapAttachmentStore(t *testing.T) {
	root := t.TempDir()
	webDist := filepath.Join(root, "web")
	attachmentStore := filepath.Join(root, "attachments")
	if err := os.Mkdir(webDist, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(attachmentStore, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := validateWebDistSeparation(webDist, attachmentStore); err != nil {
		t.Fatalf("separate directories rejected: %v", err)
	}

	nestedStore := filepath.Join(webDist, "attachments")
	if err := os.Mkdir(nestedStore, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := validateWebDistSeparation(webDist, nestedStore); err == nil {
		t.Fatal("attachment store nested under web dist was accepted")
	}

	nestedWebDist := filepath.Join(attachmentStore, "web")
	if err := os.Mkdir(nestedWebDist, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := validateWebDistSeparation(nestedWebDist, attachmentStore); err == nil {
		t.Fatal("web dist nested under attachment store was accepted")
	}
}
