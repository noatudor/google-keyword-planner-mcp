package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ncosentino/google-keyword-planner-mcp/go/internal/keywordplanner"
)

const (
	maxTopics          = 25
	maxCompareKeywords = 20
	maxCompareMarkets  = 8
)

// --- score_topics ---

type scoreTopicsInput struct {
	Topics []string `json:"topics" jsonschema:"Candidate topics as head terms (1-25), e.g. from web search: ['heat pump', 'stairlift', 'hospital bed rental']. Write them in the market's language."`
	marketInput
	scoringInput
	Alternatives int `json:"alternatives,omitempty" jsonschema:"How many runner-up offerName variants to return per topic, for a keyword phrasing test. Default 3, max 5."`
}

type topicScore struct {
	Topic           string                      `json:"topic"`
	Verdict         string                      `json:"verdict"`
	OfferName       string                      `json:"offerName,omitempty"`
	BidderEvidence  string                      `json:"bidderEvidence,omitempty"`
	BestVariant     *keywordplanner.KeywordRow  `json:"bestVariant,omitempty"`
	Alternatives    []keywordplanner.KeywordRow `json:"alternatives,omitempty"`
	HeadTerm        *keywordplanner.KeywordRow  `json:"headTerm,omitempty"`
	BrandWarning    string                      `json:"brandWarning,omitempty"`
	IdeasConsidered int                         `json:"ideasConsidered"`
	StrongVariants  int                         `json:"strongVariants"`
	Error           string                      `json:"error,omitempty"`
}

type scoreTopicsResponse struct {
	Market       keywordplanner.Market `json:"market"`
	CurrencyCode string                `json:"currencyCode,omitempty"`
	Launchable   int                   `json:"launchable"`
	Topics       []*topicScore         `json:"topics"`
	Notes        []string              `json:"notes,omitempty"`
}

