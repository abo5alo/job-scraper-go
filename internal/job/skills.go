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
	// keywords are lowercase words the pattern can't match without: at
	// least one of them is in any text it matches. Checking for them first
	// is much cheaper than running the pattern, which scans the whole
	// description even when the skill isn't there, and most skills aren't.
	keywords []string
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
	{"Python", CategoryLanguage, regexp.MustCompile(`(?i)\bpython\b`), []string{"python"}},
	{"Java", CategoryLanguage, regexp.MustCompile(`(?i)\bjava\b`), []string{"java"}}, // \b keeps out "JavaScript"
	{"JavaScript", CategoryLanguage, regexp.MustCompile(`(?i)\bjavascript\b`), []string{"javascript"}},
	{"TypeScript", CategoryLanguage, regexp.MustCompile(`(?i)\btypescript\b`), []string{"typescript"}},
	// "Go" is also "Go-Live", "Go-to-Market" and "Go the extra mile". In real
	// postings the language almost always appears in a list ("Python, Go,
	// Rust") or after "in"/"with", so only those count. Go's regexp has no
	// lookahead, so "[^-]|$" is what rules out "Go-Live".
	{"Go", CategoryLanguage, regexp.MustCompile(`(?i:\bgo-?lang\b)|[,/(]\s*Go\b(?:[^-]|$)|\bGo\s*[,/)]|\b(?:in|with|using) Go\b(?:[^-]|$)`), []string{"go"}},
	{"C#", CategoryLanguage, regexp.MustCompile(`(?i)\bc#`), []string{"c#"}},
	{"C++", CategoryLanguage, regexp.MustCompile(`(?i)\bc\+\+`), []string{"c++"}},
	{"PHP", CategoryLanguage, regexp.MustCompile(`(?i)\bphp\b`), []string{"php"}},
	{"Kotlin", CategoryLanguage, regexp.MustCompile(`(?i)\bkotlin\b`), []string{"kotlin"}},
	{"Swift", CategoryLanguage, regexp.MustCompile(`\bSwift(?:UI)?\b`), []string{"swift"}},
	{"Rust", CategoryLanguage, regexp.MustCompile(`\bRust\b`), []string{"rust"}},
	{"SQL", CategoryLanguage, regexp.MustCompile(`(?i)\bsql\b`), []string{"sql"}},

	{"React", CategoryFramework, regexp.MustCompile(`\bReact(?:\.?js| Native)?\b`), []string{"react"}},
	{"Angular", CategoryFramework, regexp.MustCompile(`(?i)\bangular(?:js)?\b`), []string{"angular"}},
	{"Vue", CategoryFramework, regexp.MustCompile(`(?i)\bvue(?:\.?js)?\b`), []string{"vue"}},
	{"Node.js", CategoryFramework, regexp.MustCompile(`(?i)\bnode\.?js\b`), []string{"node"}},
	// The leading [^\w.] keeps out domains like "example.net", so ASP.NET
	// needs its own alternative.
	{".NET", CategoryFramework, regexp.MustCompile(`(?i)(?:^|[^\w.])\.net\b|\basp\.net\b`), []string{".net"}},
	{"Spring", CategoryFramework, regexp.MustCompile(`(?i)\bspring (?:boot|framework)\b`), []string{"spring"}},
	{"Django", CategoryFramework, regexp.MustCompile(`(?i)\bdjango\b`), []string{"django"}},
	{"Laravel", CategoryFramework, regexp.MustCompile(`(?i)\blaravel\b`), []string{"laravel"}},
	{"Flutter", CategoryFramework, regexp.MustCompile(`(?i)\bflutter\b`), []string{"flutter"}},

	{"Machine learning", CategoryData, regexp.MustCompile(`(?i:\bmachine learning\b)|\bML\b`), []string{"machine learning", "ml"}},
	{"LLMs", CategoryData, regexp.MustCompile(`\bLLMs?\b|(?i:\blarge language models?\b)`), []string{"llm", "large language model"}},
	{"Power BI", CategoryData, regexp.MustCompile(`(?i)\bpower ?bi\b`), []string{"power"}},
	{"Tableau", CategoryData, regexp.MustCompile(`(?i)\btableau\b`), []string{"tableau"}},
	{"PostgreSQL", CategoryData, regexp.MustCompile(`(?i)\bpostgres(?:ql)?\b`), []string{"postgres"}},
	{"MongoDB", CategoryData, regexp.MustCompile(`(?i)\bmongo(?:db)?\b`), []string{"mongo"}},

	{"AWS", CategoryCloud, regexp.MustCompile(`(?i)\baws\b|\bamazon web services\b`), []string{"aws", "amazon web services"}},
	{"Azure", CategoryCloud, regexp.MustCompile(`(?i)\bazure\b`), []string{"azure"}},
	{"Google Cloud", CategoryCloud, regexp.MustCompile(`(?i)\bgcp\b|\bgoogle cloud\b`), []string{"gcp", "google cloud"}},
	{"Docker", CategoryCloud, regexp.MustCompile(`(?i)\bdocker\b`), []string{"docker"}},
	{"Kubernetes", CategoryCloud, regexp.MustCompile(`(?i)\bkubernetes\b|\bk8s\b`), []string{"kubernetes", "k8s"}},
	{"Terraform", CategoryCloud, regexp.MustCompile(`(?i)\bterraform\b`), []string{"terraform"}},
	{"Linux", CategoryCloud, regexp.MustCompile(`(?i)\blinux\b`), []string{"linux"}},

	{"Excel", CategoryTool, regexp.MustCompile(`(?i)\bexcel\b`), []string{"excel"}}, // "excel in" is removed first, see below
	{"Microsoft Office", CategoryTool, regexp.MustCompile(`(?i)\b(?:ms|microsoft) office\b`), []string{"office"}},
	{"SAP", CategoryTool, regexp.MustCompile(`(?i)\bsap\b`), []string{"sap"}},
	{"Salesforce", CategoryTool, regexp.MustCompile(`(?i)\bsalesforce\b`), []string{"salesforce"}},
	{"HubSpot", CategoryTool, regexp.MustCompile(`(?i)\bhubspot\b`), []string{"hubspot"}},
	{"Odoo", CategoryTool, regexp.MustCompile(`(?i)\bodoo\b`), []string{"odoo"}},
	{"CRM", CategoryTool, regexp.MustCompile(`(?i)\bcrm\b`), []string{"crm"}},
	{"Figma", CategoryTool, regexp.MustCompile(`(?i)\bfigma\b`), []string{"figma"}},
	{"Adobe Creative Suite", CategoryTool, regexp.MustCompile(`(?i)\bphotoshop\b|\badobe (?:creative|illustrator|indesign|premiere|after effects|xd)\b`), []string{"photoshop", "adobe"}},
	{"AutoCAD", CategoryTool, regexp.MustCompile(`(?i)\bautocad\b`), []string{"autocad"}},
	{"Revit", CategoryTool, regexp.MustCompile(`(?i)\brevit\b`), []string{"revit"}},
	{"Primavera", CategoryTool, regexp.MustCompile(`(?i)\bprimavera\b`), []string{"primavera"}},
	{"SEO", CategoryTool, regexp.MustCompile(`(?i)\bseo\b`), []string{"seo"}},
	{"Google Ads", CategoryTool, regexp.MustCompile(`(?i)\bgoogle ads\b`), []string{"google ads"}},

	{"IFRS", CategoryCert, regexp.MustCompile(`\bIFRS\b`), []string{"ifrs"}},
	{"PMP", CategoryCert, regexp.MustCompile(`\bPMP\b`), []string{"pmp"}},
	{"ACCA", CategoryCert, regexp.MustCompile(`\bACCA\b`), []string{"acca"}},
	{"CFA", CategoryCert, regexp.MustCompile(`\bCFA\b`), []string{"cfa"}},

	{"English", CategorySpoken, regexp.MustCompile(`(?i)\benglish\b`), []string{"english"}},
	{"Arabic", CategorySpoken, regexp.MustCompile(`(?i)\barabic\b`), []string{"arabic"}},
	{"French", CategorySpoken, regexp.MustCompile(`(?i)\bfrench\b`), []string{"french"}},
	{"German", CategorySpoken, regexp.MustCompile(`(?i)\bgerman\b`), []string{"german"}},
}

