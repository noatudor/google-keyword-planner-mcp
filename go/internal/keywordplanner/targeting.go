package keywordplanner

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Language is a supported Keyword Planner language.
type Language struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Constant string `json:"constant"`
}

// Country is a supported Keyword Planner country target.
type Country struct {
	Code            string `json:"code"`
	Name            string `json:"name"`
	Constant        string `json:"constant"`
	DefaultLanguage string `json:"defaultLanguage"`
}

var languages = []Language{
	{"en", "English", "languageConstants/1000"},
	{"de", "German", "languageConstants/1001"},
	{"fr", "French", "languageConstants/1002"},
	{"es", "Spanish", "languageConstants/1003"},
	{"it", "Italian", "languageConstants/1004"},
	{"ja", "Japanese", "languageConstants/1005"},
	{"da", "Danish", "languageConstants/1009"},
	{"nl", "Dutch", "languageConstants/1010"},
	{"fi", "Finnish", "languageConstants/1011"},
	{"ko", "Korean", "languageConstants/1012"},
	{"no", "Norwegian", "languageConstants/1013"},
	{"pt", "Portuguese", "languageConstants/1014"},
	{"sv", "Swedish", "languageConstants/1015"},
	{"ar", "Arabic", "languageConstants/1019"},
	{"bg", "Bulgarian", "languageConstants/1020"},
	{"cs", "Czech", "languageConstants/1021"},
	{"el", "Greek", "languageConstants/1022"},
	{"hu", "Hungarian", "languageConstants/1024"},
	{"pl", "Polish", "languageConstants/1030"},
	{"ro", "Romanian", "languageConstants/1032"},
	{"sk", "Slovak", "languageConstants/1033"},
	{"sl", "Slovenian", "languageConstants/1034"},
	{"tr", "Turkish", "languageConstants/1037"},
	{"hr", "Croatian", "languageConstants/1039"},
}

// countries use Google's country criterion IDs, which are 2000 + the ISO 3166-1 numeric code.
var countries = []Country{
	{"US", "United States", "geoTargetConstants/2840", "en"},
	{"GB", "United Kingdom", "geoTargetConstants/2826", "en"},
	{"CA", "Canada", "geoTargetConstants/2124", "en"},
	{"AU", "Australia", "geoTargetConstants/2036", "en"},
	{"NZ", "New Zealand", "geoTargetConstants/2554", "en"},
	{"IE", "Ireland", "geoTargetConstants/2372", "en"},
	{"ZA", "South Africa", "geoTargetConstants/2710", "en"},
	{"IN", "India", "geoTargetConstants/2356", "en"},
	{"SG", "Singapore", "geoTargetConstants/2702", "en"},
	{"DE", "Germany", "geoTargetConstants/2276", "de"},
	{"AT", "Austria", "geoTargetConstants/2040", "de"},
	{"CH", "Switzerland", "geoTargetConstants/2756", "de"},
	{"FR", "France", "geoTargetConstants/2250", "fr"},
	{"BE", "Belgium", "geoTargetConstants/2056", "nl"},
	{"LU", "Luxembourg", "geoTargetConstants/2442", "fr"},
	{"NL", "Netherlands", "geoTargetConstants/2528", "nl"},
	{"ES", "Spain", "geoTargetConstants/2724", "es"},
	{"PT", "Portugal", "geoTargetConstants/2620", "pt"},
	{"IT", "Italy", "geoTargetConstants/2380", "it"},
	{"PL", "Poland", "geoTargetConstants/2616", "pl"},
	{"SE", "Sweden", "geoTargetConstants/2752", "sv"},
	{"DK", "Denmark", "geoTargetConstants/2208", "da"},
	{"NO", "Norway", "geoTargetConstants/2578", "no"},
	{"FI", "Finland", "geoTargetConstants/2246", "fi"},
	{"CZ", "Czechia", "geoTargetConstants/2203", "cs"},
	{"SK", "Slovakia", "geoTargetConstants/2703", "sk"},
	{"HU", "Hungary", "geoTargetConstants/2348", "hu"},
	{"RO", "Romania", "geoTargetConstants/2642", "ro"},
	{"BG", "Bulgaria", "geoTargetConstants/2100", "bg"},
	{"GR", "Greece", "geoTargetConstants/2300", "el"},
	{"HR", "Croatia", "geoTargetConstants/2191", "hr"},
	{"SI", "Slovenia", "geoTargetConstants/2705", "sl"},
	{"TR", "Turkey", "geoTargetConstants/2792", "tr"},
	{"MX", "Mexico", "geoTargetConstants/2484", "es"},
	{"AR", "Argentina", "geoTargetConstants/2032", "es"},
	{"CL", "Chile", "geoTargetConstants/2152", "es"},
	{"CO", "Colombia", "geoTargetConstants/2170", "es"},
	{"PE", "Peru", "geoTargetConstants/2604", "es"},
	{"BR", "Brazil", "geoTargetConstants/2076", "pt"},
	{"JP", "Japan", "geoTargetConstants/2392", "ja"},
	{"KR", "South Korea", "geoTargetConstants/2410", "ko"},
	{"AE", "United Arab Emirates", "geoTargetConstants/2784", "ar"},
	{"SA", "Saudi Arabia", "geoTargetConstants/2682", "ar"},
}

