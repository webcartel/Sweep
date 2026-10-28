package guard

import (
	"strings"
	"testing"

	"github.com/webcartel/sweep/internal/i18n"
)

// A refusal is read by a person, so it follows the language of the
// interface. The English text is still there under English.
func TestRefusalFollowsTheInterfaceLanguage(t *testing.T) {
	g, _ := newGuard(t)

	restore := i18n.Current()
	t.Cleanup(func() { i18n.Use(restore) })

	i18n.Use(i18n.LangRU)
	_, err := g.Normalize("")
	if err == nil || !strings.Contains(err.Error(), "путь") {
		t.Fatalf("the refusal in Russian = %v", err)
	}

	i18n.Use(i18n.LangEN)
	_, err = g.Normalize("")
	if err == nil || !strings.Contains(err.Error(), "the path is empty") {
		t.Fatalf("the refusal in English = %v", err)
	}
}
