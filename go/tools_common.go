package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ncosentino/google-keyword-planner-mcp/go/internal/keywordplanner"
)

const (
	maxSeedKeywords       = 20
	maxPerCountryMarkets  = 10
	maxBrandCheckKeywords = 100
	fanOutConcurrency     = 2
)

// marketInput is shared by every keyword tool.
type marketInput struct {
	Language   string   `json:"language,omitempty"    jsonschema:"Language: ISO code ('de'), English name ('German') or resource name ('languageConstants/1001' = German, 'languageConstants/1000' = English). Omit to infer it from the first country (DE→de); omit both for all languages."`
	Countries  []string `json:"countries,omitempty"   jsonschema:"Countries to target: ISO alpha-2 codes (['DE','AT']), presets (DACH, BENELUX, NORDICS, IBERIA, EN_CORE, LATAM_ES) or 'geoTargetConstants/<id>'. Omit for worldwide. Bids differ a lot per country, so set this for every AFS decision."`
	PerCountry bool     `json:"per_country,omitempty" jsonschema:"With 2+ countries: also look every returned keyword up in each country separately (in that country's own language unless language is set) and add byCountry and bestCountry to each row. Costs one extra API call per country (max 10)."`
	Network    string   `json:"network,omitempty"     jsonschema:"GOOGLE_SEARCH (default, the network AFS advertisers bid on) or GOOGLE_SEARCH_AND_PARTNERS."`
}

// scoringInput overrides the AFS scoring defaults.
type scoringInput struct {
	StrongMinHighBid  float64 `json:"strong_min_high_bid,omitempty" jsonschema:"Override the strong tier's minimum high top-of-page bid, in account currency. Default 2.00."`
	StrongMinSearches int64   `json:"strong_min_searches,omitempty" jsonschema:"Override the strong tier's minimum monthly searches. Default 500."`
	UsableMinHighBid  float64 `json:"usable_min_high_bid,omitempty" jsonschema:"Override the usable tier's minimum high top-of-page bid. Default 1.00."`
	UsableMinSearches int64   `json:"usable_min_searches,omitempty" jsonschema:"Override the minimum monthly searches at which strong-level bids still count as usable. Default 100."`
	RPCRatio          float64 `json:"rpc_ratio,omitempty"           jsonschema:"Optional calibration from our own tracker data: RPC as a fraction of the high top-of-page bid (e.g. 0.12). When set, rows get estimatedRpc and breakEvenFbCpc. Never guess it; leave it out when uncalibrated."`
	TargetROI         float64 `json:"target_roi,omitempty"          jsonschema:"ROI the break-even Facebook CPC must still leave, as a fraction. Default 0.15 (the 15% ROI floor). Only used with rpc_ratio."`
}

func (si scoringInput) thresholds() keywordplanner.Thresholds {
	th := keywordplanner.DefaultThresholds()
	if si.StrongMinHighBid > 0 {
		th.StrongMinHighBid = si.StrongMinHighBid
	}
	if si.StrongMinSearches > 0 {
		th.StrongMinSearches = si.StrongMinSearches
	}
	if si.UsableMinHighBid > 0 {
		th.UsableMinHighBid = si.UsableMinHighBid
	}
	if si.UsableMinSearches > 0 {
		th.UsableMinSearches = si.UsableMinSearches
	}
	return th
}

// session carries the resolved market and scoring settings through one tool call.
type session struct {
	client       *keywordplanner.Client
	market       keywordplanner.Market
	perCountry   bool
	keepLanguage bool
	scoring      scoringInput
	currency     string
	notes        []string
	mu           sync.Mutex
}

