package job

import (
	"regexp"
	"strings"
)

type Seniority string

const (
	SeniorityIntern    Seniority = "intern"
	SeniorityJunior    Seniority = "junior"
	SeniorityMid       Seniority = "mid"
	SenioritySenior    Seniority = "senior"
	SeniorityLead      Seniority = "lead"
	SeniorityExecutive Seniority = "executive"
)

// Seniorities lists every level, from least to most senior.
var Seniorities = []Seniority{
	SeniorityIntern, SeniorityJunior, SeniorityMid,
	SenioritySenior, SeniorityLead, SeniorityExecutive,
}

func ParseSeniority(s string) (Seniority, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, level := range Seniorities {
		if string(level) == s {
			return level, true
		}
	}
	return "", false
}

// Rules are checked in order and the first match wins, so the order encodes
// precedence: "Senior Director" is executive, not senior, and "Senior
// Intern" is still an intern.
var seniorityRules = []struct {
	level Seniority
	re    *regexp.Regexp
}{
	{SeniorityIntern, regexp.MustCompile(`(?i)\b(interns?|internships?|trainee|working student)\b`)},
	{SeniorityExecutive, regexp.MustCompile(`(?i)\b(chief|c[etofi]o|vp|svp|evp|vice president|director|head of)\b`)},
	// "Staff" only means lead for engineering-type roles: a "Staff
	// Accountant" is an entry-level accounting job.
	{SeniorityLead, regexp.MustCompile(`(?i)\b(lead|principal|staff (\w+ ){0,2}(engineer|scientist|developer|designer|architect))\b`)},
	{SenioritySenior, regexp.MustCompile(`(?i)\b(senior|sr)\b`)},
	// Tamheer is Saudi Arabia's on-the-job training program for new graduates.
	{SeniorityJunior, regexp.MustCompile(`(?i)\b(junior|jr|entry[ -]level|graduate|new grad|tamheer)\b`)},
}

// Phrases that contain a level keyword without describing the job's level:
// a "Lead Generation Specialist" is a sales role, and a "CEO Office Manager"
// works in the CEO's office rather than being the CEO. They're removed
// before the rules run.
var misleadingPhrases = regexp.MustCompile(`(?i)\b(leads? gen(eration)?|c[etofi]o office|office of the (c[etofi]o|chief \w+ officer))\b`)

// SeniorityFromTitle detects a level from keywords in a job title. It
// returns "" when the title has no level keyword, so callers can tell
// "no signal" apart from an actual match.
func SeniorityFromTitle(title string) Seniority {
	title = misleadingPhrases.ReplaceAllString(title, "")
	for _, rule := range seniorityRules {
		if rule.re.MatchString(title) {
			return rule.level
		}
	}
	return ""
}

// SeniorityFromLabel maps an ATS's own experience-level field onto our
// levels. Each system spells labels differently ("Entry level",
// "entry_level"), so separators are normalized first. Vague labels like
// "Mid-Senior level" or "Associate" return "" and the title decides.
func SeniorityFromLabel(label string) Seniority {
	l := strings.NewReplacer("_", " ", "-", " ").Replace(strings.ToLower(strings.TrimSpace(label)))
	switch {
	case l == "internship" || l == "intern" || strings.HasPrefix(l, "student"):
		return SeniorityIntern
	case l == "entry level" || l == "graduate":
		return SeniorityJunior
	case l == "director" || l == "executive":
		return SeniorityExecutive
	}
	return ""
}
