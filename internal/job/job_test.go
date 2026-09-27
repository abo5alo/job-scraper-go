package job

import "testing"

func TestSeniorityFromTitle(t *testing.T) {
	tests := []struct {
		title string
		want  Seniority
	}{
		{"Senior Backend Engineer", SenioritySenior},
		{"Sr. Data Analyst", SenioritySenior},
		{"Junior Frontend Developer", SeniorityJunior},
		{"Graduate Software Engineer", SeniorityJunior},
		{"Software Engineering Intern", SeniorityIntern},
		{"Internal Auditor", ""}, // "internal" is not "intern"
		{"Team Lead - Education", SeniorityLead},
		{"Principal Engineer", SeniorityLead},
		{"Staff Software Development Engineer SDM", SeniorityLead},
		{"Staff Accountant", ""}, // not an engineering "staff" level
		{"Lead Generation Specialist", ""},
		{"CEO Office Manager- Riyadh", ""},
		{"Project Manager, Office of the CEO", ""},
		{"UX Writer (Tamheer)", SeniorityJunior},
		{"Head of Security", SeniorityExecutive},
		{"VP Pricing & Packaging", SeniorityExecutive},
		{"Senior Director, Finance", SeniorityExecutive}, // director beats senior
		{"CTO", SeniorityExecutive},
		{"Accountant", ""},
	}
	for _, tt := range tests {
		if got := SeniorityFromTitle(tt.title); got != tt.want {
			t.Errorf("SeniorityFromTitle(%q) = %q, want %q", tt.title, got, tt.want)
		}
	}
}

func TestSeniorityFromLabel(t *testing.T) {
	tests := map[string]Seniority{
		"Internship":       SeniorityIntern,
		"student_college":  SeniorityIntern,
		"Entry level":      SeniorityJunior,
		"entry_level":      SeniorityJunior,
		"Director":         SeniorityExecutive,
		"Mid-Senior level": "", // too vague to trust
		"Associate":        "",
		"":                 "",
	}
	for label, want := range tests {
		if got := SeniorityFromLabel(label); got != want {
			t.Errorf("SeniorityFromLabel(%q) = %q, want %q", label, got, want)
		}
	}
}

func TestCountryFromText(t *testing.T) {
	tests := map[string]string{
		"Dubai, United Arab Emirates":         "AE",
		"Riyadh, Saudi Arabia":                "SA",
		"Al Khobar":                           "SA",
		"Karachi, Pakistan; Lahore, Pakistan": "PK",
		"Cairo Office":                        "EG",
		"New Delhi, India":                    "IN",
		"Bucharest, Romania":                  "RO", // must not match "oman"
		"Remote":                              "",
		"Worldwide":                           "",
		"Remote in Europe":                    "", // "in" is not India
		"":                                    "",
	}
	for text, want := range tests {
		if got := CountryFromText(text); got != want {
			t.Errorf("CountryFromText(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestCountryCode(t *testing.T) {
	tests := map[string]string{
		"ae":           "AE",
		"SA":           "SA",
		"Saudi Arabia": "SA",
		"UAE":          "AE",
		"xx":           "",
		"":             "",
	}
	for in, want := range tests {
		if got := CountryCode(in); got != want {
			t.Errorf("CountryCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalize(t *testing.T) {
	// The title's explicit "Senior" beats the source's "Entry level" label,
	// and a structured country beats what the location text suggests.
	j := Job{Title: "Senior Accountant", Seniority: SeniorityJunior, Country: "sa", Location: "Dubai"}
	j.Normalize()
	if j.Seniority != SenioritySenior || j.Country != "SA" {
		t.Errorf("got seniority=%q country=%q, want senior/SA", j.Seniority, j.Country)
	}

	// No signal anywhere: default to mid, detect country from location text.
	j = Job{Title: "Accountant", Location: "Cairo, Egypt"}
	j.Normalize()
	if j.Seniority != SeniorityMid || j.Country != "EG" {
		t.Errorf("got seniority=%q country=%q, want mid/EG", j.Seniority, j.Country)
	}
}
