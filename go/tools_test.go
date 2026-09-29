package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ncosentino/google-keyword-planner-mcp/go/internal/keywordplanner"
)

var testNow = time.Date(2026, time.September, 29, 10, 0, 0, 0, time.UTC)

// fakeAds is a routing fake of the Google Ads API. Each handler receives the decoded request body.
type fakeAds struct {
	mu         sync.Mutex
	calls      map[string]int
	ideas      func(req map[string]any) []map[string]any
	historical func(req map[string]any) []map[string]any
	forecast   func(req map[string]any) map[string]any
}

func (f *fakeAds) count(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[method]
}

func (f *fakeAds) client(t *testing.T) *keywordplanner.Client {
	t.Helper()
	f.calls = map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		method := r.URL.Path[strings.LastIndexAny(r.URL.Path, ":/")+1:]
		f.mu.Lock()
		f.calls[method]++
		f.mu.Unlock()
		var resp any
		switch method {
		case "generateKeywordIdeas":
			resp = map[string]any{"results": call(f.ideas, req)}
		case "generateKeywordHistoricalMetrics":
			resp = map[string]any{"results": call(f.historical, req)}
		case "generateKeywordForecastMetrics":
			m := map[string]any{}
			if f.forecast != nil {
				m = f.forecast(req)
			}
			resp = map[string]any{"campaignForecastMetrics": m}
		case "search":
			resp = map[string]any{"results": []any{map[string]any{"customer": map[string]any{"currencyCode": "EUR"}}}}
		default:
			t.Errorf("unexpected API path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return keywordplanner.NewTestClient("dev-token", "123", "", srv.URL, srv.Client(), keywordplanner.WithClock(func() time.Time { return testNow }))
}

func call(fn func(map[string]any) []map[string]any, req map[string]any) []map[string]any {
	if fn == nil {
		return []map[string]any{}
	}
	return fn(req)
}

// metrics builds keywordIdeaMetrics / keywordMetrics with int64s as JSON strings, like the real API.
func metrics(searches int, comp string, low, high, cpc float64, monthly []map[string]any) map[string]any {
	micros := func(v float64) string { return jsonInt(int64(v * 1_000_000)) }
	return map[string]any{
		"avgMonthlySearches": jsonInt(int64(searches)), "competition": comp,
		"lowTopOfPageBidMicros": micros(low), "highTopOfPageBidMicros": micros(high), "averageCpcMicros": micros(cpc),
		"monthlySearchVolumes": monthly,
	}
}

func jsonInt(v int64) string { b, _ := json.Marshal(v); return string(b) }

func idea(text string, m map[string]any, brand string) map[string]any {
	out := map[string]any{"text": text, "keywordIdeaMetrics": m}
	if brand != "" {
		out["keywordAnnotations"] = map[string]any{"concepts": []any{
			map[string]any{"name": brand, "conceptGroup": map[string]any{"name": "Others brands", "type": "OTHER_BRANDS"}},
		}}
	}
	return out
}

func histRow(text string, m map[string]any, closeVariants ...string) map[string]any {
	return map[string]any{"text": text, "keywordMetrics": m, "closeVariants": closeVariants}
}

// series returns Sep 2024..Aug 2026 monthly volumes: base per month, peak in the given months.
func series(base, peak int64, peakMonths ...time.Month) []map[string]any {
	var out []map[string]any
	for i := range 24 {
		d := time.Date(2024, time.September, 1, 0, 0, 0, 0, time.UTC).AddDate(0, i, 0)
		v := base
		if slices.Contains(peakMonths, d.Month()) {
			v = peak
		}
		out = append(out, map[string]any{"year": jsonInt(int64(d.Year())), "month": strings.ToUpper(d.Month().String()), "monthlySearches": jsonInt(v)})
	}
	return out
}

func geoOf(req map[string]any) string {
	if g, ok := req["geoTargetConstants"].([]any); ok && len(g) > 0 {
		return g[0].(string)
	}
	return ""
}

func keywordsOf(req map[string]any) []string {
	var out []string
	raw, _ := req["keywords"].([]any)
	if seed, ok := req["keywordSeed"].(map[string]any); ok {
		raw, _ = seed["keywords"].([]any)
	}
	for _, k := range raw {
		out = append(out, k.(string))
	}
	return out
}

// callTool calls a tool through a real MCP session, including schema validation and middleware.
func callTool(t *testing.T, client *keywordplanner.Client, name string, args map[string]any) (map[string]any, bool) {
	t.Helper()
	ctx := context.Background()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := newServer(client).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	defer serverSession.Close()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	defer session.Close()
	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool %s: %v", name, err)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil && !res.IsError {
		t.Fatalf("result is not JSON: %s", text)
	}
	if out == nil {
		out = map[string]any{"error": text}
	}
	return out, res.IsError
}

func rowsOf(v any) []map[string]any {
	var out []map[string]any
	list, _ := v.([]any)
	for _, r := range list {
		out = append(out, r.(map[string]any))
	}
	return out
}

func texts(rows []map[string]any) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r["text"].(string))
	}
	return out
}

