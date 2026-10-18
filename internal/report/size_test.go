package report_test

import "testing"

import "github.com/webcartel/sweep/internal/report"

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{999, "999 B"},
		{1024, "1.0 KiB"},
		{1025, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1048576, "1.0 MiB"},
		{851443712, "812 MiB"},
		{1288490188, "1.2 GiB"},
		{-5, "-5 B"},
	}
	for _, test := range tests {
		if got := report.FormatBytes(test.in); got != test.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", test.in, got, test.want)
		}
	}
}
