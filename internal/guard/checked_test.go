package guard

import (
	"testing"

	"github.com/webcartel/sweep/internal/model"
)

// TestCheckNormalizedAgreesWithCheck pins that a path the scanner has already
// normalized may be asked about directly without the verdict changing: the
// scanner would otherwise pay for a second Lstat and a second resolve on every
// item before anything is walked at all.
func TestCheckNormalizedAgreesWithCheck(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		mode       model.Mode
		include    []string
		minAgeDays int
		allowed    bool
	}{
		{"a Windows root may not be emptied", `C:\Windows`, model.ModeContents, nil, 0, false},
		{"a Windows exception passes", `C:\Windows\Temp`, model.ModeContents, nil, 0, true},
		{"System32 stays closed even with filters", `C:\Windows\System32`, model.ModeFiles, []string{"*.log"}, 1, false},
		{"the desktop keeps its filters", `C:\Users\probe\Desktop`, model.ModeContents, nil, 0, false},
		{"the desktop with filters passes", `C:\Users\probe\Desktop`, model.ModeFiles, []string{"*.old"}, 1, true},
		{"Downloads is not emptied whole", `C:\Users\probe\Downloads`, model.ModeContents, nil, 0, false},
		{"old installers in Downloads pass", `C:\Users\probe\Downloads`, model.ModeFiles, []string{"*.exe", "*.msi"}, 30, true},
		{"own user data passes", `C:\Users\probe\AppData\Local\Temp\x`, model.ModeContents, nil, 0, true},
		{"a path that is not there is not refused", `C:\Temp\nope`, model.ModeSelf, nil, 0, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g, _ := newGuard(t)

			direct, err := g.Normalize(test.path)
			if err != nil {
				t.Fatalf("Normalize: %v", err)
			}
			asked, err := g.CheckNormalized(direct, test.mode, test.include, test.minAgeDays)
			if err != nil {
				t.Fatalf("CheckNormalized: %v", err)
			}
			called, err := g.Check(test.path, test.mode, test.include, test.minAgeDays)
			if err != nil {
				t.Fatalf("Check: %v", err)
			}

			if asked.Allowed != called.Allowed {
				t.Errorf("Allowed = %v, want the Check verdict %v", asked.Allowed, called.Allowed)
			}
			if asked.Reason != called.Reason {
				t.Errorf("Reason = %q, want %q", asked.Reason, called.Reason)
			}
			if asked.Message != called.Message {
				t.Errorf("Message = %q, want %q", asked.Message, called.Message)
			}
			if asked.Allowed != test.allowed {
				t.Errorf("Allowed = %v, want %v", asked.Allowed, test.allowed)
			}
		})
	}
}
