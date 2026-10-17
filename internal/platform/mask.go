package platform

import (
	"errors"
	"regexp"
	"strings"
)

// Mask is a path mask compiled once and matched many times.
type Mask struct {
	pattern *regexp.Regexp
}

// CompileMask reads a path mask the way the rule format writes one. The
// separator of a Windows path is a separator and not an escape, `*` takes any
// run of characters inside one segment, `**` takes any run across segments,
// `?` takes one character inside a segment, and every other character stands
// for itself — so a mask needs no escape at all, which is what Windows paths
// allow: of the three mask characters, none can appear in a name.
//
// `**` stands where it is written: the mask `a\**\b` needs both separators.
// Turning it into any number of directories is the business of the walking
// code, which knows what a directory is.
//
// Matching ignores case, the way Windows itself compares paths.
func CompileMask(mask string) (*Mask, error) {
	if mask == "" {
		return nil, errors.New("the mask is empty")
	}

	var pattern strings.Builder
	pattern.WriteString(`(?i)\A`)

	for i := 0; i < len(mask); {
		switch mask[i] {
		case '*':
			if i+1 < len(mask) && mask[i+1] == '*' {
				pattern.WriteString(`.*`)
				i += 2
				continue
			}
			pattern.WriteString(`[^\\]*`)
		case '?':
			pattern.WriteString(`[^\\]`)
		default:
			// A run of ordinary characters goes through as one piece, so that
			// a character outside ASCII keeps its bytes together.
			run := i
			for i < len(mask) && mask[i] != '*' && mask[i] != '?' {
				i++
			}
			pattern.WriteString(regexp.QuoteMeta(mask[run:i]))
			continue
		}
		i++
	}

	pattern.WriteString(`\z`)

	compiled, err := regexp.Compile(pattern.String())
	if err != nil {
		return nil, err
	}
	return &Mask{pattern: compiled}, nil
}

// Match says whether the path is taken by the mask.
func (m *Mask) Match(path string) bool { return m.pattern.MatchString(path) }
