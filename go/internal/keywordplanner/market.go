package keywordplanner

import (
	"fmt"
	"sort"
	"strings"
)

// Market is a resolved language + countries + network combination.
type Market struct {
	Label            string            `json:"label"`
	Language         ResolvedLanguage  `json:"language"`
	LanguageInferred bool              `json:"languageInferred,omitempty"`
	Countries        []ResolvedCountry `json:"countries,omitempty"`
	Network          string            `json:"network"`
}

// Targeting converts the market into request targeting.
func (m Market) Targeting() Targeting {
	t := Targeting{Language: m.Language.Constant, Network: m.Network}
	for _, c := range m.Countries {
		t.GeoTargets = append(t.GeoTargets, c.Constant)
	}
	return t
}

// ResolveNetwork maps "search", "search_and_partners" and the API enum names to the enum.
// Empty defaults to GOOGLE_SEARCH, the network AFS advertisers bid on.
func ResolveNetwork(s string) (string, error) {
	switch strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), "-", "_")) {
	case "", "SEARCH", "GOOGLE_SEARCH":
		return "GOOGLE_SEARCH", nil
	case "PARTNERS", "SEARCH_AND_PARTNERS", "GOOGLE_SEARCH_AND_PARTNERS":
		return "GOOGLE_SEARCH_AND_PARTNERS", nil
	}
	return "", fmt.Errorf("unknown network %q; use GOOGLE_SEARCH or GOOGLE_SEARCH_AND_PARTNERS", s)
}

// ResolveMarket resolves user input into a Market. When language is empty and countries are
// given, the language is inferred from the first country. Notes explain any inference.
func ResolveMarket(label, language string, countryInputs []string, network string) (Market, []string, error) {
	var notes []string
	lang, err := ResolveLanguage(language)
	if err != nil {
		return Market{}, nil, err
	}
	cs, err := ResolveCountries(countryInputs)
	if err != nil {
		return Market{}, nil, err
	}
	nw, err := ResolveNetwork(network)
	if err != nil {
		return Market{}, nil, err
	}
	m := Market{Language: lang, Countries: cs, Network: nw}
	if lang.Constant == "" && len(cs) > 0 && cs[0].DefaultLanguage != "" {
		inferred, _ := LanguageByCode(cs[0].DefaultLanguage)
		m.Language = ResolvedLanguage{Code: inferred.Code, Constant: inferred.Constant}
		m.LanguageInferred = true
		langs := map[string]bool{}
		for _, c := range cs {
			if c.DefaultLanguage != "" {
				langs[c.DefaultLanguage] = true
			}
		}
		if len(langs) > 1 {
			all := make([]string, 0, len(langs))
			for l := range langs {
				all = append(all, l)
			}
			sort.Strings(all)
			notes = append(notes, fmt.Sprintf("countries use different languages (%s); language %q was inferred from %s. Pass language, or per_country=true for per-country languages.",
				strings.Join(all, ", "), inferred.Code, cs[0].Code))
		}
	}
	m.Label = label
	if m.Label == "" {
		m.Label = defaultLabel(m)
	}
	return m, notes, nil
}

// PerCountry splits the market into one market per country. Unless keepLanguage is set,
// each country gets its own default language.
func (m Market) PerCountry(keepLanguage bool) []Market {
	out := make([]Market, 0, len(m.Countries))
	for _, c := range m.Countries {
		pm := Market{Language: m.Language, LanguageInferred: m.LanguageInferred, Countries: []ResolvedCountry{c}, Network: m.Network}
		if !keepLanguage && c.DefaultLanguage != "" {
			l, _ := LanguageByCode(c.DefaultLanguage)
			pm.Language = ResolvedLanguage{Code: l.Code, Constant: l.Constant}
			pm.LanguageInferred = true
		}
		pm.Label = c.Code
		if pm.Label == "" {
			pm.Label = c.Constant
		}
		out = append(out, pm)
	}
	return out
}

