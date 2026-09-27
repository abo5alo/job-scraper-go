package job

import (
	"strings"
	"unicode"
)

// countryAliases maps lowercase country names, common abbreviations and major
// cities to ISO 3166-1 alpha-2 codes. Coverage is deliberately focused: the
// Middle East and North Africa in depth, since those are the companies we
// track, plus the countries they most often hire in.
//
// Ambiguous city names are left out on purpose. "Hyderabad" is in both India
// and Pakistan, so guessing would be wrong half the time.
var countryAliases = map[string]string{
	// Gulf
	"united arab emirates": "AE", "uae": "AE", "emirates": "AE", "dubai": "AE",
	"abu dhabi": "AE", "sharjah": "AE", "ajman": "AE", "ras al khaimah": "AE",
	"saudi arabia": "SA", "saudi": "SA", "ksa": "SA", "riyadh": "SA", "jeddah": "SA",
	"jiddah": "SA", "dammam": "SA", "khobar": "SA", "al khobar": "SA", "dhahran": "SA",
	"makkah": "SA", "mecca": "SA", "madinah": "SA", "medina": "SA", "neom": "SA",
	"qatar": "QA", "doha": "QA",
	"kuwait":  "KW",
	"bahrain": "BH", "manama": "BH",
	"oman": "OM", "muscat": "OM",
	// Levant and Iraq
	"jordan": "JO", "amman": "JO",
	"lebanon": "LB", "beirut": "LB",
	"iraq": "IQ", "baghdad": "IQ", "erbil": "IQ",
	"palestine": "PS", "ramallah": "PS",
	// North Africa and Turkey
	"egypt": "EG", "cairo": "EG", "new cairo": "EG", "giza": "EG", "alexandria": "EG",
	"morocco": "MA", "casablanca": "MA", "rabat": "MA",
	"tunisia": "TN", "tunis": "TN",
	"algeria": "DZ", "algiers": "DZ",
	"turkey": "TR", "türkiye": "TR", "turkiye": "TR", "istanbul": "TR",
	// Where regional companies commonly hire
	"pakistan": "PK", "karachi": "PK", "lahore": "PK", "islamabad": "PK",
	"india": "IN", "bangalore": "IN", "bengaluru": "IN", "mumbai": "IN",
	"new delhi": "IN", "delhi": "IN", "pune": "IN", "gurgaon": "IN", "gurugram": "IN",
	"united kingdom": "GB", "uk": "GB", "england": "GB", "london": "GB",
	"germany": "DE", "berlin": "DE", "munich": "DE",
	"netherlands": "NL", "amsterdam": "NL",
	"france": "FR", "paris": "FR",
	"spain": "ES", "madrid": "ES", "barcelona": "ES",
	"portugal": "PT", "lisbon": "PT",
	"ireland": "IE", "dublin": "IE",
	"poland": "PL", "warsaw": "PL",
	"bulgaria": "BG", "sofia": "BG",
	"serbia": "RS", "belgrade": "RS",
	"romania": "RO", "bucharest": "RO",
	"united states": "US", "usa": "US", "new york": "US", "san francisco": "US",
	"canada": "CA", "toronto": "CA", "vancouver": "CA", "montreal": "CA",
}

// knownCodes is the set of codes that appear in countryAliases.
var knownCodes = func() map[string]bool {
	m := make(map[string]bool)
	for _, code := range countryAliases {
		m[code] = true
	}
	return m
}()

// CountryCode converts a structured country value, either a code ("ae",
// "SA") or a name ("Saudi Arabia"), into an uppercase ISO code. It returns
// "" when the value isn't recognized.
func CountryCode(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 2 {
		if code := strings.ToUpper(s); knownCodes[code] {
			return code
		}
		return ""
	}
	return CountryFromText(s)
}

// CountryFromText finds the first country mentioned in free text like
// "Karachi, Pakistan; Lahore, Pakistan" or "Cairo Office". It deliberately
// never matches two-letter codes: in free text "in" is far more likely to be
// the English word than India.
func CountryFromText(text string) string {
	// Lowercase, turn punctuation into spaces, and pad with spaces, so every
	// word can be matched as " word " and "oman" can't match inside "romania".
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	padded := " " + strings.Join(strings.Fields(b.String()), " ") + " "

	// Earliest mention wins; on a tie the longer alias wins ("new delhi"
	// over "delhi"). Map iteration order is random, so ties must be broken
	// explicitly or results would change between runs.
	bestIdx, bestAlias, bestCode := -1, "", ""
	for alias, code := range countryAliases {
		idx := strings.Index(padded, " "+alias+" ")
		if idx < 0 {
			continue
		}
		if bestIdx < 0 || idx < bestIdx || (idx == bestIdx && len(alias) > len(bestAlias)) {
			bestIdx, bestAlias, bestCode = idx, alias, code
		}
	}
	return bestCode
}