func scoreTopics(ctx context.Context, client *keywordplanner.Client, input scoreTopicsInput) (*mcp.CallToolResult, any, error) {
	if len(input.Topics) == 0 || len(input.Topics) > maxTopics {
		return toolError("scoring topics: pass 1-%d topics, got %d", maxTopics, len(input.Topics))
	}
	alts := input.Alternatives
	if alts <= 0 {
		alts = 3
	}
	alts = min(alts, 5)
	s, err := newSession(ctx, client, input.marketInput, input.scoringInput)
	if err != nil {
		return toolError("scoring topics: %v", err)
	}

	results := make([]*topicScore, len(input.Topics))
	brandNames := map[string]bool{}
	var brandMu sync.Mutex
	forEach(len(input.Topics), func(i int) {
		topic := strings.TrimSpace(input.Topics[i])
		ts := &topicScore{Topic: topic}
		results[i] = ts
		res, err := client.KeywordIdeas(ctx, keywordplanner.IdeasRequest{Seeds: []string{topic}, Targeting: s.market.Targeting()})
		if err != nil {
			ts.Verdict, ts.Error = keywordplanner.VerdictSkip, err.Error()
			return
		}
		brands := keywordplanner.BrandNames(res.Ideas)
		brandMu.Lock()
		for b := range brands {
			brandNames[b] = true
		}
		brandMu.Unlock()
		scorer := s.scorer(s.market, brands, true)
		headKey := keywordplanner.DedupeKey(topic)
		var candidates []keywordplanner.KeywordRow
		for _, d := range res.Ideas {
			row := scorer.Row(d)
			row.MonthlySearchVolumes = nil
			if keywordplanner.DedupeKey(d.Text) == headKey {
				h := row
				h.IsSeed = true
				ts.HeadTerm = &h
			}
			if row.Brand.IsBrand || !keywordplanner.SharesTopic(d.Text, topic) {
				continue
			}
			candidates = append(candidates, row)
		}
		candidates = keywordplanner.Dedupe(candidates)
		sortRows(candidates, "afs_score")
		// A launchable variant always beats a higher-scoring borderline one.
		sort.SliceStable(candidates, func(i, j int) bool {
			return keywordplanner.VerdictRank(candidates[i].Verdict) > keywordplanner.VerdictRank(candidates[j].Verdict)
		})
		ts.IdeasConsidered = len(candidates)
		for _, c := range candidates {
			if c.DemandTier == keywordplanner.TierStrong {
				ts.StrongVariants++
			}
		}
		if ts.HeadTerm != nil && ts.HeadTerm.Brand.IsBrand {
			ts.BrandWarning = "the topic itself is a brand (" + strings.Join(ts.HeadTerm.Brand.Brands, ", ") + "): use a generic equivalent"
		}
		if len(candidates) == 0 {
			ts.Verdict = keywordplanner.VerdictSkip
			ts.Error = "no non-brand variant of this topic found"
			return
		}
		best := candidates[0]
		ts.BestVariant = &best
		ts.Alternatives = candidates[1:min(len(candidates), alts+1)]
	})

	// Head terms that Google did not return as ideas get an exact lookup, all topics in one call.
	var missing []string
	for _, ts := range results {
		if ts.HeadTerm == nil && ts.Error == "" {
			missing = append(missing, ts.Topic)
		}
	}
	if len(missing) > 0 {
		bc := brandCheck{names: brandNames, checked: true}
		if res, err := s.lookup(ctx, missing, s.market, bc); err == nil {
			for _, ts := range results {
				if r, ok := res.rows[ts.Topic]; ok && ts.HeadTerm == nil {
					r.MonthlySearchVolumes = nil
					r.IsSeed = true
					ts.HeadTerm = &r
				}
			}
		} else {
			s.note("head-term lookup failed: %v", err)
		}
	}

	var rows []*keywordplanner.KeywordRow
	for _, ts := range results {
		if ts.BestVariant != nil {
			rows = append(rows, ts.BestVariant)
			for i := range ts.Alternatives {
				rows = append(rows, &ts.Alternatives[i])
			}
		}
		if ts.HeadTerm != nil {
			rows = append(rows, ts.HeadTerm)
		}
	}
	s.addCountryBreakdown(ctx, rows, brandCheck{names: brandNames, checked: true})

	launchable := 0
	for _, ts := range results {
		if ts.BestVariant == nil {
			continue
		}
		ts.Verdict = ts.BestVariant.Verdict
		ts.OfferName = ts.BestVariant.Text
		ts.BidderEvidence = keywordplanner.BidderEvidence(*ts.BestVariant, s.market.Label, s.currency)
		if ts.Verdict == keywordplanner.VerdictLaunch {
			launchable++
		}
	}
	sort.SliceStable(results, func(i, j int) bool { return topicRank(results[i]) > topicRank(results[j]) })
	return jsonResult(scoreTopicsResponse{Market: s.market, CurrencyCode: s.currency, Launchable: launchable, Topics: results, Notes: s.notes})
}

func topicRank(ts *topicScore) int {
	if ts.BestVariant == nil {
		return -1
	}
	return keywordplanner.VerdictRank(ts.BestVariant.Verdict)*1000 + ts.BestVariant.AfsScore
}

// --- find_offer_variants ---

type findOfferVariantsInput struct {
	Topic string `json:"topic" jsonschema:"The head term to expand, in the market's language, e.g. 'treppenlift' or 'hospital bed'."`
	marketInput
	scoringInput
	ExtraModifiers []string `json:"extra_modifiers,omitempty" jsonschema:"More variants to test: templates with {t} for the topic ('{t} für zuhause') or plain words appended to the topic ('elektrisch')."`
	IncludeIdeas   *bool    `json:"include_ideas,omitempty"   jsonschema:"Also add Google's own related ideas that contain the topic. Default true."`
	Limit          int      `json:"limit,omitempty"           jsonschema:"Maximum number of variants to return. Default 20."`
}

type offerRecommendation struct {
	OfferName string `json:"offerName,omitempty"`
	RunnerUp  string `json:"runnerUp,omitempty"`
	Why       string `json:"why"`
}

