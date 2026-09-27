package job

import (
	"regexp"
	"strings"
)

// Skill is something a job asks for, found by a pattern in its text.
type Skill struct {
	Name     string
	Category string
	re       *regexp.Regexp
}

// Skill categories, in the order the list below uses them.
const (
	CategoryLanguage  = "programming language"
	CategoryFramework = "framework"
	CategoryData      = "data and AI"
	CategoryCloud     = "cloud and devops"
	CategoryTool      = "business tool"
	CategoryCert      = "certification"
	CategorySpoken    = "spoken language"
)

// Skills is every skill we detect. The list was chosen by counting mentions
// across real postings: the region hires for far more than software, so
// business tools, certifications and spoken languages sit next to
// programming languages.
//
// Most patterns ignore case. The ones that don't are words with an everyday
// meaning, where the capital letter is the only signal: "React" the library
// versus "react quickly", "Swift" versus "swift execution".
var Skills = []Skill{
	{"Python", CategoryLanguage, regexp.MustCompile(`(?i)\bpython\b`)},
	{"Java", CategoryLanguage, regexp.MustCompile(`(?i)\bjava\b`)}, // \b keeps out "JavaScript"
	{"JavaScript", CategoryLanguage, regexp.MustCompile(`(?i)\bjavascript\b`)},
	{"TypeScript", CategoryLanguage, regexp.MustCompile(`(?i)\btypescript\b`)},
	// "Go" is also "Go-Live", "Go-to-Market" and "Go the extra mile". In real
	// postings the language almost always appears in a list ("Python, Go,
	// Rust") or after "in"/"with", so only those count. Go's regexp has no
	// lookahead, so "[^-]|$" is what rules out "Go-Live".
	{"Go", CategoryLanguage, regexp.MustCompile(`(?i:\bgo-?lang\b)|[,/(]\s*Go\b(?:[^-]|$)|\bGo\s*[,/)]|\b(?:in|with|using) Go\b(?:[^-]|$)`)},
	{"C#", CategoryLanguage, regexp.MustCompile(`(?i)\bc#`)},
	{"C++", CategoryLanguage, regexp.MustCompile(`(?i)\bc\+\+`)},
	{"PHP", CategoryLanguage, regexp.MustCompile(`(?i)\bphp\b`)},
	{"Kotlin", CategoryLanguage, regexp.MustCompile(`(?i)\bkotlin\b`)},
	{"Swift", CategoryLanguage, regexp.MustCompile(`\bSwift(?:UI)?\b`)},
	{"Rust", CategoryLanguage, regexp.MustCompile(`\bRust\b`)},
	{"SQL", CategoryLanguage, regexp.MustCompile(`(?i)\bsql\b`)},

	{"React", CategoryFramework, regexp.MustCompile(`\bReact(?:\.?js| Native)?\b`)},
	{"Angular", CategoryFramework, regexp.MustCompile(`(?i)\bangular(?:js)?\b`)},
	{"Vue", CategoryFramework, regexp.MustCompile(`(?i)\bvue(?:\.?js)?\b`)},
	{"Node.js", CategoryFramework, regexp.MustCompile(`(?i)\bnode\.?js\b`)},
	// The leading [^\w.] keeps out domains like "example.net", so ASP.NET
	// needs its own alternative.
	{".NET", CategoryFramework, regexp.MustCompile(`(?i)(?:^|[^\w.])\.net\b|\basp\.net\b`)},
	{"Spring", CategoryFramework, regexp.MustCompile(`(?i)\bspring (?:boot|framework)\b`)},
	{"Django", CategoryFramework, regexp.MustCompile(`(?i)\bdjango\b`)},
	{"Laravel", CategoryFramework, regexp.MustCompile(`(?i)\blaravel\b`)},
	{"Flutter", CategoryFramework, regexp.MustCompile(`(?i)\bflutter\b`)},

	{"Machine learning", CategoryData, regexp.MustCompile(`(?i:\bmachine learning\b)|\bML\b`)},
	{"LLMs", CategoryData, regexp.MustCompile(`\bLLMs?\b|(?i:\blarge language models?\b)`)},
	{"Power BI", CategoryData, regexp.MustCompile(`(?i)\bpower ?bi\b`)},
	{"Tableau", CategoryData, regexp.MustCompile(`(?i)\btableau\b`)},
	{"PostgreSQL", CategoryData, regexp.MustCompile(`(?i)\bpostgres(?:ql)?\b`)},
	{"MongoDB", CategoryData, regexp.MustCompile(`(?i)\bmongo(?:db)?\b`)},

	{"AWS", CategoryCloud, regexp.MustCompile(`(?i)\baws\b|\bamazon web services\b`)},
	{"Azure", CategoryCloud, regexp.MustCompile(`(?i)\bazure\b`)},
	{"Google Cloud", CategoryCloud, regexp.MustCompile(`(?i)\bgcp\b|\bgoogle cloud\b`)},
	{"Docker", CategoryCloud, regexp.MustCompile(`(?i)\bdocker\b`)},
	{"Kubernetes", CategoryCloud, regexp.MustCompile(`(?i)\bkubernetes\b|\bk8s\b`)},
	{"Terraform", CategoryCloud, regexp.MustCompile(`(?i)\bterraform\b`)},
	{"Linux", CategoryCloud, regexp.MustCompile(`(?i)\blinux\b`)},

	{"Excel", CategoryTool, regexp.MustCompile(`(?i)\bexcel\b`)}, // "excel in" is removed first, see below
	{"Microsoft Office", CategoryTool, regexp.MustCompile(`(?i)\b(?:ms|microsoft) office\b`)},
	{"SAP", CategoryTool, regexp.MustCompile(`(?i)\bsap\b`)},
	{"Salesforce", CategoryTool, regexp.MustCompile(`(?i)\bsalesforce\b`)},
	{"HubSpot", CategoryTool, regexp.MustCompile(`(?i)\bhubspot\b`)},
	{"Odoo", CategoryTool, regexp.MustCompile(`(?i)\bodoo\b`)},
	{"CRM", CategoryTool, regexp.MustCompile(`(?i)\bcrm\b`)},
	{"Figma", CategoryTool, regexp.MustCompile(`(?i)\bfigma\b`)},
	{"Adobe Creative Suite", CategoryTool, regexp.MustCompile(`(?i)\bphotoshop\b|\badobe (?:creative|illustrator|indesign|premiere|after effects|xd)\b`)},
	{"AutoCAD", CategoryTool, regexp.MustCompile(`(?i)\bautocad\b`)},
	{"Revit", CategoryTool, regexp.MustCompile(`(?i)\brevit\b`)},
	{"Primavera", CategoryTool, regexp.MustCompile(`(?i)\bprimavera\b`)},
	{"SEO", CategoryTool, regexp.MustCompile(`(?i)\bseo\b`)},
	{"Google Ads", CategoryTool, regexp.MustCompile(`(?i)\bgoogle ads\b`)},

	{"IFRS", CategoryCert, regexp.MustCompile(`\bIFRS\b`)},
	{"PMP", CategoryCert, regexp.MustCompile(`\bPMP\b`)},
	{"ACCA", CategoryCert, regexp.MustCompile(`\bACCA\b`)},
	{"CFA", CategoryCert, regexp.MustCompile(`\bCFA\b`)},

	{"English", CategorySpoken, regexp.MustCompile(`(?i)\benglish\b`)},
	{"Arabic", CategorySpoken, regexp.MustCompile(`(?i)\barabic\b`)},
	{"French", CategorySpoken, regexp.MustCompile(`(?i)\bfrench\b`)},
	{"German", CategorySpoken, regexp.MustCompile(`(?i)\bgerman\b`)},
}

// misleadingSkillPhrases use a skill's name as an ordinary word: "excel in
// a fast-paced team" isn't asking for spreadsheets. They're removed before
// matching, the same approach as misleadingPhrases in seniority.go.
var misleadingSkillPhrases = regexp.MustCompile(`(?i)\bexcel (?:in|at|as)\b`)

// SkillsFromText returns the names of the skills text mentions, in the
// order of the Skills list, or nil if it mentions none.
func SkillsFromText(text string) []string {
	text = misleadingSkillPhrases.ReplaceAllString(text, "")
	var found []string
	for _, s := range Skills {
		if s.re.MatchString(text) {
			found = append(found, s.Name)
		}
	}
	return found
}

// LookupSkill finds a skill by name, ignoring case: "python" finds "Python".
func LookupSkill(name string) (Skill, bool) {
	for _, s := range Skills {
		if strings.EqualFold(s.Name, strings.TrimSpace(name)) {
			return s, true
		}
	}
	return Skill{}, false
}
