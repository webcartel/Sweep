// Package settings keeps the small choices a user makes in the interface
// and wants kept between runs. Today it holds one: the language. The file
// is small and written by sweep alone, and it lives next to config.yaml so
// that a portable copy of the config carries the language with it.
package settings

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// document is the whole settings.yaml. Anything else the file holds is
// read and left alone, so the file may grow without this reader breaking.
type document struct {
	Language string `yaml:"language"`
}

// Read answers the language saved in the file, and whether a file was
// there at all. A file that is not there is not an error: the first run
// has no settings yet, and every default applies.
func Read(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	var doc document
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", false, err
	}
	return doc.Language, true, nil
}

// Write saves the language, making the directory on the way. It writes a
// temporary file and renames it over the target, so a crash in the middle
// of a write cannot leave half a file behind.
func Write(path, lang string) error {
	data, err := yaml.Marshal(document{Language: lang})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