type offerVariantsResponse struct {
	Topic          string                      `json:"topic"`
	Market         keywordplanner.Market       `json:"market"`
	CurrencyCode   string                      `json:"currencyCode,omitempty"`
	Recommendation offerRecommendation         `json:"recommendation"`
	HeadTerm       *keywordplanner.KeywordRow  `json:"headTerm,omitempty"`
	Variants       []keywordplanner.KeywordRow `json:"variants"`
	NotFound       []string                    `json:"notFound,omitempty"`
	Notes          []string                    `json:"notes,omitempty"`
}

func findOfferVariants(ctx context.Context, client *keywordplanner.Client, input findOfferVariantsInput) (*mcp.CallToolResult, any, error) {
	topic := strings.TrimSpace(input.Topic)
	if topic == "" {
		return toolError("finding offer variants: topic must not be empty")
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 20
	}
	s, err := newSession(ctx, client, input.marketInput, input.scoringInput)
	if err != nil {
		return toolError("finding offer variants: %v", err)
	}

	type candidate struct{ text, source, modifier string }
	var cands []candidate
	seen := map[string]bool{}
	add := func(text, source, modifier string) {
		k := keywordplanner.DedupeKey(text)
		if k != "" && !seen[k] {
			seen[k] = true
			cands = append(cands, candidate{text, source, modifier})
		}
	}
	add(topic, "head", "")
	for _, tpl := range keywordplanner.VariantTemplates(s.market.Language.Code) {
		add(strings.ReplaceAll(tpl, "{t}", topic), "template", strings.TrimSpace(strings.ReplaceAll(tpl, "{t}", "")))
	}
	for _, m := range input.ExtraModifiers {
		if strings.Contains(m, "{t}") {
			add(strings.ReplaceAll(m, "{t}", topic), "extra", strings.TrimSpace(strings.ReplaceAll(m, "{t}", "")))
		} else if m = strings.TrimSpace(m); m != "" {
			add(topic+" "+m, "extra", m)
		}
	}

	bc := brandCheck{names: map[string]bool{}, concepts: map[string][]keywordplanner.Concept{}}
	if defaultTrue(input.IncludeIdeas) {
		res, err := client.KeywordIdeas(ctx, keywordplanner.IdeasRequest{Seeds: []string{topic}, Targeting: s.market.Targeting()})
		if err != nil {
			s.note("related ideas unavailable: %v", err)
		} else {
			bc.names, bc.checked = keywordplanner.BrandNames(res.Ideas), true
			ideas := make([]keywordplanner.KeywordData, 0, len(res.Ideas))
			for _, d := range res.Ideas {
				if len(d.Concepts) > 0 {
					bc.concepts[keywordplanner.DedupeKey(d.Text)] = d.Concepts
				}
				if keywordplanner.SharesTopic(d.Text, topic) {
					ideas = append(ideas, d)
				}
			}
			sort.SliceStable(ideas, func(i, j int) bool { return ideas[i].HighTopOfPageBidMicros > ideas[j].HighTopOfPageBidMicros })
			for _, d := range ideas[:min(len(ideas), 30)] {
				add(d.Text, "idea", "")
			}
		}
	}

	texts := make([]string, len(cands))
	for i, c := range cands {
		texts[i] = c.text
	}
	res, err := s.lookup(ctx, texts, s.market, bc)
	if err != nil {
		return toolError("finding offer variants: %v", err)
	}
	var head *keywordplanner.KeywordRow
	var variants []keywordplanner.KeywordRow
	for _, c := range cands {
		r, ok := res.rows[c.text]
		if !ok {
			continue
		}
		r.Source, r.Modifier, r.MonthlySearchVolumes = c.source, c.modifier, nil
		if c.source == "head" {
			h := r
			head = &h
			continue
		}
		variants = append(variants, r)
	}
	variants = keywordplanner.Dedupe(variants)
	sortRows(variants, "afs_score")
	variants = variants[:min(len(variants), limit)]

	ptrs := make([]*keywordplanner.KeywordRow, 0, len(variants)+1)
	for i := range variants {
		ptrs = append(ptrs, &variants[i])
	}
	if head != nil {
		ptrs = append(ptrs, head)
	}
	s.addCountryBreakdown(ctx, ptrs, bc)

	return jsonResult(offerVariantsResponse{
		Topic:          topic,
		Market:         s.market,
		CurrencyCode:   s.currency,
		Recommendation: recommendOffer(head, variants, s.currency),
		HeadTerm:       head,
		Variants:       variants,
		NotFound:       res.notFound,
		Notes:          s.notes,
	})
}

