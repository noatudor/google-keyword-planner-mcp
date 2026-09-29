package main

import (
	"context"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ncosentino/google-keyword-planner-mcp/go/internal/keywordplanner"
)

// generateKeywordIdeasInput is the input schema for the generate_keyword_ideas tool.
// SeedKeywords, URL and Site all carry ",omitempty" so none is marked required in the
// exported JSON schema: the tool only requires that at least one of them be provided,
// which is enforced at runtime in generateKeywordIdeas rather than by the schema.
type generateKeywordIdeasInput struct {
	SeedKeywords []string `json:"seed_keywords,omitempty" jsonschema:"Seed keywords to generate ideas from (max 20, e.g. ['treppenlift', 'treppenlift mieten']). At least one of seed_keywords, url or site must be provided. Seeds are always returned, so this doubles as a lookup."`
	URL          string   `json:"url,omitempty"           jsonschema:"A page to generate ideas from, e.g. an advertiser landing page or a news article, to see the commercial keywords around it. At least one of seed_keywords, url or site must be provided."`
	Site         string   `json:"site,omitempty"          jsonschema:"A whole domain (e.g. 'example.com') to generate ideas from every page of it. Used instead of seed_keywords and url."`
	marketInput
	scoringInput
	MinMonthlySearches    int64    `json:"min_monthly_searches,omitempty"    jsonschema:"Only keep ideas with at least this many average monthly searches."`
	MinHighBid            float64  `json:"min_high_bid,omitempty"            jsonschema:"Only keep ideas whose high top-of-page bid is at least this, in account currency (e.g. 2.0)."`
	MinAverageCpc         float64  `json:"min_average_cpc,omitempty"         jsonschema:"Only keep ideas whose average CPC is at least this, in account currency."`
	Competition           []string `json:"competition,omitempty"             jsonschema:"Only keep these competition levels: LOW, MEDIUM, HIGH."`
	MinDemandTier         string   `json:"min_demand_tier,omitempty"         jsonschema:"Only keep ideas at or above this demand tier: weak, usable or strong."`
	Intent                []string `json:"intent,omitempty"                  jsonschema:"Only keep these commercial intents: transactional, commercial, informational, unclassified, navigational."`
	ExcludeBrands         *bool    `json:"exclude_brands,omitempty"          jsonschema:"Drop keywords Google annotates as brands (no-brand rule). Default true."`
	ExcludeTerms          []string `json:"exclude_terms,omitempty"           jsonschema:"Drop ideas containing any of these words (case- and accent-insensitive), e.g. extra brand names."`
	DedupeVariants        *bool    `json:"dedupe_variants,omitempty"         jsonschema:"Collapse plural, accent and punctuation twins into one row, keeping the best-scoring text. Default true."`
	SortBy                string   `json:"sort_by,omitempty"                 jsonschema:"Sort order: afs_score (default), advertiser_value, high_bid, average_cpc, searches, trend, or relevance (Google's order)."`
	Limit                 int      `json:"limit,omitempty"                   jsonschema:"Maximum number of ideas to return. Default 50, max 2000."`
	IncludeMonthlyVolumes bool     `json:"include_monthly_volumes,omitempty" jsonschema:"Include the 24-month search volume series on each row. Default false (the trend summary is always included)."`
}

type ideasResponse struct {
	SeedKeywords   []string                    `json:"seedKeywords,omitempty"`
	URL            string                      `json:"url,omitempty"`
	Site           string                      `json:"site,omitempty"`
	Market         keywordplanner.Market       `json:"market"`
	CurrencyCode   string                      `json:"currencyCode,omitempty"`
	TotalIdeas     int                         `json:"totalIdeas"`
	MatchedFilters int                         `json:"matchedFilters"`
	BrandFiltered  int                         `json:"brandFiltered"`
	Count          int                         `json:"count"`
	TierCounts     map[string]int              `json:"tierCounts"`
	MobileShare    *float64                    `json:"mobileShare,omitempty"`
	Ideas          []keywordplanner.KeywordRow `json:"ideas"`
	Notes          []string                    `json:"notes,omitempty"`
}