func defaultLabel(m Market) string {
	if len(m.Countries) == 0 {
		if m.Language.Code != "" {
			return "all countries · " + m.Language.Code
		}
		return "worldwide"
	}
	codes := make([]string, 0, len(m.Countries))
	for _, c := range m.Countries {
		if c.Code != "" {
			codes = append(codes, c.Code)
		} else {
			codes = append(codes, c.Constant)
		}
	}
	return strings.Join(codes, "+")
}

// DedupeKey normalizes a keyword for look-alike detection: accent-folded, lowercase,
// with simple plural "s" endings removed.
func DedupeKey(text string) string {
	tokens := strings.Fields(Normalize(text))
	for i, t := range tokens {
		tokens[i] = stem(t)
	}
	return strings.Join(tokens, " ")
}

func stem(t string) string {
	if len(t) > 4 && strings.HasSuffix(t, "s") && !strings.HasSuffix(t, "ss") {
		return t[:len(t)-1]
	}
	return t
}

// Dedupe collapses look-alike rows, keeping the best row and listing the others as close
// variants. Order is preserved. Two rows are look-alikes when their texts normalize to the
// same key (plural, accent and punctuation twins), or when they carry an identical metric
// fingerprint: Google reports exactly the same numbers for variants it treats as one search
// ("kosten treppenlift", "kosten für einen treppenlift").
func Dedupe(rows []KeywordRow) []KeywordRow {
	index := map[string]int{}
	out := make([]KeywordRow, 0, len(rows))
	for _, r := range rows {
		keys := []string{"t:" + DedupeKey(r.Text)}
		if fp := fingerprint(r); fp != "" {
			keys = append(keys, fp)
		}
		i, found := -1, false
		for _, k := range keys {
			if j, ok := index[k]; ok {
				i, found = j, true
				break
			}
		}
		if !found {
			for _, k := range keys {
				index[k] = len(out)
			}
			out = append(out, r)
			continue
		}
		for _, k := range keys {
			index[k] = i
		}
		kept := &out[i]
		if better(r, *kept) {
			r.CloseVariants = uniqueSorted(append(append(r.CloseVariants, kept.CloseVariants...), kept.Text))
			r.IsSeed = r.IsSeed || kept.IsSeed
			*kept = r
		} else {
			kept.CloseVariants = uniqueSorted(append(kept.CloseVariants, r.Text))
			kept.IsSeed = kept.IsSeed || r.IsSeed
		}
	}
	return out
}

func fingerprint(r KeywordRow) string {
	if r.AvgMonthlySearches == 0 || r.HighTopOfPageBidMicros == 0 {
		return ""
	}
	return fmt.Sprintf("m:%d/%d/%d/%d/%.2f", r.AvgMonthlySearches, r.LowTopOfPageBidMicros, r.HighTopOfPageBidMicros, r.CompetitionIndex, r.AverageCpc)
}

// better prefers the higher score, then more searches. On a tie the row seen first wins:
// Google lists the canonical phrasing of a grouped search before odd spellings ("walkin").
func better(a, b KeywordRow) bool {
	if a.AfsScore != b.AfsScore {
		return a.AfsScore > b.AfsScore
	}
	return a.AvgMonthlySearches > b.AvgMonthlySearches
}

// SharesTopic reports whether candidate is about topic: every significant topic word
// (4+ letters) must appear in the candidate, also inside compounds ("treppenliftkosten").
func SharesTopic(candidate, topic string) bool {
	cand := Normalize(candidate)
	var sig []string
	for _, t := range strings.Fields(Normalize(topic)) {
		if len(t) >= 4 {
			sig = append(sig, stem(t))
		}
	}
	if len(sig) == 0 {
		return strings.Contains(cand, Normalize(topic))
	}
	joined := strings.ReplaceAll(cand, " ", "") // "stair lift" still matches "stairlift"
	for _, s := range sig {
		if !strings.Contains(cand, s) && !strings.Contains(joined, s) {
			return false
		}
	}
	return true
}
