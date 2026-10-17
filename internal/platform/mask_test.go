package platform

import "testing"

func TestCompileMaskReadsTheMaskLanguageOfTheRules(t *testing.T) {
	tests := []struct {
		name  string
		mask  string
		path  string
		taken bool
	}{
		{
			name:  "a star stays inside one segment",
			mask:  `C:\Cache\*.log`,
			path:  `C:\Cache\a.log`,
			taken: true,
		},
		{
			name:  "a star does not cross a separator",
			mask:  `C:\Cache\*.log`,
			path:  `C:\Cache\sub\a.log`,
			taken: false,
		},
		{
			name:  "a question takes one character",
			mask:  `C:\Cache\a?c`,
			path:  `C:\Cache\abc`,
			taken: true,
		},
		{
			name:  "a question takes exactly one character",
			mask:  `C:\Cache\a?c`,
			path:  `C:\Cache\ac`,
			taken: false,
		},
		{
			name:  "two stars cross separators",
			mask:  `C:\**\Cache`,
			path:  `C:\Users\probe\Cache`,
			taken: true,
		},
		{
			name:  "two stars stand where they are written",
			mask:  `C:\**\Cache`,
			path:  `C:\Cache`,
			taken: false,
		},
		{
			name:  "matching ignores case, as Windows does",
			mask:  `c:\cache\*.LOG`,
			path:  `C:\Cache\a.log`,
			taken: true,
		},
		{
			name:  "a trailing separator is a directory, not an escape",
			mask:  `%LOCALAPPDATA%\Google\Chrome\User Data\*\`,
			path:  `%LOCALAPPDATA%\Google\Chrome\User Data\Default\`,
			taken: true,
		},
		{
			name:  "a star beside a separator is a mask, not an escaped star",
			mask:  `C:\Temp\*\Cache`,
			path:  `C:\Temp\probe\Cache`,
			taken: true,
		},
		{
			name:  "brackets are ordinary characters",
			mask:  `C:\Temp\[a]`,
			path:  `C:\Temp\[a]`,
			taken: true,
		},
		{
			name:  "the whole path has to match",
			mask:  `C:\Temp`,
			path:  `C:\Temp\x`,
			taken: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mask, err := CompileMask(test.mask)
			if err != nil {
				t.Fatalf("CompileMask(%q): %v", test.mask, err)
			}
			if got := mask.Match(test.path); got != test.taken {
				t.Errorf("Match(%q) = %v, want %v", test.path, got, test.taken)
			}
		})
	}
}

func TestCompileMaskRejectsAnEmptyMask(t *testing.T) {
	if _, err := CompileMask(""); err == nil {
		t.Fatal("CompileMask accepted an empty mask")
	}
}