func generateKeywordIdeas(ctx context.Context, client *keywordplanner.Client, input generateKeywordIdeasInput) (*mcp.CallToolResult, any, error) {
	if len(input.SeedKeywords) == 0 && input.URL == "" && input.Site == "" {
		return toolError("at least one of seed_keywords or url must be provided (or site)")
	}
	if len(input.SeedKeywords) > maxSeedKeywords {
		return toolError("seed_keywords accepts at most %d keywords, got %d", maxSeedKeywords, len(input.SeedKeywords))
	}
	if !validSorts[input.SortBy] {
		return toolError("unknown sort_by %q", input.SortBy)
	}
	if input.MinDemandTier != "" && keywordplanner.TierRank(input.MinDemandTier) == 0 {
		return toolError("unknown min_demand_tier %q; use weak, usable or strong", input.MinDemandTier)
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 2000)

	s, err := newSession(ctx, client, input.marketInput, input.scoringInput)
	if err != nil {
		return toolError("generating keyword ideas: %v", err)
	}
	res, err := client.KeywordIdeas(ctx, keywordplanner.IdeasRequest{
		Seeds: input.SeedKeywords, URL: input.URL, Site: input.Site, Targeting: s.market.Targeting(),
	})
	if err != nil {
		return toolError("generating keyword ideas: %v", err)
	}

	scorer := s.scorer(s.market, keywordplanner.BrandNames(res.Ideas), true)
	seedKeys := map[string]bool{}
	for _, seed := range input.SeedKeywords {
		seedKeys[keywordplanner.DedupeKey(seed)] = true
	}
	rows := make([]keywordplanner.KeywordRow, 0, len(res.Ideas))
	for _, d := range res.Ideas {
		row := scorer.Row(d)
		row.IsSeed = seedKeys[keywordplanner.DedupeKey(d.Text)]
		rows = append(rows, row)
	}
	if defaultTrue(input.DedupeVariants) {
		rows = keywordplanner.Dedupe(rows)
	}

	f := newIdeaFilter(input)
	var seeds, matched []keywordplanner.KeywordRow
	brandFiltered := 0
	for _, r := range rows {
		if r.IsSeed {
			seeds = append(seeds, r)
			continue
		}
		if r.Brand.IsBrand && f.excludeBrands {
			brandFiltered++
			continue
		}
		if f.keep(r) {
			matched = append(matched, r)
		}
	}
	tierCounts := map[string]int{keywordplanner.TierStrong: 0, keywordplanner.TierUsable: 0, keywordplanner.TierWeak: 0}
	for _, r := range matched {
		tierCounts[r.DemandTier]++
	}
	if input.SortBy != "relevance" {
		sortRows(seeds, input.SortBy)
		sortRows(matched, input.SortBy)
	}
	out := append(seeds, matched[:min(len(matched), max(0, limit-len(seeds)))]...)

	ptrs := make([]*keywordplanner.KeywordRow, len(out))
	for i := range out {
		if !input.IncludeMonthlyVolumes {
			out[i].MonthlySearchVolumes = nil
		}
		ptrs[i] = &out[i]
	}
	s.addCountryBreakdown(ctx, ptrs, brandCheck{names: scorer.Brands, checked: true})

	return jsonResult(ideasResponse{
		SeedKeywords:   input.SeedKeywords,
		URL:            input.URL,
		Site:           input.Site,
		Market:         s.market,
		CurrencyCode:   s.currency,
		TotalIdeas:     len(res.Ideas),
		MatchedFilters: len(matched),
		BrandFiltered:  brandFiltered,
		Count:          len(out),
		TierCounts:     tierCounts,
		MobileShare:    mobileShare(res.DeviceSearches),
		Ideas:          out,
		Notes:          s.notes,
	})
}

type ideaFilter struct {
	in            generateKeywordIdeasInput
	excludeBrands bool
	excludeTerms  []string
	competition   []string
	intent        []string
}

func newIdeaFilter(in generateKeywordIdeasInput) ideaFilter {
	f := ideaFilter{in: in, excludeBrands: defaultTrue(in.ExcludeBrands)}
	for _, t := range in.ExcludeTerms {
		if n := keywordplanner.Normalize(t); n != "" {
			f.excludeTerms = append(f.excludeTerms, n)
		}
	}
	for _, c := range in.Competition {
		f.competition = append(f.competition, strings.ToUpper(strings.TrimSpace(c)))
	}
	for _, i := range in.Intent {
		f.intent = append(f.intent, strings.ToLower(strings.TrimSpace(i)))
	}
	return f
}

func (f ideaFilter) keep(r keywordplanner.KeywordRow) bool {
	if r.AvgMonthlySearches < f.in.MinMonthlySearches || r.HighTopOfPageBid < f.in.MinHighBid || r.AverageCpc < f.in.MinAverageCpc {
		return false
	}
	if len(f.competition) > 0 && !slices.Contains(f.competition, r.Competition) {
		return false
	}
	if len(f.intent) > 0 && !slices.Contains(f.intent, r.Intent) {
		return false
	}
	if f.in.MinDemandTier != "" && keywordplanner.TierRank(r.DemandTier) < keywordplanner.TierRank(f.in.MinDemandTier) {
		return false
	}
	norm := " " + keywordplanner.Normalize(r.Text) + " "
	for _, t := range f.excludeTerms {
		if strings.Contains(norm, t) {
			return false
		}
	}
	return true
}