func recommendOffer(head *keywordplanner.KeywordRow, variants []keywordplanner.KeywordRow, currency string) offerRecommendation {
	var best, runner *keywordplanner.KeywordRow
	for i := range variants {
		v := &variants[i]
		if v.Brand.IsBrand || v.Verdict == keywordplanner.VerdictSkip {
			continue
		}
		if best == nil {
			best = v
			continue
		}
		if v.Modifier != best.Modifier || v.Source == "idea" {
			runner = v
			break
		}
	}
	if head != nil && !head.Brand.IsBrand && (best == nil || head.AfsScore > best.AfsScore) {
		if best != nil {
			runner = best
		}
		best = head
	}
	if best == nil {
		return offerRecommendation{Why: "no variant has enough advertiser demand; do not launch this topic on planner evidence"}
	}
	rec := offerRecommendation{OfferName: best.Text, Why: keywordplanner.BidderEvidence(*best, "", currency)}
	if runner != nil {
		rec.RunnerUp = runner.Text
		rec.Why += fmt.Sprintf("; runner-up %q (score %d vs %d) for a keyword phrasing test", runner.Text, runner.AfsScore, best.AfsScore)
	}
	return rec
}

// --- compare_keyword_markets ---

type marketSpec struct {
	Label     string   `json:"label,omitempty"     jsonschema:"Short name for the market, e.g. 'DE' or 'US-en'. Defaults to the country codes."`
	Language  string   `json:"language,omitempty"  jsonschema:"Language code, name or languageConstants/<id>. Omit to infer it from the first country."`
	Countries []string `json:"countries,omitempty" jsonschema:"ISO alpha-2 codes, presets or geoTargetConstants/<id>."`
}

type compareKeywordMarketsInput struct {
	Keywords []string     `json:"keywords" jsonschema:"Keywords to compare (1-20). Use each market's own language when they differ, or compare one English keyword across English markets."`
	Markets  []marketSpec `json:"markets"  jsonschema:"Markets to compare (1-8), e.g. [{\"countries\":[\"US\"]},{\"countries\":[\"GB\"]},{\"countries\":[\"CA\"]}]."`
	Network  string       `json:"network,omitempty" jsonschema:"GOOGLE_SEARCH (default) or GOOGLE_SEARCH_AND_PARTNERS."`
	scoringInput
}

type marketSummary struct {
	Label                string   `json:"label"`
	Language             string   `json:"language,omitempty"`
	Countries            []string `json:"countries,omitempty"`
	KeywordsFound        int      `json:"keywordsFound"`
	StrongKeywords       int      `json:"strongKeywords"`
	LaunchKeywords       int      `json:"launchKeywords"`
	TotalAdvertiserValue float64  `json:"totalAdvertiserValue"`
	AvgHighTopOfPageBid  float64  `json:"avgHighTopOfPageBid"`
	Error                string   `json:"error,omitempty"`
}

type keywordAcrossMarkets struct {
	Text       string                                    `json:"text"`
	Markets    map[string]*keywordplanner.CountryMetrics `json:"markets"`
	BestMarket string                                    `json:"bestMarket,omitempty"`
	Brand      *keywordplanner.BrandInfo                 `json:"brand,omitempty"`
}

type compareMarketsResponse struct {
	CurrencyCode  string                  `json:"currencyCode,omitempty"`
	Markets       []keywordplanner.Market `json:"markets"`
	MarketRanking []marketSummary         `json:"marketRanking"`
	Keywords      []keywordAcrossMarkets  `json:"keywords"`
	Notes         []string                `json:"notes,omitempty"`
}

