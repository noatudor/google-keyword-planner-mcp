package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ncosentino/google-keyword-planner-mcp/go/internal/keywordplanner"
)

const afsRowNote = " Every keyword row is scored for AdSense-for-Search arbitrage: bids and average CPC in account currency (currencyCode), " +
	"demandTier (strong = high bid >= 2.00 + HIGH/MEDIUM competition + 500 searches; usable; weak), commercialIntent, brand flag from Google's brand annotations, " +
	"24-month trend and seasonality, advertiserValue (searches × high bid), afsScore 0-100, verdict launch/borderline/skip and reasons. " +
	"Set countries (e.g. ['DE']) for per-market bids; per_country=true adds a byCountry breakdown."

// toolArrayFields declares, for each tool name, which top-level argument fields
// are array-typed. coerceStringifiedArrayArgs (stringified_args.go) uses this to
// know which fields to repair; it is a plain data map, so every tool with a
// list parameter is covered by one code path.
var toolArrayFields = map[string][]string{
	"generate_keyword_ideas":      {"seed_keywords", "countries", "competition", "intent", "exclude_terms"},
	"get_historical_metrics":      {"keywords", "countries"},
	"get_keyword_forecast":        {"keywords", "countries"},
	"score_topics":                {"topics", "countries"},
	"find_offer_variants":         {"countries", "extra_modifiers"},
	"compare_keyword_markets":     {"keywords", "markets"},
	"find_seasonal_opportunities": {"seed_keywords", "countries", "exclude_terms"},
}

// registerTools adds every Keyword Planner tool to srv.
func registerTools(srv *mcp.Server, client *keywordplanner.Client) {
	addTool(srv, client, "generate_keyword_ideas",
		"Find keywords advertisers pay for, from seed keywords, a url or a whole site, using Google Ads Keyword Planner. "+
			"Filters (min_high_bid, min_monthly_searches, competition, min_demand_tier, intent, exclude_terms), drops brand keywords by default, "+
			"collapses look-alike variants, sorts by afs_score and returns the top 50 (limit up to 2000). Seed keywords are always returned, so it also works as a lookup."+afsRowNote,
		generateKeywordIdeas)
	addTool(srv, client, "get_historical_metrics",
		"Look up exact keywords (e.g. candidate offerNames) in Google Ads Keyword Planner: monthly searches, competition, top-of-page bids, "+
			"average CPC advertisers actually paid, 24-month volumes, trend and brand check."+afsRowNote,
		getHistoricalMetrics)
	addTool(srv, client, "get_keyword_forecast",
		"Forecast impressions, clicks, CTR, average CPC and cost per keyword (and in total) for a simulated Google Search campaign at a given max CPC, "+
			"per market. Shows the price at which advertisers actually clear. Bids are in account currency.",
		getKeywordForecast)
	addTool(srv, client, "score_topics",
		"Screen candidate topics (e.g. from web search) for AdSense-for-Search arbitrage in one call. For each topic: the best non-brand offerName variant, "+
			"runner-up variants for a phrasing test, the head term, verdict, and a one-line bidderEvidence with searches, competition, bids and average CPC "+
			"to answer 'who is bidding behind this keyword?'. Topics are ranked best first."+afsRowNote,
		scoreTopics)
	addTool(srv, client, "find_offer_variants",
		"Pick the exact offerName for one topic: expands the head term with the market language's commercial modifiers "+
			"(cost, financing, rental, near me, for seniors, grant, used, ... in en/de/fr/es/it/nl/pt/pl/sv/da/no/fi), adds Google's related ideas, "+
			"looks all of them up and recommends the best variant plus a runner-up for a phrasing test."+afsRowNote,
		findOfferVariants)
	addTool(srv, client, "compare_keyword_markets",
		"Compare the same keywords across markets (up to 8, each a language + countries): searches, bids, average CPC, tier and score per market, "+
			"the best market per keyword and a market ranking by strong keywords and advertiser value. Use it to take a winner into a new country or language.",
		compareKeywordMarkets)
	addTool(srv, client, "find_seasonal_opportunities",
		"Find commercial, non-brand keywords whose historical demand peaks inside an upcoming launch window (default 14-42 days out) or is rising "+
			"year over year, ranked by advertiser value × seasonal lift. For calendar topics: heating, winter tyres, insurance enrolment, events."+afsRowNote,
		findSeasonalOpportunities)
	addTool(srv, client, "get_targeting_reference",
		"Static reference, no API quota: supported language and country codes with their Google constants, country presets, networks, "+
			"demand tier thresholds, intent modifier lexicons and how every AFS field is computed.",
		getTargetingReference)
}

func addTool[In any](srv *mcp.Server, client *keywordplanner.Client, name, description string,
	handler func(context.Context, *keywordplanner.Client, In) (*mcp.CallToolResult, any, error)) {
	mcp.AddTool(srv, &mcp.Tool{Name: name, Description: description},
		func(ctx context.Context, _ *mcp.CallToolRequest, input In) (*mcp.CallToolResult, any, error) {
			return handler(ctx, client, input)
		})
}
