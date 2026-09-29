package main

import (
	"context"
	"math"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ncosentino/google-keyword-planner-mcp/go/internal/keywordplanner"
)

const (
	maxLookupKeywords   = 500
	maxForecastKeywords = 20
)

// getHistoricalMetricsInput is the input schema for the get_historical_metrics tool.
type getHistoricalMetricsInput struct {
	Keywords []string `json:"keywords" jsonschema:"Exact keywords to look up (max 500), e.g. candidate offerNames ['wärmepumpe finanzieren', 'wärmepumpe mieten']."`
	marketInput
	scoringInput
	DetectBrands          *bool `json:"detect_brands,omitempty"           jsonschema:"Check the keywords against Google's brand annotations (one extra API call per 20 keywords). Default true."`
	IncludeMonthlyVolumes *bool `json:"include_monthly_volumes,omitempty" jsonschema:"Include the 24-month search volume series on each row. Default true."`
}

type historicalResponse struct {
	Market       keywordplanner.Market       `json:"market"`
	CurrencyCode string                      `json:"currencyCode,omitempty"`
	Keywords     []keywordplanner.KeywordRow `json:"keywords"`
	Count        int                         `json:"count"`
	NotFound     []string                    `json:"notFound,omitempty"`
	MobileShare  *float64                    `json:"mobileShare,omitempty"`
	Notes        []string                    `json:"notes,omitempty"`
}

func getHistoricalMetrics(ctx context.Context, client *keywordplanner.Client, input getHistoricalMetricsInput) (*mcp.CallToolResult, any, error) {
	if len(input.Keywords) == 0 {
		return toolError("getting historical metrics: keywords must not be empty")
	}
	if len(input.Keywords) > maxLookupKeywords {
		return toolError("getting historical metrics: at most %d keywords per call, got %d", maxLookupKeywords, len(input.Keywords))
	}
	s, err := newSession(ctx, client, input.marketInput, input.scoringInput)
	if err != nil {
		return toolError("getting historical metrics: %v", err)
	}
	var bc brandCheck
	if defaultTrue(input.DetectBrands) {
		bc = s.checkBrands(ctx, input.Keywords, s.market)
	}
	res, err := s.lookup(ctx, input.Keywords, s.market, bc)
	if err != nil {
		return toolError("getting historical metrics: %v", err)
	}
	rows := make([]keywordplanner.KeywordRow, 0, len(res.rows))
	for _, kw := range input.Keywords {
		if r, ok := res.rows[kw]; ok {
			if !defaultTrue(input.IncludeMonthlyVolumes) {
				r.MonthlySearchVolumes = nil
			}
			rows = append(rows, r)
		}
	}
	ptrs := make([]*keywordplanner.KeywordRow, len(rows))
	for i := range rows {
		ptrs[i] = &rows[i]
	}
	s.addCountryBreakdown(ctx, ptrs, bc)
	return jsonResult(historicalResponse{
		Market:       s.market,
		CurrencyCode: s.currency,
		Keywords:     rows,
		Count:        len(rows),
		NotFound:     res.notFound,
		MobileShare:  mobileShare(res.deviceSearches),
		Notes:        s.notes,
	})
}

// getKeywordForecastInput is the input schema for the get_keyword_forecast tool.
type getKeywordForecastInput struct {
	Keywords     []string `json:"keywords"                 jsonschema:"Keywords to forecast (max 20). Each is forecast on its own, plus one combined total."`
	MaxCPCMicros int64    `json:"max_cpc_micros,omitempty" jsonschema:"Maximum CPC bid in micros (1,000,000 = 1.00 in account currency). Defaults to 1,000,000 if omitted or 0. Ignored when max_cpc is set."`
	MaxCPC       float64  `json:"max_cpc,omitempty"        jsonschema:"Maximum CPC bid in account currency units (e.g. 2.5). Overrides max_cpc_micros."`
	DailyBudget  float64  `json:"daily_budget,omitempty"   jsonschema:"Optional daily budget in account currency units for the simulated campaign."`
	ForecastDays int      `json:"forecast_days,omitempty"  jsonschema:"Number of days to forecast, starting tomorrow. Defaults to 30 if omitted or 0. Max 365."`
	MatchType    string   `json:"match_type,omitempty"     jsonschema:"Keyword match type: BROAD (default), PHRASE or EXACT."`
	Language     string   `json:"language,omitempty"       jsonschema:"Language: ISO code ('de'), English name or 'languageConstants/<id>'. Omit to infer it from the first country."`
	Countries    []string `json:"countries,omitempty"      jsonschema:"Countries: ISO alpha-2 codes, presets (DACH, EN_CORE, ...) or 'geoTargetConstants/<id>'. Omit for worldwide."`
	Network      string   `json:"network,omitempty"        jsonschema:"GOOGLE_SEARCH (default) or GOOGLE_SEARCH_AND_PARTNERS."`
}

