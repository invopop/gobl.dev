package editor

import (
	"strings"

	"github.com/invopop/gobl.dev/editor/examples"
)

// pickExampleFromAcceptLanguage walks an Accept-Language header in order of
// appearance (ignoring q-weights, close enough for a default picker) and
// returns the first curated example whose country matches a tag's region.
// Falls back to the US example if no tag yields a match.
func pickExampleFromAcceptLanguage(header string) examples.Example {
	for _, raw := range strings.Split(header, ",") {
		tag := strings.TrimSpace(raw)
		if i := strings.Index(tag, ";"); i >= 0 {
			tag = tag[:i]
		}
		region := regionFromTag(tag)
		if region == "" {
			continue
		}
		for _, e := range examples.All() {
			if e.Country == region {
				return e
			}
		}
	}
	return examples.DefaultFor("US")
}

// regionFromTag extracts the ISO-3166-1 alpha-2 region code from a BCP-47 tag
// like "en-GB" or "es-419". Returns the region in upper-case, or an empty
// string if no two-letter region is present.
func regionFromTag(tag string) string {
	parts := strings.Split(tag, "-")
	for _, p := range parts[1:] {
		if len(p) == 2 && isAlpha(p) {
			return strings.ToUpper(p)
		}
	}
	return ""
}

func isAlpha(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return false
		}
	}
	return true
}