func compareKeywordMarkets(ctx context.Context, client *keywordplanner.Client, input compareKeywordMarketsInput) (*mcp.CallToolResult, any, error) {
	if len(input.Keywords) == 0 || len(input.Keywords) > maxCompareKeywords {
		return toolError("comparing markets: pass 1-%d keywords, got %d", maxCompareKeywords, len(input.Keywords))
	}
	if len(input.Markets) == 0 || len(input.Markets) > maxCompareMarkets {
		return toolError("comparing markets: pass 1-%d markets, got %d", maxCompareMarkets, len(input.Markets))
	}
	s, err := newSession(ctx, client, marketInput{Network: input.Network}, input.scoringInput)
	if err != nil {
		return toolError("comparing markets: %v", err)
	}
	markets := make([]keywordplanner.Market, len(input.Markets))
	labels := map[string]bool{}
	for i, spec := range input.Markets {
		if spec.Language == "" && len(spec.Countries) == 0 {
			return toolError("comparing markets: market %d needs a language or countries", i+1)
		}
		m, notes, err := keywordplanner.ResolveMarket(spec.Label, spec.Language, spec.Countries, input.Network)
		if err != nil {
			return toolError("comparing markets: market %d: %v", i+1, err)
		}
		if labels[m.Label] {
			m.Label = fmt.Sprintf("%s#%d", m.Label, i+1)
		}
		labels[m.Label] = true
		markets[i] = m
		for _, n := range notes {
			s.note("%s: %s", m.Label, n)
		}
	}

	bc := s.checkBrands(ctx, input.Keywords, markets[0])
	results := make([]*lookupResult, len(markets))
	summaries := make([]marketSummary, len(markets))
	forEach(len(markets), func(i int) {
		m := markets[i]
		sum := marketSummary{Label: m.Label, Language: m.Language.Code}
		for _, c := range m.Countries {
			sum.Countries = append(sum.Countries, c.Code)
		}
		res, err := s.lookup(ctx, input.Keywords, m, bc)
		if err != nil {
			sum.Error = err.Error()
			summaries[i] = sum
			return
		}
		results[i] = res
		totalBid := 0.0
		for _, r := range res.rows {
			sum.KeywordsFound++
			sum.TotalAdvertiserValue += r.AdvertiserValue
			totalBid += r.HighTopOfPageBid
			if r.DemandTier == keywordplanner.TierStrong {
				sum.StrongKeywords++
			}
			if r.Verdict == keywordplanner.VerdictLaunch {
				sum.LaunchKeywords++
			}
		}
		if sum.KeywordsFound > 0 {
			sum.AvgHighTopOfPageBid = float64(int(totalBid/float64(sum.KeywordsFound)*100+0.5)) / 100
		}
		sum.TotalAdvertiserValue = float64(int(sum.TotalAdvertiserValue*100+0.5)) / 100
		summaries[i] = sum
	})

	kws := make([]keywordAcrossMarkets, 0, len(input.Keywords))
	for _, kw := range input.Keywords {
		k := keywordAcrossMarkets{Text: kw, Markets: map[string]*keywordplanner.CountryMetrics{}}
		for i, m := range markets {
			if results[i] == nil {
				continue
			}
			if r, ok := results[i].rows[kw]; ok {
				k.Markets[m.Label] = keywordplanner.CountrySlice(r, m.Language.Code)
				if k.Brand == nil && r.Brand.IsBrand {
					b := r.Brand
					k.Brand = &b
				}
			} else {
				k.Markets[m.Label] = &keywordplanner.CountryMetrics{Language: m.Language.Code}
			}
		}
		k.BestMarket = keywordplanner.BestCountry(k.Markets)
		kws = append(kws, k)
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].StrongKeywords != summaries[j].StrongKeywords {
			return summaries[i].StrongKeywords > summaries[j].StrongKeywords
		}
		return summaries[i].TotalAdvertiserValue > summaries[j].TotalAdvertiserValue
	})
	return jsonResult(compareMarketsResponse{CurrencyCode: s.currency, Markets: markets, MarketRanking: summaries, Keywords: kws, Notes: s.notes})
}

// --- find_seasonal_opportunities ---

