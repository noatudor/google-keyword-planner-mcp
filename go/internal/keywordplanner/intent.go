package keywordplanner

import (
	"sort"
	"strings"
	"unicode"
)

// Commercial intent classes, strongest AFS value first.
const (
	IntentTransactional = "transactional"
	IntentCommercial    = "commercial"
	IntentInformational = "informational"
	IntentNavigational  = "navigational"
	IntentUnclassified  = "unclassified"
)

// Lexicon holds the intent modifiers and offerName variant templates for one language.
// Templates use {t} for the topic, e.g. "{t} kosten".
type Lexicon struct {
	Transactional []string `json:"transactional"`
	Commercial    []string `json:"commercial"`
	Informational []string `json:"informational"`
	Templates     []string `json:"variantTemplates"`
}

// Lexicons are keyed by ISO language code. English is also checked for every language,
// because English loanwords ("leasing", "online") are common in all our markets.
//
//nolint:misspell // Foreign-language modifier words, not English misspellings.
var Lexicons = map[string]Lexicon{
	"en": {
		Transactional: []string{"buy", "price", "prices", "pricing", "cost", "costs", "cheap", "affordable", "financing", "finance",
			"lease", "leasing", "rent", "rental", "rentals", "hire", "rent to own", "near me", "quote", "quotes", "for sale",
			"deal", "deals", "discount", "order", "install", "installation", "installer", "repair", "replacement", "service",
			"services", "company", "companies", "contractor", "contractors", "supplier", "suppliers", "provider", "providers",
			"used", "refurbished", "wholesale", "monthly payments", "payment plan", "same day", "estimate", "book"},
		Commercial: []string{"best", "top", "compare", "comparison", "vs", "versus", "review", "reviews", "rated", "insurance",
			"grant", "grants", "rebate", "rebates", "subsidy", "programs", "program", "options", "plans", "for seniors",
			"for elderly", "for small business", "types of", "alternatives", "worth it", "how much", "value", "certified", "licensed"},
		Informational: []string{"what is", "what are", "how to", "how does", "how do", "why", "meaning", "definition", "history",
			"wiki", "facts", "diy", "tutorial", "pdf", "free", "jobs", "job", "salary", "lyrics", "game", "games", "download",
			"images", "pictures", "video", "videos", "clipart", "drawing", "coloring", "quiz", "reddit", "news"},
		Templates: []string{"{t} cost", "{t} price", "{t} financing", "{t} rental", "rent to own {t}", "{t} near me",
			"{t} for seniors", "{t} grant", "{t} insurance", "best {t}", "{t} deals", "{t} lease", "used {t}",
			"{t} installation", "{t} companies", "{t} quote", "cheap {t}", "{t} for sale", "{t} monthly payments", "{t} programs"},
	},
	"de": {
		Transactional: []string{"kaufen", "kosten", "preis", "preise", "günstig", "billig", "finanzieren", "finanzierung", "kredit",
			"leasing", "mieten", "miete", "mietkauf", "in der nähe", "angebot", "angebote", "anbieter", "firma", "firmen",
			"installation", "einbau", "einbauen", "montage", "reparatur", "austausch", "gebraucht", "bestellen", "händler",
			"ratenzahlung", "raten", "online kaufen", "service", "handwerker", "fachbetrieb", "sanierung"},
		Commercial: []string{"vergleich", "test", "testsieger", "beste", "besten", "erfahrungen", "bewertung", "versicherung",
			"förderung", "zuschuss", "zuschüsse", "fördermittel", "kfw", "bafa", "für senioren", "rechner", "tarif", "tarife",
			"lohnt sich", "was kostet", "alternativen"},
		Informational: []string{"was ist", "wie", "warum", "bedeutung", "definition", "geschichte", "selber bauen", "selbst bauen",
			"anleitung", "kostenlos", "gratis", "jobs", "gehalt", "stellenangebote", "ausbildung", "wiki", "bilder", "video"},
		Templates: []string{"{t} kosten", "{t} preis", "{t} finanzieren", "{t} mieten", "{t} leasing", "{t} förderung",
			"{t} kaufen", "{t} gebraucht", "{t} vergleich", "{t} angebot", "{t} in der nähe", "{t} test", "{t} anbieter",
			"{t} günstig", "{t} einbau", "{t} für senioren", "{t} zuschuss", "{t} ratenzahlung"},
	},
	"fr": {
		Transactional: []string{"acheter", "achat", "prix", "coût", "cout", "tarif", "tarifs", "pas cher", "financement", "crédit",
			"leasing", "location", "louer", "location avec option d'achat", "près de chez moi", "devis", "promo", "promotion",
			"installation", "installateur", "réparation", "remplacement", "occasion", "vente", "entreprise", "artisan"},
		Commercial: []string{"comparatif", "comparateur", "meilleur", "meilleure", "meilleurs", "avis", "test", "assurance", "aide",
			"aides", "subvention", "prime", "maprimerenov", "pour seniors", "pour personnes âgées", "simulation", "rentable"},
		Informational: []string{"qu'est-ce", "qu est ce", "comment", "pourquoi", "définition", "histoire", "gratuit", "emploi",
			"salaire", "tuto", "wiki", "images", "vidéo"},
		Templates: []string{"{t} prix", "{t} coût", "{t} financement", "location {t}", "{t} occasion", "{t} devis",
			"meilleur {t}", "{t} aide", "{t} subvention", "{t} pas cher", "{t} installation", "{t} pour seniors",
			"{t} comparatif", "{t} assurance"},
	},
	"es": {
		Transactional: []string{"comprar", "precio", "precios", "coste", "costo", "barato", "barata", "financiación", "financiacion",
			"crédito", "credito", "leasing", "renting", "alquiler", "alquilar", "cerca de mi", "cerca de mí", "presupuesto",
			"oferta", "ofertas", "instalación", "instalacion", "reparación", "reparacion", "segunda mano", "venta", "empresa",
			"empresas", "a plazos"},
		Commercial: []string{"comparativa", "comparar", "mejor", "mejores", "opiniones", "seguro", "seguros", "ayudas", "ayuda",
			"subvención", "subvencion", "para mayores", "para personas mayores", "vale la pena", "cuánto cuesta", "cuanto cuesta"},
		Informational: []string{"qué es", "que es", "cómo", "como", "por qué", "porque", "significado", "definición", "historia",
			"gratis", "empleo", "trabajo", "sueldo", "salario", "imágenes", "video"},
		Templates: []string{"{t} precio", "{t} financiación", "alquiler de {t}", "{t} barato", "{t} segunda mano",
			"{t} ofertas", "mejor {t}", "{t} ayudas", "{t} subvención", "{t} instalación", "{t} para mayores",
			"{t} cerca de mi", "{t} a plazos", "{t} seguro"},
	},
	"it": {
		Transactional: []string{"comprare", "acquistare", "acquisto", "prezzo", "prezzi", "costo", "costi", "economico", "economici",
			"finanziamento", "leasing", "noleggio", "affitto", "vicino a me", "preventivo", "offerta", "offerte", "installazione",
			"riparazione", "usato", "usati", "vendita", "a rate", "ditta", "azienda"},
		Commercial: []string{"migliore", "migliori", "recensioni", "opinioni", "confronto", "assicurazione", "incentivi", "bonus",
			"detrazione", "per anziani", "conviene", "quanto costa"},
		Informational: []string{"cos'è", "cos e", "cosa è", "come", "perché", "perche", "significato", "definizione", "storia",
			"gratis", "lavoro", "stipendio", "immagini"},
		Templates: []string{"{t} prezzo", "{t} costo", "noleggio {t}", "{t} finanziamento", "{t} usato", "{t} offerte",
			"migliore {t}", "{t} incentivi", "{t} bonus", "{t} installazione", "{t} per anziani", "{t} preventivo", "{t} a rate"},
	},
	"nl": {
		Transactional: []string{"kopen", "prijs", "prijzen", "kosten", "goedkoop", "financiering", "financieren", "lease", "leasen",
			"huren", "huur", "in de buurt", "offerte", "aanbieding", "aanbiedingen", "installatie", "reparatie", "tweedehands",
			"bedrijf", "gespreid betalen"},
		Commercial: []string{"vergelijken", "vergelijking", "beste", "ervaringen", "review", "verzekering", "subsidie", "premie",
			"voor ouderen", "voor senioren", "de moeite waard", "wat kost"},
		Informational: []string{"wat is", "hoe", "waarom", "betekenis", "definitie", "gratis", "vacature", "vacatures", "salaris"},
		Templates: []string{"{t} kosten", "{t} prijs", "{t} huren", "{t} kopen", "{t} lease", "{t} subsidie",
			"{t} tweedehands", "{t} offerte", "beste {t}", "{t} vergelijken", "{t} installatie", "{t} voor ouderen"},
	},
	"pt": {
		Transactional: []string{"comprar", "preço", "preco", "preços", "custo", "barato", "financiamento", "crédito", "credito",
			"leasing", "aluguel", "aluguer", "alugar", "perto de mim", "orçamento", "orcamento", "oferta", "promoção",
			"instalação", "reparação", "conserto", "usado", "usados", "venda", "empresa", "parcelado", "prestações"},
		Commercial: []string{"melhor", "melhores", "comparação", "comparativo", "opiniões", "avaliações", "seguro", "apoio",
			"apoios", "subsídio", "para idosos", "vale a pena", "quanto custa"},
		Informational: []string{"o que é", "o que e", "como", "por que", "porque", "significado", "definição", "grátis", "gratis",
			"emprego", "salário", "imagens"},
		Templates: []string{"{t} preço", "{t} financiamento", "aluguel de {t}", "{t} usado", "{t} barato", "melhor {t}",
			"{t} orçamento", "{t} instalação", "{t} para idosos", "{t} apoio", "{t} seguro", "{t} parcelado"},
	},
	"pl": {
		Transactional: []string{"kupić", "kupic", "cena", "ceny", "koszt", "koszty", "tani", "tanie", "finansowanie", "kredyt",
			"leasing", "wynajem", "najem", "wypożyczalnia", "w pobliżu", "oferta", "montaż", "montaz", "naprawa", "używane",
			"uzywane", "sprzedaż", "firma", "na raty"},
		Commercial: []string{"najlepszy", "najlepsze", "ranking", "opinie", "porównanie", "ubezpieczenie", "dotacja",
			"dofinansowanie", "dla seniorów", "czy warto", "ile kosztuje"},
		Informational: []string{"co to jest", "jak", "dlaczego", "znaczenie", "definicja", "darmowe", "za darmo", "praca", "pensja"},
		Templates: []string{"{t} cena", "{t} koszt", "{t} wynajem", "{t} leasing", "{t} na raty", "{t} dofinansowanie",
			"{t} używane", "najlepszy {t}", "{t} opinie", "{t} montaż", "{t} dla seniorów"},
	},
	"sv": {
		Transactional: []string{"köpa", "kopa", "pris", "priser", "kostnad", "billig", "billiga", "finansiering", "leasing", "hyra",
			"uthyrning", "nära mig", "offert", "installation", "reparation", "begagnad", "företag", "delbetalning"},
		Commercial: []string{"bäst", "bästa", "jämför", "jämförelse", "recension", "omdöme", "försäkring", "bidrag", "rot",
			"för äldre", "lönar sig"},
		Informational: []string{"vad är", "hur", "varför", "betydelse", "gratis", "jobb", "lön"},
		Templates: []string{"{t} pris", "{t} kostnad", "hyra {t}", "{t} leasing", "{t} begagnad", "bästa {t}",
			"{t} bidrag", "{t} installation", "{t} för äldre", "{t} offert"},
	},
	"da": {
		Transactional: []string{"køb", "købe", "pris", "priser", "billig", "billige", "finansiering", "leasing", "leje", "udlejning",
			"nær mig", "tilbud", "installation", "reparation", "brugt", "brugte", "firma", "afbetaling"},
		Commercial:    []string{"bedste", "sammenligning", "test", "anmeldelser", "forsikring", "tilskud", "for ældre", "kan det betale sig"},
		Informational: []string{"hvad er", "hvordan", "hvorfor", "betydning", "gratis", "job", "løn"},
		Templates: []string{"{t} pris", "{t} leje", "{t} leasing", "{t} brugt", "bedste {t}", "{t} tilskud",
			"{t} installation", "{t} tilbud", "{t} for ældre"},
	},
	"no": {
		Transactional: []string{"kjøpe", "kjop", "pris", "priser", "billig", "finansiering", "leasing", "leie", "utleie",
			"nær meg", "tilbud", "installasjon", "reparasjon", "brukt", "firma", "nedbetaling"},
		Commercial: []string{"beste", "sammenligning", "test", "anmeldelser", "forsikring", "tilskudd", "støtte", "enova",
			"for eldre", "lønner seg"},
		Informational: []string{"hva er", "hvordan", "hvorfor", "betydning", "gratis", "jobb", "lønn"},
		Templates: []string{"{t} pris", "leie {t}", "{t} leasing", "{t} brukt", "beste {t}", "{t} tilskudd",
			"{t} installasjon", "{t} tilbud", "{t} for eldre"},
	},
	"fi": {
		Transactional: []string{"osta", "ostaa", "hinta", "hinnat", "halpa", "rahoitus", "leasing", "vuokraus", "vuokra",
			"lähellä", "tarjous", "asennus", "korjaus", "käytetty", "yritys", "osamaksu"},
		Commercial: []string{"paras", "parhaat", "vertailu", "testi", "arvostelu", "vakuutus", "avustus", "tuki",
			"ikäihmisille", "kannattaako"},
		Informational: []string{"mikä on", "miten", "miksi", "merkitys", "ilmainen", "työpaikat", "palkka"},
		Templates: []string{"{t} hinta", "{t} vuokraus", "{t} leasing", "{t} käytetty", "paras {t}", "{t} avustus",
			"{t} asennus", "{t} tarjous"},
	},
}

