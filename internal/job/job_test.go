package job

import (
	"slices"
	"testing"
)

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

func TestSkillsFromText(t *testing.T) {
	tests := []struct {
		text string
		want []string
	}{
		// Phrases taken from real postings.
		{"Comfortable in Go, TypeScript, GraphQL, Postgres", []string{"TypeScript", "Go", "PostgreSQL"}},
		{"distributed systems written in Go. You will report", []string{"Go"}},
		{"automation skills (Python, Bash, Go)", []string{"Python", "Go"}},
		{"NodeJS, Go-Lang, and/or Python a plus", []string{"Python", "Go", "Node.js"}},
		{"UAT, Go-Live & Post-Go-Live Stabilization", nil},
		{"Go-To-Market Specialist", nil},
		{"Go the extra mile to meet sales quotas. Go beyond Excel", []string{"Excel"}},
		{"Excel in a fast-paced team", nil},
		{"Advanced MS Excel and Microsoft Office", []string{"Excel", "Microsoft Office"}},
		{"Swift/SwiftUI for our iOS app", []string{"Swift"}},
		{"ensuring swift time-to-value", nil},
		{"React and React Native; able to react quickly", []string{"React"}},
		{"Java 8 and Spring Boot", []string{"Java", "Spring"}},
		{"JavaScript only", []string{"JavaScript"}},
		{"ASP.NET Core and C#; see example.net", []string{"C#", ".NET"}},
		{"Fluent in Arabic and English", []string{"English", "Arabic"}},
		{"ACCA or CPA qualified, IFRS knowledge", []string{"IFRS", "ACCA"}},
		{"Accountant", nil},
	}
	for _, tt := range tests {
		if got := SkillsFromText(tt.text); !slices.Equal(got, tt.want) {
			t.Errorf("SkillsFromText(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}

func TestLookupSkill(t *testing.T) {
	if s, ok := LookupSkill(" arabic "); !ok || s.Name != "Arabic" || s.Category != CategorySpoken {
		t.Errorf("LookupSkill(arabic) = %+v, %v", s, ok)
	}
	if _, ok := LookupSkill("Cobol"); ok {
		t.Error("LookupSkill(Cobol) found a skill, want none")
	}
}

func TestNormalizeHidesPlaceholderCompany(t *testing.T) {
	for name, want := range map[string]string{"Company": "", " confidential ": "", "Careem": "Careem"} {
		j := Job{Company: name}
		j.Normalize()
		if j.Company != want {
			t.Errorf("company %q normalized to %q, want %q", name, j.Company, want)
		}
	}
}

func TestIsTech(t *testing.T) {
	tests := []struct {
		title  string
		skills []string
		want   bool
	}{
		// Real titles, tech by title alone.
		{"Senior Full-Stack Software Engineer (React, Next.js & Java)", nil, true},
		{"Senior DevOps Engineer", nil, true},
		{"Senior Site Reliability Engineer", nil, true}, // "site", but not a site engineer
		{"Data Analyst", nil, true},
		{"IT Release Manager", nil, true},
		{"Senior ServiceNow ITSM Architect with AI & ITAM exposure", nil, true},
		{"Analyste en informatique polyvalent(e)", nil, true},
		{"Product Designer", nil, true},
		// Real titles that sound technical but aren't tech jobs.
		{"Senior Site Engineer", nil, false},
		{"B2B Sales Engineer - Air Filtration", nil, false},
		{"Business Development Manager - Software Sales - Public Sector", nil, false},
		{"Senior Electrical BIM Engineer", nil, false},
		{"Senior Graphic Designer", nil, false},
		{"Technical Recruitment Partner", nil, false},
		// Non-tech titles win even when the job lists tech skills.
		{"Cloud Sales Account Manager", []string{"AWS", "Azure"}, false},
		// Vague titles fall back to skills: two technical ones make it tech.
		{"Customer Success Engineer", []string{"Java", "SQL", "React"}, true},
		{"TechOps & Support Manager", []string{"MongoDB", "Kubernetes", "Linux"}, true},
		{"Operations Specialist", []string{"SQL", "Excel"}, false}, // SQL alone doesn't count
		{"Accountant", nil, false},
	}
	for _, tt := range tests {
		if got := IsTech(tt.title, tt.skills); got != tt.want {
			t.Errorf("IsTech(%q, %v) = %v, want %v", tt.title, tt.skills, got, tt.want)
		}
	}
}
