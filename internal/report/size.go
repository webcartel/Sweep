// Package report renders sizes and the documents a frontend prints. It holds
// no business decisions: what the plan holds is what it says.
package report

import "fmt"

// FormatBytes gives a byte count the short form a person reads: `0 B`,
// `999 B`, `1.0 KiB`, `1.0 MiB`, `1.2 GiB` — the units of the disk, not the
// network, and one decimal after the first place.
func FormatBytes(n int64) string {
	negative := n < 0
	if negative {
		n = -n
	}
	const unit = 1024
	if n < unit {
		return signed(negative, fmt.Sprintf("%d B", n))
	}
	value := float64(n)
	for _, name := range []string{"KiB", "MiB", "GiB", "TiB", "PiB"} {
		value /= unit
		if value < unit {
			// A large number needs no decimal: `812 MiB` says more per
			// column than `812.0 MiB` does.
			if value >= 100 {
				return signed(negative, fmt.Sprintf("%.0f %s", value, name))
			}
			return signed(negative, fmt.Sprintf("%.1f %s", value, name))
		}
	}
	return signed(negative, fmt.Sprintf("%.1f EiB", value/unit))
}

// signed puts the sign back on none of the sizes the plan carries: a size is
// never negative, and the form is here for the odd caller that asks anyway.
func signed(negative bool, text string) string {
	if negative {
		return "-" + text
	}
	return text
}