// Special ad category hints. These are informational only: Adspy assigns the real
// special ad category from offerName, and the agent must never set it.
//
//nolint:misspell // Foreign-language words, not English misspellings.
var sensitiveCategoryWords = map[string][]string{
	"CREDIT": {"loan", "loans", "credit", "credit card", "financing", "mortgage", "refinance", "payday", "debt", "kredit",
		"finanzierung", "finanzieren", "ratenzahlung", "darlehen", "baufinanzierung", "hypothek", "crédit", "financement", "prêt",
		"préstamo", "prestamo", "préstamos", "financiación", "financiacion", "credito", "crédito", "finanziamento", "prestito",
		"mutuo", "lening", "financiering", "hypotheek", "empréstimo", "emprestimo", "financiamento", "kredyt", "pożyczka",
		"pozyczka", "finansowanie", "lån", "finansiering", "a plazos", "a rate", "na raty", "rent to own", "monthly payments"},
	"EMPLOYMENT": {"job", "jobs", "hiring", "career", "careers", "vacancy", "vacancies", "salary", "work from home",
		"stellenangebote", "stellenangebot", "gehalt", "jobs in", "emploi", "offre d'emploi", "empleo", "trabajo", "ofertas de empleo",
		"lavoro", "offerte di lavoro", "vacature", "vacatures", "emprego", "vagas", "praca", "praca w", "jobb", "job offer"},
	"HOUSING": {"apartment for rent", "apartments for rent", "house for rent", "homes for sale", "houses for sale", "real estate",
		"condo", "wohnung mieten", "wohnung kaufen", "haus kaufen", "haus mieten", "immobilien", "mietwohnung", "appartement à louer",
		"maison à vendre", "piso en alquiler", "pisos", "casa en venta", "appartamento in affitto", "case in vendita",
		"huurwoning", "huis kopen", "apartamento para alugar", "casa à venda", "mieszkanie", "mieszkania"},
}

