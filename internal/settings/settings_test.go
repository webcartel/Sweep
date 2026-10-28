package settings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/webcartel/sweep/internal/settings"
)

func TestReadAnswersNothingWhenTheFileIsAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")

	name, found, err := settings.Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if found {
		t.Fatalf("Read = %q, found; want nothing at all", name)
	}
	if name != "" {
		t.Errorf("Read = %q, want an empty name", name)
	}
}

func TestWriteThenReadRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")

	if err := settings.Write(path, "ru"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	name, found, err := settings.Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !found || name != "ru" {
		t.Fatalf("Read = %q, %v; want ru, true", name, found)
	}
}

func TestWriteCreatesTheDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deep", "nested", "settings.yaml")

	if err := settings.Write(path, "en"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the file is not there: %v", err)
	}
}

// A config the user wrote by hand may carry more than the language. The
// reader must answer the language it knows and leave the rest alone.
func TestReadIgnoresUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")
	const doc = "language: ru\ntheme: dark\n"
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatalf("write the file: %v", err)
	}

	name, found, err := settings.Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !found || name != "ru" {
		t.Fatalf("Read = %q, %v; want ru, true", name, found)
	}
}