type findSeasonalInput struct {
	SeedKeywords []string `json:"seed_keywords,omitempty" jsonschema:"Seed keywords for the season or category (max 20), e.g. ['heizung', 'winterreifen', 'kamin']. At least one of seed_keywords or url."`
	URL          string   `json:"url,omitempty"           jsonschema:"A page to seed from, e.g. a news article about an upcoming event or subsidy."`
	marketInput
	scoringInput
	WindowStartDays int      `json:"window_start_days,omitempty" jsonschema:"Start of the launch window in days from today. Default 14 (the review lag)."`
	WindowEndDays   int      `json:"window_end_days,omitempty"   jsonschema:"End of the launch window in days from today. Default 42 (the 2-6 week research horizon)."`
	MinLift         float64  `json:"min_lift,omitempty"          jsonschema:"Minimum seasonal lift: window-month volume vs the yearly average. Default 1.2."`
	IncludeRising   *bool    `json:"include_rising,omitempty"    jsonschema:"Also include keywords whose demand is rising year over year, even without a seasonal peak. Default true."`
	MinDemandTier   string   `json:"min_demand_tier,omitempty"   jsonschema:"Minimum demand tier: weak, usable (default) or strong."`
	ExcludeTerms    []string `json:"exclude_terms,omitempty"     jsonschema:"Drop ideas containing any of these words."`
	Limit           int      `json:"limit,omitempty"             jsonschema:"Maximum number of opportunities. Default 30."`
}

type seasonalResponse struct {
	Market        keywordplanner.Market       `json:"market"`
	CurrencyCode  string                      `json:"currencyCode,omitempty"`
	WindowFrom    string                      `json:"windowFrom"`
	WindowTo      string                      `json:"windowTo"`
	IdeasScanned  int                         `json:"ideasScanned"`
	Count         int                         `json:"count"`
	Opportunities []keywordplanner.KeywordRow `json:"opportunities"`
	Notes         []string                    `json:"notes,omitempty"`
}

func findSeasonalOpportunities(ctx context.Context, client *keywordplanner.Client, input findSeasonalInput) (*mcp.CallToolResult, any, error) {
	if len(input.SeedKeywords) == 0 && input.URL == "" {
		return toolError("finding seasonal opportunities: at least one of seed_keywords or url must be provided")
	}
	if len(input.SeedKeywords) > maxSeedKeywords {
		return toolError("finding seasonal opportunities: at most %d seed_keywords", maxSeedKeywords)
	}
	minTier := input.MinDemandTier
	if minTier == "" {
		minTier = keywordplanner.TierUsable
	}
	if keywordplanner.TierRank(minTier) == 0 {
		return toolError("finding seasonal opportunities: unknown min_demand_tier %q", input.MinDemandTier)
	}
	startDays, endDays := input.WindowStartDays, input.WindowEndDays
	if startDays <= 0 {
		startDays = 14
	}
	if endDays <= 0 {
		endDays = 42
	}
	if endDays < startDays {
		return toolError("finding seasonal opportunities: window_end_days must be >= window_start_days")
	}
	minLift := input.MinLift
	if minLift <= 0 {
		minLift = 1.2
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 30
	}

	s, err := newSession(ctx, client, input.marketInput, input.scoringInput)
	if err != nil {
		return toolError("finding seasonal opportunities: %v", err)
	}
	res, err := client.KeywordIdeas(ctx, keywordplanner.IdeasRequest{Seeds: input.SeedKeywords, URL: input.URL, Targeting: s.market.Targeting()})
	if err != nil {
		return toolError("finding seasonal opportunities: %v", err)
	}
	now := client.Now()
	from, to := now.AddDate(0, 0, startDays), now.AddDate(0, 0, endDays)
	scorer := s.scorer(s.market, keywordplanner.BrandNames(res.Ideas), true)
	f := newIdeaFilter(generateKeywordIdeasInput{ExcludeTerms: input.ExcludeTerms, MinDemandTier: minTier})
	includeRising := defaultTrue(input.IncludeRising)

	type scored struct {
		row   keywordplanner.KeywordRow
		value float64
	}
	var hits []scored
	for _, d := range res.Ideas {
		r := scorer.Row(d)
		r.MonthlySearchVolumes = nil
		if r.Brand.IsBrand || r.Trend == nil || !f.keep(r) ||
			r.Intent == keywordplanner.IntentInformational || r.Intent == keywordplanner.IntentNavigational {
			continue
		}
		lift := r.Trend.SeasonalLift(from, to)
		rising := r.Trend.Direction == "rising"
		if lift < minLift && (!includeRising || !rising) {
			continue
		}
		r.SeasonalLift = &lift
		why := fmt.Sprintf("why now: window volume %.2fx the yearly average", lift)
		if rising {
			why += ", demand rising"
		}
		r.Reasons = append([]string{why}, r.Reasons...)
		value := r.AdvertiserValue * maxf(lift, 1)
		if rising {
			value *= 1.2
		}
		hits = append(hits, scored{r, value})
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].value > hits[j].value })
	out := make([]keywordplanner.KeywordRow, 0, min(len(hits), limit))
	for _, h := range hits[:min(len(hits), limit)] {
		out = append(out, h.row)
	}
	out = keywordplanner.Dedupe(out)
	ptrs := make([]*keywordplanner.KeywordRow, len(out))
	for i := range out {
		ptrs[i] = &out[i]
	}
	s.addCountryBreakdown(ctx, ptrs, brandCheck{names: scorer.Brands, checked: true})
	return jsonResult(seasonalResponse{
		Market:        s.market,
		CurrencyCode:  s.currency,
		WindowFrom:    from.Format(time.DateOnly),
		WindowTo:      to.Format(time.DateOnly),
		IdeasScanned:  len(res.Ideas),
		Count:         len(out),
		Opportunities: out,
		Notes:         s.notes,
	})
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// --- get_targeting_reference ---