// compoundLanguages glue words together ("treppenliftkosten"), so their modifiers also match as substrings.
var compoundLanguages = map[string]bool{"de": true, "nl": true, "sv": true, "da": true, "no": true, "fi": true}

// IntentResult is the commercial intent classification of one keyword.
type IntentResult struct {
	Intent            string
	Modifiers         []string
	SensitiveCategory string
}

// ClassifyIntent classifies a keyword's commercial intent from its modifier words.
// lang is an ISO code; empty checks every lexicon. isBrand forces navigational.
func ClassifyIntent(text, lang string, isBrand bool) IntentResult {
	norm := Normalize(text)
	lexs := lexiconsFor(lang)
	var trans, comm, info []string
	for _, code := range lexs {
		info = append(info, matchModifiers(norm, Lexicons[code].Informational, false)...)
	}
	// Remove informational words before compound matching, so "kostenlos" (free)
	// does not count as "kosten" (cost).
	commercialText := " " + norm + " "
	for _, m := range info {
		commercialText = strings.ReplaceAll(commercialText, " "+Normalize(m)+" ", " ")
	}
	commercialText = strings.TrimSpace(commercialText)
	for _, code := range lexs {
		// Only the market language matches inside compounds: English "order" must not
		// match German "förderung".
		compound := code == lang && compoundLanguages[lang]
		lx := Lexicons[code]
		trans = append(trans, matchModifiers(commercialText, lx.Transactional, compound)...)
		comm = append(comm, matchModifiers(commercialText, lx.Commercial, compound)...)
	}
	res := IntentResult{SensitiveCategory: SensitiveCategoryHint(text)}
	switch {
	case isBrand:
		res.Intent = IntentNavigational
	case len(trans) > 0:
		res.Intent = IntentTransactional
	case len(comm) > 0:
		res.Intent = IntentCommercial
	case len(info) > 0:
		res.Intent = IntentInformational
	default:
		res.Intent = IntentUnclassified
	}
	res.Modifiers = uniqueSorted(append(append(trans, comm...), info...))
	return res
}