func treppenliftIdeas(map[string]any) []map[string]any {
	return []map[string]any{
		idea("treppenlift", metrics(40000, "HIGH", 1.2, 6.5, 2.4, nil), ""),
		idea("treppenlift mieten", metrics(2400, "HIGH", 2.1, 9.4, 3.2, nil), ""),
		idea("treppenlift kosten", metrics(12000, "HIGH", 1.8, 7.9, 2.9, nil), ""),
		idea("Treppenlift Kosten", metrics(900, "HIGH", 1.5, 6.0, 2.0, nil), ""),
		idea("stannah treppenlift", metrics(5000, "HIGH", 2.0, 9.0, 3.0, nil), "Stannah"),
		idea("treppenlift selber bauen", metrics(700, "LOW", 0.1, 0.4, 0.2, nil), ""),
		idea("treppenlift gebraucht", metrics(3600, "MEDIUM", 0.9, 3.1, 1.2, nil), ""),
	}
}

func TestGenerateKeywordIdeas_ScoresFiltersAndKeepsSeeds(t *testing.T) {
	t.Parallel()
	f := &fakeAds{ideas: treppenliftIdeas}
	client := f.client(t)

	out, isErr := callTool(t, client, "generate_keyword_ideas", map[string]any{
		"seed_keywords": []string{"treppenlift"}, "countries": []string{"DE"}, "min_demand_tier": "strong", "limit": 3,
	})
	if isErr {
		t.Fatalf("tool error: %v", out)
	}
	if out["currencyCode"] != "EUR" {
		t.Errorf("currencyCode = %v, want EUR", out["currencyCode"])
	}
	market := out["market"].(map[string]any)
	if market["language"].(map[string]any)["code"] != "de" || market["label"] != "DE" {
		t.Errorf("market = %v, want de inferred for DE", market)
	}
	rows := rowsOf(out["ideas"])
	got := texts(rows)
	// Seed first, then strong non-brand ideas by afs score; brand, weak and duplicate rows are gone.
	if len(got) != 3 || got[0] != "treppenlift" || !slices.Contains(got, "treppenlift mieten") || !slices.Contains(got, "treppenlift kosten") {
		t.Errorf("ideas = %v", got)
	}
	if slices.Contains(got, "stannah treppenlift") || out["brandFiltered"].(float64) != 1 {
		t.Errorf("brand keyword not filtered: %v brandFiltered=%v", got, out["brandFiltered"])
	}
	for _, r := range rows {
		if r["text"] == "treppenlift kosten" && !slices.Contains(anyStrings(r["closeVariants"]), "Treppenlift Kosten") {
			t.Errorf("duplicate variant not collapsed: %v", r["closeVariants"])
		}
		if r["text"] == "treppenlift mieten" && (r["verdict"] != "launch" || r["demandTier"] != "strong" || r["highTopOfPageBid"].(float64) != 9.4) {
			t.Errorf("treppenlift mieten row = %v", r)
		}
	}
	if f.count("generateKeywordIdeas") != 1 {
		t.Errorf("ideas calls = %d, want 1", f.count("generateKeywordIdeas"))
	}
}