func newSession(ctx context.Context, client *keywordplanner.Client, mi marketInput, si scoringInput) (*session, error) {
	market, notes, err := keywordplanner.ResolveMarket("", mi.Language, mi.Countries, mi.Network)
	if err != nil {
		return nil, err
	}
	s := &session{
		client:       client,
		market:       market,
		keepLanguage: mi.Language != "",
		scoring:      si,
		notes:        notes,
	}
	if mi.PerCountry {
		switch {
		case len(market.Countries) < 2:
			s.note("per_country ignored: it needs at least 2 countries")
		case len(market.Countries) > maxPerCountryMarkets:
			return nil, fmt.Errorf("per_country supports at most %d countries, got %d", maxPerCountryMarkets, len(market.Countries))
		default:
			s.perCountry = true
		}
	}
	s.currency = client.CurrencyCode(ctx)
	if s.currency == "" {
		s.note("account currency unknown: bids are in the Google Ads account currency")
	}
	return s, nil
}

func (s *session) note(format string, args ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notes = append(s.notes, fmt.Sprintf(format, args...))
}

func (s *session) scorer(m keywordplanner.Market, brands map[string]bool, brandChecked bool) keywordplanner.Scorer {
	return keywordplanner.Scorer{
		Thresholds:   s.scoring.thresholds(),
		Language:     m.Language.Code,
		Currency:     s.currency,
		RPCRatio:     s.scoring.RPCRatio,
		TargetROI:    s.scoring.TargetROI,
		Now:          s.client.Now(),
		Brands:       brands,
		BrandChecked: brandChecked,
	}
}

// brandCheck holds brand annotations gathered for a set of lookup keywords.
type brandCheck struct {
	names    map[string]bool
	concepts map[string][]keywordplanner.Concept
	checked  bool
}

// checkBrands annotates lookup keywords with Google's brand concepts. Historical metrics carry
// no annotations, so the keywords are also sent as idea seeds (20 per call).
func (s *session) checkBrands(ctx context.Context, keywords []string, m keywordplanner.Market) brandCheck {
	bc := brandCheck{names: map[string]bool{}, concepts: map[string][]keywordplanner.Concept{}}
	if len(keywords) > maxBrandCheckKeywords {
		s.note("brand check covered the first %d of %d keywords", maxBrandCheckKeywords, len(keywords))
		keywords = keywords[:maxBrandCheckKeywords]
	}
	for start := 0; start < len(keywords); start += maxSeedKeywords {
		end := min(start+maxSeedKeywords, len(keywords))
		res, err := s.client.KeywordIdeas(ctx, keywordplanner.IdeasRequest{Seeds: keywords[start:end], Targeting: m.Targeting()})
		if err != nil {
			s.note("brand check failed, brand flags are unverified: %v", err)
			return bc
		}
		for n := range keywordplanner.BrandNames(res.Ideas) {
			bc.names[n] = true
		}
		for _, d := range res.Ideas {
			if len(d.Concepts) > 0 {
				bc.concepts[keywordplanner.DedupeKey(d.Text)] = d.Concepts
			}
		}
	}
	bc.checked = true
	return bc
}

// lookupResult is the scored exact-keyword lookup in one market.
type lookupResult struct {
	rows           map[string]keywordplanner.KeywordRow // keyed by requested keyword
	notFound       []string
	deviceSearches map[string]int64
}

// lookup fetches historical metrics for exact keywords and scores them. The API groups close
// variants under one result, so each requested keyword is matched by its dedupe key.
func (s *session) lookup(ctx context.Context, keywords []string, m keywordplanner.Market, bc brandCheck) (*lookupResult, error) {
	res, err := s.client.HistoricalMetrics(ctx, keywords, m.Targeting())
	if err != nil {
		return nil, err
	}
	byKey := map[string]keywordplanner.KeywordData{}
	for _, d := range res.Keywords {
		if c, ok := bc.concepts[keywordplanner.DedupeKey(d.Text)]; ok && len(d.Concepts) == 0 {
			d.Concepts = c
		}
		byKey[keywordplanner.DedupeKey(d.Text)] = d
		for _, v := range d.CloseVariants {
			if _, exists := byKey[keywordplanner.DedupeKey(v)]; !exists {
				byKey[keywordplanner.DedupeKey(v)] = d
			}
		}
	}
	scorer := s.scorer(m, bc.names, bc.checked)
	out := &lookupResult{rows: map[string]keywordplanner.KeywordRow{}, deviceSearches: res.DeviceSearches}
	for _, kw := range keywords {
		d, ok := byKey[keywordplanner.DedupeKey(kw)]
		if !ok {
			out.notFound = append(out.notFound, kw)
			continue
		}
		row := scorer.Row(d)
		if d.Text != kw {
			row.MatchedAs = d.Text
			row.Text = kw
		}
		out.rows[kw] = row
	}
	return out, nil
}