type forecastRow struct {
	Text             string  `json:"text,omitempty"`
	Impressions      float64 `json:"impressions"`
	Clicks           float64 `json:"clicks"`
	CTR              float64 `json:"ctr"`
	AverageCpc       float64 `json:"averageCpc"`
	AverageCpcMicros int64   `json:"averageCpcMicros"`
	Cost             float64 `json:"cost"`
	CostMicros       int64   `json:"costMicros"`
	Error            string  `json:"error,omitempty"`
}

type forecastResponse struct {
	Market       keywordplanner.Market `json:"market"`
	CurrencyCode string                `json:"currencyCode,omitempty"`
	ForecastDays int                   `json:"forecastDays"`
	MaxCPC       float64               `json:"maxCpc"`
	MaxCPCMicros int64                 `json:"maxCpcMicros"`
	MatchType    string                `json:"matchType"`
	Total        forecastRow           `json:"total"`
	Keywords     []forecastRow         `json:"keywords"`
	Notes        []string              `json:"notes,omitempty"`
}

var validMatchTypes = map[string]bool{"BROAD": true, "PHRASE": true, "EXACT": true}

func getKeywordForecast(ctx context.Context, client *keywordplanner.Client, input getKeywordForecastInput) (*mcp.CallToolResult, any, error) {
	if len(input.Keywords) == 0 {
		return toolError("getting keyword forecast: keywords must not be empty")
	}
	if len(input.Keywords) > maxForecastKeywords {
		return toolError("getting keyword forecast: at most %d keywords per call, got %d", maxForecastKeywords, len(input.Keywords))
	}
	matchType := input.MatchType
	if matchType == "" {
		matchType = "BROAD"
	}
	if !validMatchTypes[matchType] {
		return toolError("getting keyword forecast: unknown match_type %q; use BROAD, PHRASE or EXACT", input.MatchType)
	}
	days := input.ForecastDays
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		return toolError("getting keyword forecast: forecast_days must be at most 365")
	}
	bid := input.MaxCPCMicros
	if input.MaxCPC > 0 {
		bid = int64(math.Round(input.MaxCPC * 1_000_000))
	}
	if bid <= 0 {
		bid = 1_000_000
	}

	s, err := newSession(ctx, client, marketInput{Language: input.Language, Countries: input.Countries, Network: input.Network}, scoringInput{})
	if err != nil {
		return toolError("getting keyword forecast: %v", err)
	}
	req := keywordplanner.ForecastRequest{
		MatchType:         matchType,
		MaxCPCBidMicros:   bid,
		DailyBudgetMicros: int64(math.Round(input.DailyBudget * 1_000_000)),
		Days:              days,
		Targeting:         s.market.Targeting(),
	}

	req.Keywords = input.Keywords
	total, err := client.Forecast(ctx, req)
	if err != nil {
		return toolError("getting keyword forecast: %v", err)
	}
	rows := make([]forecastRow, len(input.Keywords))
	if len(input.Keywords) == 1 {
		rows[0] = toForecastRow(input.Keywords[0], total)
	} else {
		forEach(len(input.Keywords), func(i int) {
			one := req
			one.Keywords = []string{input.Keywords[i]}
			m, err := client.Forecast(ctx, one)
			if err != nil {
				rows[i] = forecastRow{Text: input.Keywords[i], Error: err.Error()}
				return
			}
			rows[i] = toForecastRow(input.Keywords[i], m)
		})
	}
	return jsonResult(forecastResponse{
		Market:       s.market,
		CurrencyCode: s.currency,
		ForecastDays: days,
		MaxCPC:       float64(bid) / 1_000_000,
		MaxCPCMicros: bid,
		MatchType:    matchType,
		Total:        toForecastRow("", total),
		Keywords:     rows,
		Notes:        s.notes,
	})
}

func toForecastRow(text string, m *keywordplanner.ForecastMetrics) forecastRow {
	return forecastRow{
		Text:             text,
		Impressions:      math.Round(m.Impressions),
		Clicks:           math.Round(m.Clicks*10) / 10,
		CTR:              math.Round(m.CTR*10000) / 10000,
		AverageCpc:       math.Round(float64(m.AverageCpcMicros)/10_000) / 100,
		AverageCpcMicros: m.AverageCpcMicros,
		Cost:             math.Round(float64(m.CostMicros)/10_000) / 100,
		CostMicros:       m.CostMicros,
	}
}