func TestGenerateKeywordIdeas_PerCountryBreakdown(t *testing.T) {
	t.Parallel()
	f := &fakeAds{
		ideas: treppenliftIdeas,
		historical: func(req map[string]any) []map[string]any {
			high := 9.4
			if geoOf(req) == "geoTargetConstants/2040" { // Austria bids lower
				high = 3.0
			}
			var out []map[string]any
			for _, kw := range keywordsOf(req) {
				out = append(out, histRow(kw, metrics(1000, "HIGH", 1.0, high, 2.0, nil)))
			}
			return out
		},
	}
	client := f.client(t)
	out, isErr := callTool(t, client, "generate_keyword_ideas", map[string]any{
		"seed_keywords": []string{"treppenlift"}, "countries": []string{"DE", "AT"}, "per_country": true, "limit": 2,
	})
	if isErr {
		t.Fatalf("tool error: %v", out)
	}
	for _, r := range rowsOf(out["ideas"]) {
		by := r["byCountry"].(map[string]any)
		if by["DE"] == nil || by["AT"] == nil {
			t.Fatalf("byCountry = %v, want DE and AT", by)
		}
		if r["bestCountry"] != "DE" {
			t.Errorf("bestCountry = %v, want DE (higher bids)", r["bestCountry"])
		}
		if by["AT"].(map[string]any)["language"] != "de" {
			t.Errorf("AT language = %v, want de", by["AT"])
		}
	}
	if f.count("generateKeywordHistoricalMetrics") != 2 {
		t.Errorf("historical calls = %d, want one per country", f.count("generateKeywordHistoricalMetrics"))
	}
}

func TestGenerateKeywordIdeas_InvalidCountry_IsError(t *testing.T) {
	t.Parallel()
	f := &fakeAds{}
	out, isErr := callTool(t, f.client(t), "generate_keyword_ideas", map[string]any{"seed_keywords": []string{"x"}, "countries": []string{"Atlantis"}})
	if !isErr || !strings.Contains(out["error"].(string), "unknown country") {
		t.Errorf("want unknown-country tool error, got %v (isError=%v)", out, isErr)
	}
}

func TestGetHistoricalMetrics_MatchesCloseVariantsAndBrands(t *testing.T) {
	t.Parallel()
	f := &fakeAds{
		ideas: func(map[string]any) []map[string]any {
			return []map[string]any{idea("stannah stairlift", metrics(900, "HIGH", 2, 8, 3, nil), "Stannah")}
		},
		historical: func(map[string]any) []map[string]any {
			return []map[string]any{
				histRow("stairlift rental", metrics(1900, "HIGH", 2.5, 11.2, 4.1, series(1500, 2500, time.November)), "stairlift rentals"),
				histRow("stannah stairlift", metrics(900, "HIGH", 2, 8, 3, nil)),
			}
		},
	}
	out, isErr := callTool(t, f.client(t), "get_historical_metrics", map[string]any{
		"keywords": []string{"stairlift rentals", "stannah stairlift", "unknown thing"}, "countries": []string{"US"},
	})
	if isErr {
		t.Fatalf("tool error: %v", out)
	}
	rows := rowsOf(out["keywords"])
	if len(rows) != 2 {
		t.Fatalf("rows = %v", texts(rows))
	}
	if rows[0]["text"] != "stairlift rentals" || rows[0]["matchedAs"] != "stairlift rental" || rows[0]["demandTier"] != "strong" {
		t.Errorf("close variant row = %v", rows[0])
	}
	if rows[0]["trend"] == nil || len(rows[0]["monthlySearchVolumes"].([]any)) != 24 {
		t.Errorf("trend or monthly volumes missing: %v", rows[0]["trend"])
	}
	if brand := rows[1]["brand"].(map[string]any); brand["isBrand"] != true || rows[1]["verdict"] != "skip" {
		t.Errorf("brand row = %v", rows[1])
	}
	if nf := anyStrings(out["notFound"]); len(nf) != 1 || nf[0] != "unknown thing" {
		t.Errorf("notFound = %v", nf)
	}
}

func TestGetKeywordForecast_PerKeywordAndTotal(t *testing.T) {
	t.Parallel()
	f := &fakeAds{forecast: func(req map[string]any) map[string]any {
		groups := req["campaign"].(map[string]any)["adGroups"].([]any)
		n := len(groups[0].(map[string]any)["biddableKeywords"].([]any))
		return map[string]any{"impressions": 1000 * n, "clicks": 50 * n, "clickThroughRate": 0.05, "averageCpcMicros": "1500000", "costMicros": jsonInt(int64(75_000_000 * n))}
	}}
	out, isErr := callTool(t, f.client(t), "get_keyword_forecast", map[string]any{
		"keywords": []string{"a", "b"}, "max_cpc": 2.5, "countries": []string{"DE"}, "match_type": "PHRASE",
	})
	if isErr {
		t.Fatalf("tool error: %v", out)
	}
	if out["total"].(map[string]any)["clicks"].(float64) != 100 || out["maxCpcMicros"].(float64) != 2_500_000 || out["matchType"] != "PHRASE" {
		t.Errorf("forecast = %v", out)
	}
	rows := rowsOf(out["keywords"])
	if len(rows) != 2 || rows[0]["clicks"].(float64) != 50 || rows[1]["cost"].(float64) != 75 || rows[0]["averageCpc"].(float64) != 1.5 {
		t.Errorf("per-keyword rows = %v", rows)
	}
	if f.count("generateKeywordForecastMetrics") != 3 {
		t.Errorf("forecast calls = %d, want 3 (total + one per keyword)", f.count("generateKeywordForecastMetrics"))
	}
}