// addCountryBreakdown adds byCountry and bestCountry to rows when per_country is on.
func (s *session) addCountryBreakdown(ctx context.Context, rows []*keywordplanner.KeywordRow, bc brandCheck) {
	if !s.perCountry || len(rows) == 0 {
		return
	}
	texts := make([]string, 0, len(rows))
	seen := map[string]bool{}
	for _, r := range rows {
		if !seen[r.Text] {
			seen[r.Text] = true
			texts = append(texts, r.Text)
		}
	}
	markets := s.market.PerCountry(s.keepLanguage)
	results := make([]*lookupResult, len(markets))
	forEach(len(markets), func(i int) {
		res, err := s.lookup(ctx, texts, markets[i], bc)
		if err != nil {
			s.note("per-country lookup for %s failed: %v", markets[i].Label, err)
			return
		}
		results[i] = res
	})
	for _, r := range rows {
		r.ByCountry = map[string]*keywordplanner.CountryMetrics{}
		for i, m := range markets {
			if results[i] == nil {
				continue
			}
			if cr, ok := results[i].rows[r.Text]; ok {
				r.ByCountry[m.Label] = keywordplanner.CountrySlice(cr, m.Language.Code)
			} else {
				r.ByCountry[m.Label] = &keywordplanner.CountryMetrics{Language: m.Language.Code}
			}
		}
		r.BestCountry = keywordplanner.BestCountry(r.ByCountry)
	}
}

// forEach runs fn(0..n-1) with at most fanOutConcurrency calls in flight. The client's
// rate limiter still paces the actual API requests.
func forEach(n int, fn func(i int)) {
	sem := make(chan struct{}, fanOutConcurrency)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			fn(i)
		}(i)
	}
	wg.Wait()
}

func mobileShare(device map[string]int64) *float64 {
	total := int64(0)
	for _, v := range device {
		total += v
	}
	if total == 0 {
		return nil
	}
	share := float64(device["MOBILE"]) / float64(total)
	share = float64(int(share*1000+0.5)) / 1000
	return &share
}

func sortRows(rows []keywordplanner.KeywordRow, by string) {
	key := func(r keywordplanner.KeywordRow) float64 {
		switch by {
		case "advertiser_value":
			return r.AdvertiserValue
		case "high_bid":
			return r.HighTopOfPageBid
		case "average_cpc":
			return r.AverageCpc
		case "searches":
			return float64(r.AvgMonthlySearches)
		case "trend":
			if r.Trend != nil && r.Trend.YoYPct != nil {
				return *r.Trend.YoYPct
			}
			if r.Trend != nil && r.Trend.Momentum3mPct != nil {
				return *r.Trend.Momentum3mPct
			}
			return -1e9
		}
		return float64(r.AfsScore)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ki, kj := key(rows[i]), key(rows[j])
		if ki != kj {
			return ki > kj
		}
		return rows[i].AdvertiserValue > rows[j].AdvertiserValue
	})
}

var validSorts = map[string]bool{"": true, "relevance": true, "afs_score": true, "advertiser_value": true,
	"high_bid": true, "average_cpc": true, "searches": true, "trend": true}

// defaultTrue reads an optional boolean that defaults to true.
func defaultTrue(p *bool) bool {
	return p == nil || *p
}

func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling result: %w", err)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
}

// toolError returns an in-band tool error (IsError=true) so the calling model sees it as a failure.
func toolError(format string, args ...any) (*mcp.CallToolResult, any, error) {
	b, _ := json.Marshal(map[string]string{"error": fmt.Sprintf(format, args...)})
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil, nil
}