// CountryPresets expand to several countries, e.g. "DACH" → DE, AT, CH.
var CountryPresets = map[string][]string{
	"DACH":     {"DE", "AT", "CH"},
	"BENELUX":  {"BE", "NL", "LU"},
	"NORDICS":  {"SE", "DK", "NO", "FI"},
	"IBERIA":   {"ES", "PT"},
	"EN_CORE":  {"US", "GB", "CA", "AU", "IE", "NZ"},
	"LATAM_ES": {"MX", "AR", "CL", "CO", "PE"},
}

// Languages returns the supported languages.
func Languages() []Language { return append([]Language(nil), languages...) }

// Countries returns the supported countries.
func Countries() []Country { return append([]Country(nil), countries...) }

// ResolvedLanguage is a resolved language: Code may be empty when a raw constant was passed.
type ResolvedLanguage struct {
	Code     string `json:"code,omitempty"`
	Constant string `json:"constant"`
}

// ResolveLanguage accepts an ISO code ("de"), an English name ("German") or a resource name
// ("languageConstants/1001", or just "1001"). Empty input resolves to an empty language.
func ResolveLanguage(input string) (ResolvedLanguage, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return ResolvedLanguage{}, nil
	}
	if strings.HasPrefix(s, "languageConstants/") || isDigits(s) {
		constant := s
		if isDigits(s) {
			constant = "languageConstants/" + s
		}
		for _, l := range languages {
			if l.Constant == constant {
				return ResolvedLanguage{Code: l.Code, Constant: constant}, nil
			}
		}
		return ResolvedLanguage{Constant: constant}, nil
	}
	lower := strings.ToLower(strings.ReplaceAll(s, "_", "-"))
	if i := strings.Index(lower, "-"); i > 0 { // "de-DE", "de_DE" → "de"
		lower = lower[:i]
	}
	for _, l := range languages {
		if l.Code == lower || strings.EqualFold(l.Name, s) {
			return ResolvedLanguage{Code: l.Code, Constant: l.Constant}, nil
		}
	}
	codes := make([]string, 0, len(languages))
	for _, l := range languages {
		codes = append(codes, l.Code)
	}
	return ResolvedLanguage{}, fmt.Errorf("unknown language %q; use an ISO code (%s) or a languageConstants/<id> resource name", input, strings.Join(codes, ", "))
}

// ResolvedCountry is a resolved country: Code may be empty when a raw constant was passed.
type ResolvedCountry struct {
	Code            string `json:"code,omitempty"`
	Constant        string `json:"constant"`
	DefaultLanguage string `json:"defaultLanguage,omitempty"`
}

// ResolveCountries accepts ISO alpha-2 codes ("DE"), English names ("Germany"), presets ("DACH")
// and resource names ("geoTargetConstants/2276", which may also be a region or city).
// Duplicates are removed and input order is preserved.
func ResolveCountries(inputs []string) ([]ResolvedCountry, error) {
	var out []ResolvedCountry
	seen := map[string]bool{}
	add := func(rc ResolvedCountry) {
		if !seen[rc.Constant] {
			seen[rc.Constant] = true
			out = append(out, rc)
		}
	}
	for _, raw := range inputs {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		upper := strings.ToUpper(strings.TrimPrefix(strings.ToUpper(s), "ALL_"))
		if members, ok := CountryPresets[upper]; ok {
			for _, m := range members {
				c, _ := lookupCountry(m)
				add(c)
			}
			continue
		}
		if strings.HasPrefix(s, "geoTargetConstants/") || isDigits(s) {
			constant := s
			if isDigits(s) {
				constant = "geoTargetConstants/" + s
			}
			rc := ResolvedCountry{Constant: constant}
			for _, c := range countries {
				if c.Constant == constant {
					rc = ResolvedCountry{Code: c.Code, Constant: c.Constant, DefaultLanguage: c.DefaultLanguage}
				}
			}
			add(rc)
			continue
		}
		c, ok := lookupCountry(s)
		if !ok {
			codes := make([]string, 0, len(countries))
			for _, c := range countries {
				codes = append(codes, c.Code)
			}
			presets := make([]string, 0, len(CountryPresets))
			for p := range CountryPresets {
				presets = append(presets, p)
			}
			sort.Strings(presets)
			return nil, fmt.Errorf("unknown country %q; use an ISO alpha-2 code (%s), a preset (%s) or a geoTargetConstants/<id> resource name",
				raw, strings.Join(codes, ", "), strings.Join(presets, ", "))
		}
		add(c)
	}
	return out, nil
}

func lookupCountry(s string) (ResolvedCountry, bool) {
	upper := strings.ToUpper(s)
	if upper == "UK" {
		upper = "GB"
	}
	for _, c := range countries {
		if c.Code == upper || strings.EqualFold(c.Name, s) {
			return ResolvedCountry{Code: c.Code, Constant: c.Constant, DefaultLanguage: c.DefaultLanguage}, true
		}
	}
	return ResolvedCountry{}, false
}

// LanguageByCode returns the language for an ISO code.
func LanguageByCode(code string) (Language, bool) {
	for _, l := range languages {
		if l.Code == code {
			return l, true
		}
	}
	return Language{}, false
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	_, err := strconv.ParseUint(s, 10, 64)
	return err == nil
}