func TestScoreTopics_PicksBestNonBrandVariant(t *testing.T) {
	t.Parallel()
	f := &fakeAds{
		ideas: func(req map[string]any) []map[string]any {
			switch keywordsOf(req)[0] {
			case "treppenlift":
				return treppenliftIdeas(req)
			default: // a curiosity topic: no bids, and the seed itself is not returned
				return []map[string]any{idea("lustige katzenvideos", metrics(90000, "LOW", 0, 0, 0, nil), "")}
			}
		},
		historical: func(req map[string]any) []map[string]any {
			var out []map[string]any
			for _, kw := range keywordsOf(req) {
				out = append(out, histRow(kw, metrics(20000, "LOW", 0.05, 0.2, 0.1, nil)))
			}
			return out
		},
	}
	out, isErr := callTool(t, f.client(t), "score_topics", map[string]any{
		"topics": []string{"katzenvideos", "treppenlift"}, "countries": []string{"DE"}, "alternatives": 2,
	})
	if isErr {
		t.Fatalf("tool error: %v", out)
	}
	topics := rowsOf(out["topics"])
	if len(topics) != 2 || topics[0]["topic"] != "treppenlift" {
		t.Fatalf("topics not ranked best first: %v", out["topics"])
	}
	tl := topics[0]
	// "kosten" wins on volume (12,000 vs 2,400) at a similar CPC.
	if tl["offerName"] != "treppenlift kosten" || tl["verdict"] != "launch" {
		t.Errorf("treppenlift best = %v verdict %v", tl["offerName"], tl["verdict"])
	}
	if ev := tl["bidderEvidence"].(string); !strings.Contains(ev, "EUR") || !strings.Contains(ev, "12,000/mo") {
		t.Errorf("bidderEvidence = %q", ev)
	}
	for _, alt := range rowsOf(tl["alternatives"]) {
		if strings.Contains(alt["text"].(string), "stannah") {
			t.Errorf("brand variant offered as alternative: %v", alt["text"])
		}
	}
	if len(rowsOf(tl["alternatives"])) != 2 {
		t.Errorf("alternatives = %v, want 2", texts(rowsOf(tl["alternatives"])))
	}
	cats := topics[1]
	if cats["verdict"] != "skip" {
		t.Errorf("curiosity topic verdict = %v, want skip", cats["verdict"])
	}
	if cats["headTerm"] == nil {
		t.Error("missing head term should be looked up via historical metrics")
	}
	if out["launchable"].(float64) != 1 {
		t.Errorf("launchable = %v, want 1", out["launchable"])
	}
}

func TestFindOfferVariants_RecommendsStrongestVariant(t *testing.T) {
	t.Parallel()
	f := &fakeAds{
		ideas: treppenliftIdeas,
		historical: func(req map[string]any) []map[string]any {
			var out []map[string]any
			for _, kw := range keywordsOf(req) {
				switch kw {
				case "treppenlift":
					out = append(out, histRow(kw, metrics(40000, "HIGH", 1.2, 6.5, 2.4, nil)))
				case "treppenlift mieten":
					out = append(out, histRow(kw, metrics(2400, "HIGH", 2.1, 14.0, 5.5, nil)))
				case "treppenlift förderung":
					out = append(out, histRow(kw, metrics(8100, "HIGH", 1.9, 8.2, 3.3, nil)))
				case "treppenlift kosten":
					out = append(out, histRow(kw, metrics(12000, "HIGH", 1.8, 7.9, 2.9, nil)))
				}
			}
			return out
		},
	}
	out, isErr := callTool(t, f.client(t), "find_offer_variants", map[string]any{"topic": "treppenlift", "countries": []string{"DE"}})
	if isErr {
		t.Fatalf("tool error: %v", out)
	}
	rec := out["recommendation"].(map[string]any)
	if rec["offerName"] != "treppenlift mieten" || rec["runnerUp"] == "" || rec["runnerUp"] == nil {
		t.Errorf("recommendation = %v", rec)
	}
	variants := rowsOf(out["variants"])
	var sources []string
	for _, v := range variants {
		sources = append(sources, v["source"].(string))
	}
	if !slices.Contains(sources, "template") {
		t.Errorf("no template variants found: %v", sources)
	}
	if out["headTerm"] == nil {
		t.Error("head term missing")
	}
}

