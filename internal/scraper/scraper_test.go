package scraper

import "testing"

func TestFixMojibake(t *testing.T) {
	// Table-driven tests: the idiomatic Go way to check many cases at once.
	tests := []struct {
		in, want string
	}{
		{"MecÃ¡nico DiagnÃ³stico", "Mecánico Diagnóstico"}, // broken: repaired
		{"Mecánico", "Mecánico"},                           // already correct
		{"café", "café"},                                   // real Latin-1 text
		{"Go Engineer", "Go Engineer"},                     // plain ASCII
		{"東京 Engineer", "東京 Engineer"},                     // non-Latin script
	}
	for _, tt := range tests {
		if got := FixMojibake(tt.in); got != tt.want {
			t.Errorf("FixMojibake(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCleanText(t *testing.T) {
	tests := map[string]string{
		"  Senior Engineer ":                     "Senior Engineer",
		"Customer Marketing &amp Product Intern": "Customer Marketing & Product Intern", // no semicolon
		"Legal &amp; Compliance":                 "Legal & Compliance",
		"Legal & Compliance":                     "Legal & Compliance",
		"MecÃ¡nico":                              "Mecánico",
	}
	for in, want := range tests {
		if got := CleanText(in); got != want {
			t.Errorf("CleanText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHTMLToText(t *testing.T) {
	got := HTMLToText("<p>Hello</p><p>world</p><ul><li>Go</li><li>SQL</li></ul>")
	if want := "Hello world Go SQL"; got != want {
		t.Errorf("HTMLToText = %q, want %q", got, want)
	}
}
