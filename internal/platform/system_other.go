//go:build !windows

package platform

// system holds nothing: sweep cleans only Windows.
type system struct{}

// NewSystem answers with ErrNotWindows: only the Windows build can tell the
// build of the running system.
func NewSystem() (System, error) { return nil, ErrNotWindows }

// UILanguage answers English off Windows: sweep cleans Windows only, and
// the tests that reach for a language still get the fallback.
func (system) UILanguage() string { return "en" }

// Clipboard answers an error off Windows: there is no clipboard sweep may
// read here.
func (system) Clipboard() (string, error) { return "", ErrNotWindows }
