package editor

import "testing"

func TestPickExampleFromAcceptLanguage(t *testing.T) {
	cases := []struct {
		name   string
		header string
		wantID string
	}{
		{"empty falls back to US", "", "us"},
		{"nonsense falls back to US", "xx,yy", "us"},
		{"tag without region", "en", "us"},
		{"en-US", "en-US", "us"},
		{"it-IT maps to IT example", "it-IT", "it-fatturapa"},
		{"es-ES maps to first ES variant", "es-ES", "es"},
		{"de-DE maps to DE plain", "de-DE", "de"},
		{"fr-FR maps to FR plain", "fr-FR", "fr"},
		{"fallback through list", "xx,de-DE,fr-FR", "de"},
		{"q-suffix stripped", "en-GB;q=0.9,fr-FR;q=0.8", "gb"},
		{"region in third position", "zh-Hans-NL", "nl"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := pickExampleFromAcceptLanguage(c.header)
			if got.ID != c.wantID {
				t.Fatalf("got %q, want %q", got.ID, c.wantID)
			}
		})
	}
}