func TestCompareKeywordMarkets_RanksMarkets(t *testing.T) {
	t.Parallel()
	f := &fakeAds{historical: func(req map[string]any) []map[string]any {
		high := map[string]float64{"geoTargetConstants/2840": 12, "geoTargetConstants/2826": 4, "geoTargetConstants/2124": 0.5}[geoOf(req)]
		var out []map[string]any
		for _, kw := range keywordsOf(req) {
			out = append(out, histRow(kw, metrics(5000, "HIGH", 1, high, high/2, nil)))
		}
		return out
	}}
	out, isErr := callTool(t, f.client(t), "compare_keyword_markets", map[string]any{
		"keywords": []string{"stairlift rental", "hospital bed rental"},
		"markets":  []map[string]any{{"countries": []string{"CA"}}, {"countries": []string{"US"}}, {"label": "UK", "countries": []string{"GB"}}},
	})
	if isErr {
		t.Fatalf("tool error: %v", out)
	}
	ranking := rowsOf(out["marketRanking"])
	if len(ranking) != 3 || ranking[0]["label"] != "US" || ranking[2]["label"] != "CA" {
		t.Errorf("ranking = %v", ranking)
	}
	for _, k := range rowsOf(out["keywords"]) {
		if k["bestMarket"] != "US" {
			t.Errorf("%v bestMarket = %v, want US", k["text"], k["bestMarket"])
		}
		if k["markets"].(map[string]any)["UK"] == nil {
			t.Errorf("custom label missing: %v", k["markets"])
		}
	}
}

func TestFindSeasonalOpportunities_PicksWindowPeaks(t *testing.T) {
	t.Parallel()
	f := &fakeAds{ideas: func(map[string]any) []map[string]any {
		return []map[string]any{
			// Window for 2026-09-29 + 14..42 days is Oct 13 - Nov 10: peaks in Oct/Nov qualify.
			idea("winterreifen kaufen", metrics(20000, "HIGH", 0.8, 2.6, 1.1, series(5000, 40000, time.October, time.November)), ""),
			idea("sommerreifen kaufen", metrics(20000, "HIGH", 0.8, 2.6, 1.1, series(5000, 40000, time.April, time.May)), ""),
			idea("reifen wechseln anleitung", metrics(20000, "LOW", 0.1, 0.3, 0.1, series(5000, 40000, time.October)), ""),
		}
	}}
	out, isErr := callTool(t, f.client(t), "find_seasonal_opportunities", map[string]any{
		"seed_keywords": []string{"reifen"}, "countries": []string{"DE"}, "min_demand_tier": "weak",
	})
	if isErr {
		t.Fatalf("tool error: %v", out)
	}
	got := texts(rowsOf(out["opportunities"]))
	if len(got) != 1 || got[0] != "winterreifen kaufen" {
		t.Errorf("opportunities = %v, want only the in-window commercial keyword", got)
	}
	if out["windowFrom"] != "2026-10-13" || out["windowTo"] != "2026-11-10" {
		t.Errorf("window = %v..%v", out["windowFrom"], out["windowTo"])
	}
}

func TestGetTargetingReference_NoAPICalls(t *testing.T) {
	t.Parallel()
	f := &fakeAds{}
	out, isErr := callTool(t, f.client(t), "get_targeting_reference", map[string]any{})
	if isErr || len(rowsOf(out["languages"])) < 10 || out["countryPresets"].(map[string]any)["DACH"] == nil {
		t.Errorf("reference = %v", out)
	}
	if f.count("generateKeywordIdeas")+f.count("search") != 0 {
		t.Error("targeting reference must not call the API")
	}
}

func anyStrings(v any) []string {
	var out []string
	list, _ := v.([]any)
	for _, s := range list {
		out = append(out, s.(string))
	}
	return out
}