// SensitiveCategoryHint returns CREDIT, EMPLOYMENT or HOUSING when the keyword uses wording
// Meta treats as a special ad category, or "". Informational only.
func SensitiveCategoryHint(text string) string {
	norm := Normalize(text)
	for _, cat := range []string{"CREDIT", "EMPLOYMENT", "HOUSING"} {
		if len(matchModifiers(norm, sensitiveCategoryWords[cat], true)) > 0 {
			return cat
		}
	}
	return ""
}

// VariantTemplates returns the offerName variant templates for a language, falling back to English.
func VariantTemplates(lang string) []string {
	if lx, ok := Lexicons[lang]; ok {
		return lx.Templates
	}
	return Lexicons["en"].Templates
}

// lexiconsFor returns the lexicon codes to check: English plus the market language, or all when unknown.
func lexiconsFor(lang string) []string {
	if lang == "" {
		out := make([]string, 0, len(Lexicons))
		for code := range Lexicons {
			out = append(out, code)
		}
		sort.Strings(out)
		return out
	}
	out := []string{"en"}
	if _, ok := Lexicons[lang]; ok && lang != "en" {
		out = append(out, lang)
	}
	return out
}

// matchModifiers returns the modifiers found in a normalized keyword. Multi-word
// modifiers and single words match on word boundaries; with substr, words of 5+
// letters also match inside compounds.
func matchModifiers(norm string, modifiers []string, substr bool) []string {
	padded := " " + norm + " "
	var hits []string
	for _, m := range modifiers {
		nm := Normalize(m)
		if nm == "" {
			continue
		}
		if strings.Contains(padded, " "+nm+" ") || (substr && len(nm) >= 5 && !strings.Contains(nm, " ") && strings.Contains(norm, nm)) {
			hits = append(hits, m)
		}
	}
	return hits
}

var foldMap = map[rune]string{
	'ä': "a", 'á': "a", 'à': "a", 'â': "a", 'ã': "a", 'å': "a", 'ą': "a",
	'ö': "o", 'ó': "o", 'ò': "o", 'ô': "o", 'õ': "o", 'ø': "o",
	'ü': "u", 'ú': "u", 'ù': "u", 'û': "u",
	'é': "e", 'è': "e", 'ê': "e", 'ë': "e", 'ę': "e",
	'í': "i", 'ì': "i", 'î': "i", 'ï': "i",
	'ç': "c", 'ć': "c", 'č': "c", 'ñ': "n", 'ń': "n", 'ß': "ss", 'ł': "l",
	'ś': "s", 'š': "s", 'ź': "z", 'ż': "z", 'ž': "z", 'æ': "ae", 'ý': "y", 'ř': "r",
}

// Normalize lowercases, folds accents (ä→a, ß→ss), turns punctuation into spaces and collapses whitespace.
func Normalize(s string) string {
	var b strings.Builder
	space := true
	for _, r := range strings.ToLower(s) {
		if f, ok := foldMap[r]; ok {
			b.WriteString(f)
			space = false
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
			continue
		}
		if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

func uniqueSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
