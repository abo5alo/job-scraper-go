package job

import "regexp"

// nonTechTitle matches titles of roles that aren't tech jobs even when they
// contain a tech-sounding word: a "Sales Engineer" sells, a "Site Engineer"
// works on construction sites, a "Graphic Designer" isn't a UX designer.
// It's checked first, so it wins over techTitle.
var nonTechTitle = regexp.MustCompile(`(?i)\b(` +
	`sales|pre-?sales|account (manager|executive)|business development|marketing|brand|` +
	`recruit\w*|talent acquisition|hr|human resources|payroll|` +
	`accountant|accounting|finance|financial|audit\w*|tax|` +
	`teacher|tutor|lecturer|instructor|nurse|pharmac\w*|doctor|physician|medical|clinical|dental|` +
	// "site engineer", not just "site": Site Reliability Engineers are tech.
	`civil|mechanical|electrical|structural|mep|hvac|site (engineer|manager|supervisor)|construction|architectural|interior|bim|` +
	`maintenance|production|manufacturing|plant|quality control|qc|` +
	`procurement|purchasing|logistics|supply chain|warehouse|driver|` +
	`real estate|property|legal|lawyer|chef|cook|cashier|` +
	`customer service|call cent(er|re)|telesales|appointment setter|graphic|fashion` +
	`)\b`)

// techTitle matches titles that are tech jobs on their own.
var techTitle = regexp.MustCompile(`(?i)\b(` +
	`software|developer|programmer|devops|devsecops|sre|site reliability|` +
	`full[ -]?stack|front[ -]?end|back[ -]?end|mobile (app|engineer)|ios|android|web (developer|engineer)|` +
	`data (engineer|scientist|analyst|architect|analytics)|business intelligence|bi (developer|analyst|engineer)|` +
	`machine learning|ml|ai (engineer|developer|researcher|scientist)|deep learning|computer vision|nlp|llms?|` +
	`cloud|platform engineer|infrastructure engineer|cyber ?security|security (engineer|analyst|architect)|soc|penetration|` +
	`qa|test automation|software test\w*|tester|` +
	`it (support|specialist|manager|engineer|administrator|officer|technician)|help ?desk|` +
	`systems? (administrator|engineer)|sysadmin|network (engineer|administrator)|database|dba|` +
	`ux|ui|product designer|product manager|product owner|scrum master|` +
	`solutions? architect|technical lead|tech lead|cto|engineering manager|` +
	`web scraping|sap|erp|servicenow|itsm|dlp|data cent(er|re)|product lead|` +
	`integration (engineer|developer)|blockchain|game developer|embedded|firmware|` +
	// Some postings are in French: informatique is IT, logiciel is software.
	`informatique|logiciel|d[ée]veloppeu(r|se)` +
	`)\b`)

// itTitle matches "IT" as in "IT Release Manager". It's case-sensitive,
// since lowercase "it" is an ordinary word.
var itTitle = regexp.MustCompile(`\bIT\b`)

// technicalSkill reports whether a skill signals a technical role. Business
// tools and spoken languages don't, and neither does SQL on its own: sales
// and finance analysts use it too.
func technicalSkill(name string) bool {
	s, ok := LookupSkill(name)
	if !ok || s.Name == "SQL" {
		return false
	}
	switch s.Category {
	case CategoryLanguage, CategoryFramework, CategoryCloud:
		return true
	}
	return s.Name == "Machine learning" || s.Name == "LLMs"
}

// IsTech decides whether a job is a tech job. The title decides when it's
// clear either way. Vague titles like "Specialist" or "Consultant" fall back
// to the skills the job asks for: two technical ones make it a tech job.
func IsTech(title string, skills []string) bool {
	if nonTechTitle.MatchString(title) {
		return false
	}
	if techTitle.MatchString(title) || itTitle.MatchString(title) {
		return true
	}
	n := 0
	for _, s := range skills {
		if technicalSkill(s) {
			n++
		}
	}
	return n >= 2
}