type targetingReferenceInput struct{}

type targetingReference struct {
	Languages      []keywordplanner.Language         `json:"languages"`
	Countries      []keywordplanner.Country          `json:"countries"`
	CountryPresets map[string][]string               `json:"countryPresets"`
	Networks       []string                          `json:"networks"`
	Thresholds     keywordplanner.Thresholds         `json:"defaultThresholds"`
	Lexicons       map[string]keywordplanner.Lexicon `json:"intentLexicons"`
	Notes          []string                          `json:"notes"`
}

func getTargetingReference(_ context.Context, _ *keywordplanner.Client, _ targetingReferenceInput) (*mcp.CallToolResult, any, error) {
	return jsonResult(targetingReference{
		Languages:      keywordplanner.Languages(),
		Countries:      keywordplanner.Countries(),
		CountryPresets: keywordplanner.CountryPresets,
		Networks:       []string{"GOOGLE_SEARCH", "GOOGLE_SEARCH_AND_PARTNERS"},
		Thresholds:     keywordplanner.DefaultThresholds(),
		Lexicons:       keywordplanner.Lexicons,
		Notes: []string{
			"All bids and CPCs are in the Google Ads account currency (currencyCode on every response); *Micros fields are the raw values (1,000,000 = 1.00).",
			"lowTopOfPageBid/highTopOfPageBid are the 20th/80th percentile top-of-page bids; averageCpc is what advertisers actually paid. Together they are the RPC signal for AdSense for Search.",
			"demandTier: strong = high bid >= 2.00, competition HIGH or MEDIUM, 500+ searches; usable = high bid 1.00-2.00, or strong bids at 100-500 searches; weak = everything else.",
			"afsScore (0-100) weighs price 40, volume 20, competition 15, commercial intent 15, trend 10. Brand keywords are capped at 10 and always skipped.",
			"estimatedRpc and breakEvenFbCpc appear only when rpc_ratio is passed; calibrate it from tracker RPC vs planner high bid, never guess it.",
			"sensitiveCategoryHint is informational only: Adspy assigns the special ad category from offerName. Never set it yourself.",
			"Country constants are 2000 + the ISO 3166 numeric code; any geoTargetConstants/<id> (also regions and cities) is accepted.",
		},
	})
}
