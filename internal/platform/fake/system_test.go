package fake_test

import (
	"testing"

	"github.com/webcartel/sweep/internal/platform/fake"
)

// A fake system answers English until a test names another language, so
// that every screen built on it keeps the default the real machine has
// when nothing says more.
func TestFakeSystemReportsTheLanguageItWasGiven(t *testing.T) {
	sys := fake.NewSystem()
	if got := sys.UILanguage(); got != "en" {
		t.Fatalf("UILanguage() = %q, want English until it is set", got)
	}

	sys.SetUILanguage("ru")
	if got := sys.UILanguage(); got != "ru" {
		t.Fatalf("UILanguage() = %q, want %q", got, "ru")
	}
}