// misleadingSkillPhrases use a skill's name as an ordinary word: "excel in
// a fast-paced team" isn't asking for spreadsheets. They're removed before
// matching, the same approach as misleadingPhrases in seniority.go.
var misleadingSkillPhrases = regexp.MustCompile(`(?i)\bexcel (?:in|at|as)\b`)

// SkillsFromText returns the names of the skills text mentions, in the
// order of the Skills list, or nil if it mentions none.
func SkillsFromText(text string) []string {
	text = misleadingSkillPhrases.ReplaceAllString(text, "")
	lower := strings.ToLower(text)
	var found []string
	for _, s := range Skills {
		if s.mentionedIn(lower) && s.re.MatchString(text) {
			found = append(found, s.Name)
		}
	}
	return found
}

// mentionedIn reports whether lowercase text contains one of the skill's
// keywords, which the pattern needs in order to match at all.
func (s Skill) mentionedIn(lower string) bool {
	for _, k := range s.keywords {
		if strings.Contains(lower, k) {
			return true
		}
	}
	return false
}

// LookupSkill finds a skill by name, ignoring case: "python" finds "Python".
func LookupSkill(name string) (Skill, bool) {
	name = strings.TrimSpace(name)
	for _, s := range Skills {
		if strings.EqualFold(s.Name, name) {
			return s, true
		}
	}
	return Skill{}, false
}
