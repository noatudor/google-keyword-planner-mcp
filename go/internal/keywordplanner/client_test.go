package keywordplanner_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ncosentino/google-keyword-planner-mcp/go/internal/keywordplanner"
)

var fixedNow = time.Date(2026, time.September, 29, 10, 0, 0, 0, time.UTC)

// captureServer records the last request body and answers with body.
func captureServer(t *testing.T, status int, body string, captured *[]byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if captured != nil {
			*captured, _ = io.ReadAll(r.Body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestNewClient_NotNil(t *testing.T) {
	t.Parallel()
	if keywordplanner.NewClient("dev-token", "client-id", "client-secret", "refresh-token", "1234567890", "") == nil {
		t.Fatal("NewClient returned nil")
	}
	if keywordplanner.NewClient("dev-token", "id", "secret", "refresh", "1234567890", "9876543210",
		keywordplanner.WithMaxQPS(0), keywordplanner.WithCacheTTL(0)) == nil {
		t.Fatal("NewClient with options returned nil")
	}
}

// TestKeywordIdeas_SendsLoginCustomerIDHeader verifies the login-customer-id header
// is included when a manager account ID is configured, and omitted otherwise.
func TestKeywordIdeas_LoginCustomerIDHeader(t *testing.T) {
	t.Parallel()
	for _, loginID := range []string{"1381404200", ""} {
		var got string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = r.Header.Get("login-customer-id")
			_, _ = w.Write([]byte(`{"results":[]}`))
		}))
		client := keywordplanner.NewTestClient("dev-token", "3778350596", loginID, srv.URL, srv.Client())
		_, _ = client.KeywordIdeas(context.Background(), keywordplanner.IdeasRequest{Seeds: []string{"go"}})
		srv.Close()
		if got != loginID {
			t.Errorf("login-customer-id header = %q, want %q", got, loginID)
		}
	}
}

// TestHistoricalMetrics_ParsesV23Results is the regression test for the tool that always
// returned zero rows: v23 answers with "results" (not "metrics"), and every int64 is a JSON string.
func TestHistoricalMetrics_ParsesV23Results(t *testing.T) {
	t.Parallel()
	srv := captureServer(t, http.StatusOK, `{
		"results": [{
			"text": "wärmepumpe finanzieren",
			"closeVariants": ["wärmepumpen finanzieren"],
			"keywordMetrics": {
				"avgMonthlySearches": "1600",
				"competition": "HIGH",
				"competitionIndex": "87",
				"lowTopOfPageBidMicros": "2850000",
				"highTopOfPageBidMicros": "12810000",
				"averageCpcMicros": "3100000",
				"monthlySearchVolumes": [{"year": "2026", "month": "AUGUST", "monthlySearches": "1900"}]
			}
		}],
		"aggregateMetricResults": {"deviceSearches": [{"device": "MOBILE", "searchCount": "1200"}, {"device": "DESKTOP", "searchCount": "400"}]}
	}`, nil)
	client := keywordplanner.NewTestClient("dev-token", "123", "", srv.URL, srv.Client())

	res, err := client.HistoricalMetrics(context.Background(), []string{"wärmepumpe finanzieren"}, keywordplanner.Targeting{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Keywords) != 1 {
		t.Fatalf("got %d keywords, want 1", len(res.Keywords))
	}
	k := res.Keywords[0]
	if k.AvgMonthlySearches != 1600 || k.CompetitionIndex != 87 || k.HighTopOfPageBidMicros != 12_810_000 || k.AverageCpcMicros != 3_100_000 {
		t.Errorf("metrics parsed wrong: %+v", k)
	}
	if len(k.MonthlySearchVolumes) != 1 || k.MonthlySearchVolumes[0] != (keywordplanner.MonthlyVolume{Year: 2026, Month: 8, MonthlySearches: 1900}) {
		t.Errorf("monthly volumes parsed wrong: %+v", k.MonthlySearchVolumes)
	}
	if len(k.CloseVariants) != 1 {
		t.Errorf("close variants missing: %+v", k.CloseVariants)
	}
	if res.DeviceSearches["MOBILE"] != 1200 {
		t.Errorf("device searches parsed wrong: %+v", res.DeviceSearches)
	}
}

// TestHistoricalMetrics_SendsTargetingAndHistoryOptions verifies language, geo, network,
// average CPC and a 24-month range ending last month are requested.
func TestHistoricalMetrics_SendsTargetingAndHistoryOptions(t *testing.T) {
	t.Parallel()
	var body []byte
	srv := captureServer(t, http.StatusOK, `{"results":[]}`, &body)
	client := keywordplanner.NewTestClient("dev-token", "123", "", srv.URL, srv.Client(), keywordplanner.WithClock(func() time.Time { return fixedNow }))

	_, err := client.HistoricalMetrics(context.Background(), []string{"x"}, keywordplanner.Targeting{
		Language: "languageConstants/1001", GeoTargets: []string{"geoTargetConstants/2276"}, Network: "GOOGLE_SEARCH",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var req struct {
		Language           string   `json:"language"`
		GeoTargetConstants []string `json:"geoTargetConstants"`
		KeywordPlanNetwork string   `json:"keywordPlanNetwork"`
		Options            struct {
			IncludeAverageCpc bool `json:"includeAverageCpc"`
			Range             struct {
				Start struct {
					Year  int    `json:"year"`
					Month string `json:"month"`
				} `json:"start"`
				End struct {
					Year  int    `json:"year"`
					Month string `json:"month"`
				} `json:"end"`
			} `json:"yearMonthRange"`
		} `json:"historicalMetricsOptions"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("parse request: %v", err)
	}
	if req.Language != "languageConstants/1001" || len(req.GeoTargetConstants) != 1 || req.KeywordPlanNetwork != "GOOGLE_SEARCH" {
		t.Errorf("targeting not sent: %s", body)
	}
	if !req.Options.IncludeAverageCpc {
		t.Error("includeAverageCpc not requested")
	}
	if req.Options.Range.End.Year != 2026 || req.Options.Range.End.Month != "AUGUST" ||
		req.Options.Range.Start.Year != 2024 || req.Options.Range.Start.Month != "SEPTEMBER" {
		t.Errorf("history range = %+v, want 2024-09..2026-08", req.Options.Range)
	}
}

// TestKeywordIdeas_RequestAndAnnotations verifies geo targets, brand annotations and the
// device aggregate are requested, and concept annotations are parsed.
func TestKeywordIdeas_RequestAndAnnotations(t *testing.T) {
	t.Parallel()
	var body []byte
	srv := captureServer(t, http.StatusOK, `{"results":[{
		"text": "stannah treppenlift",
		"keywordIdeaMetrics": {"avgMonthlySearches": "880", "competition": "MEDIUM"},
		"keywordAnnotations": {"concepts": [{"name": "stannah", "conceptGroup": {"name": "Others brands", "type": "OTHER_BRANDS"}}]}
	}], "totalSize": "1"}`, &body)
	client := keywordplanner.NewTestClient("dev-token", "123", "", srv.URL, srv.Client())

	res, err := client.KeywordIdeas(context.Background(), keywordplanner.IdeasRequest{
		Seeds: []string{"treppenlift"}, Targeting: keywordplanner.Targeting{GeoTargets: []string{"geoTargetConstants/2276"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(body)
	for _, want := range []string{`"geoTargetConstants":["geoTargetConstants/2276"]`, `"keywordAnnotation":["KEYWORD_CONCEPT"]`,
		`"aggregateMetricTypes":["DEVICE"]`, `"keywordSeed":{"keywords":["treppenlift"]}`, `"includeAdultKeywords":false`} {
		if !strings.Contains(s, want) {
			t.Errorf("request body %s missing %s", s, want)
		}
	}
	if len(res.Ideas) != 1 || len(res.Ideas[0].Concepts) != 1 || res.Ideas[0].Concepts[0].GroupType != "OTHER_BRANDS" {
		t.Errorf("concepts not parsed: %+v", res.Ideas)
	}
	if res.TotalSize != 1 {
		t.Errorf("TotalSize = %d, want 1", res.TotalSize)
	}
}

func TestKeywordIdeas_SiteSeed(t *testing.T) {
	t.Parallel()
	var body []byte
	srv := captureServer(t, http.StatusOK, `{"results":[]}`, &body)
	client := keywordplanner.NewTestClient("dev-token", "123", "", srv.URL, srv.Client())
	_, _ = client.KeywordIdeas(context.Background(), keywordplanner.IdeasRequest{Site: "example.com"})
	if !strings.Contains(string(body), `"siteSeed":{"site":"example.com"}`) {
		t.Errorf("site seed not sent: %s", body)
	}
}

// TestForecast_UsesV23Shape is the regression test for the HTTP 400 "Unknown name
// campaignForecastSpec": v23 wants campaign + forecastPeriod, starting in the future.
func TestForecast_UsesV23Shape(t *testing.T) {
	t.Parallel()
	var body []byte
	srv := captureServer(t, http.StatusOK, `{"campaignForecastMetrics": {
		"impressions": 1200.5, "clickThroughRate": 0.05, "averageCpcMicros": "1800000", "clicks": 60.2, "costMicros": "108000000"
	}}`, &body)
	client := keywordplanner.NewTestClient("dev-token", "123", "", srv.URL, srv.Client(), keywordplanner.WithClock(func() time.Time { return fixedNow }))

	m, err := client.Forecast(context.Background(), keywordplanner.ForecastRequest{
		Keywords: []string{"treppenlift mieten"}, Days: 30,
		Targeting: keywordplanner.Targeting{Language: "languageConstants/1001", GeoTargets: []string{"geoTargetConstants/2276"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Clicks != 60.2 || m.CostMicros != 108_000_000 || m.AverageCpcMicros != 1_800_000 || m.CTR != 0.05 {
		t.Errorf("metrics parsed wrong: %+v", m)
	}

	var req map[string]any
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("parse request: %v", err)
	}
	if _, ok := req["campaignForecastSpec"]; ok {
		t.Error("request still uses the removed campaignForecastSpec field")
	}
	period := req["forecastPeriod"].(map[string]any)
	if period["startDate"] != "2026-09-30" || period["endDate"] != "2026-10-29" {
		t.Errorf("forecast period = %v, want 2026-09-30..2026-10-29", period)
	}
	campaign := req["campaign"].(map[string]any)
	if campaign["keywordPlanNetwork"] != "GOOGLE_SEARCH" {
		t.Errorf("keywordPlanNetwork = %v", campaign["keywordPlanNetwork"])
	}
	bid := campaign["biddingStrategy"].(map[string]any)["manualCpcBiddingStrategy"].(map[string]any)["maxCpcBidMicros"]
	if bid != "1000000" {
		t.Errorf("maxCpcBidMicros = %v, want default 1000000", bid)
	}
	kw := campaign["adGroups"].([]any)[0].(map[string]any)["biddableKeywords"].([]any)[0].(map[string]any)["keyword"].(map[string]any)
	if kw["text"] != "treppenlift mieten" || kw["matchType"] != "BROAD" {
		t.Errorf("biddable keyword = %v", kw)
	}
	if geo := campaign["geoModifiers"].([]any)[0].(map[string]any)["geoTargetConstant"]; geo != "geoTargetConstants/2276" {
		t.Errorf("geo modifier = %v", geo)
	}
}

// TestPost_ReturnsFullErrorBody verifies that unparseable API error bodies are not truncated.
func TestPost_ReturnsFullErrorBody(t *testing.T) {
	t.Parallel()
	longBody := strings.Repeat("x", 500)
	srv := captureServer(t, http.StatusBadRequest, longBody, nil)
	client := keywordplanner.NewTestClient("dev-token", "123", "", srv.URL, srv.Client())
	_, err := client.KeywordIdeas(context.Background(), keywordplanner.IdeasRequest{Seeds: []string{"test"}})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if len(err.Error()) < 500 {
		t.Errorf("error message appears truncated: len=%d, want >= 500", len(err.Error()))
	}
}

// TestPost_ParsesGoogleAdsError verifies Google's error JSON becomes a short, readable message.
func TestPost_ParsesGoogleAdsError(t *testing.T) {
	t.Parallel()
	srv := captureServer(t, http.StatusBadRequest, `{"error": {"code": 400, "message": "Request contains an invalid argument.",
		"status": "INVALID_ARGUMENT", "details": [{"errors": [{"errorCode": {"keywordPlanIdeaError": "INVALID_VALUE"},
		"message": "The input has an invalid value."}], "requestId": "abc123"}]}}`, nil)
	client := keywordplanner.NewTestClient("dev-token", "123", "", srv.URL, srv.Client())
	_, err := client.KeywordIdeas(context.Background(), keywordplanner.IdeasRequest{Seeds: []string{"x"}})
	var apiErr *keywordplanner.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v is not an *APIError", err)
	}
	msg := err.Error()
	for _, want := range []string{"INVALID_ARGUMENT", "keywordPlanIdeaError=INVALID_VALUE", "The input has an invalid value.", "abc123"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	if apiErr.Retryable() {
		t.Error("INVALID_ARGUMENT must not be retryable")
	}
}

// TestPost_RetriesOnceOnQuotaError verifies a 429 is retried exactly once.
func TestPost_RetriesOnceOnQuotaError(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error": {"code": 429, "message": "Resource has been exhausted", "status": "RESOURCE_EXHAUSTED"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[{"text":"ok","keywordIdeaMetrics":{}}]}`))
	}))
	defer srv.Close()
	client := keywordplanner.NewTestClient("dev-token", "123", "", srv.URL, srv.Client())
	res, err := client.KeywordIdeas(context.Background(), keywordplanner.IdeasRequest{Seeds: []string{"x"}})
	if err != nil {
		t.Fatalf("unexpected error after retry: %v", err)
	}
	if calls.Load() != 2 || len(res.Ideas) != 1 {
		t.Errorf("calls = %d, ideas = %d; want 2 calls and 1 idea", calls.Load(), len(res.Ideas))
	}
}

// TestPost_CachesIdenticalRequests verifies a repeated request is served from the cache.
func TestPost_CachesIdenticalRequests(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"results":[]}`))
	}))
	defer srv.Close()
	client := keywordplanner.NewTestClient("dev-token", "123", "", srv.URL, srv.Client(), keywordplanner.WithCacheTTL(time.Hour))
	ctx := context.Background()
	for range 3 {
		if _, err := client.KeywordIdeas(ctx, keywordplanner.IdeasRequest{Seeds: []string{"same"}}); err != nil {
			t.Fatal(err)
		}
	}
	_, _ = client.KeywordIdeas(ctx, keywordplanner.IdeasRequest{Seeds: []string{"different"}})
	if calls.Load() != 2 {
		t.Errorf("API calls = %d, want 2 (one per distinct request)", calls.Load())
	}
}

// TestPost_CacheRespectsByteBudget verifies responses too large for the budget are not cached.
func TestPost_CacheRespectsByteBudget(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	big := `{"results":[{"text":"` + strings.Repeat("x", 4096) + `","keywordIdeaMetrics":{}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(big))
	}))
	defer srv.Close()
	client := keywordplanner.NewTestClient("dev-token", "123", "", srv.URL, srv.Client(),
		keywordplanner.WithCacheTTL(time.Hour), keywordplanner.WithCacheMaxBytes(8192))
	for range 2 {
		if _, err := client.KeywordIdeas(context.Background(), keywordplanner.IdeasRequest{Seeds: []string{"x"}}); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 2 {
		t.Errorf("API calls = %d, want 2: a response over a quarter of the budget must not be cached", calls.Load())
	}
}

// TestCurrencyCode_CachedOnSuccessAndSoftOnFailure verifies the currency lookup is cached and
// that a failed lookup returns "" instead of an error.
func TestCurrencyCode_CachedOnSuccessAndSoftOnFailure(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !strings.HasSuffix(r.URL.Path, "/googleAds:search") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"results":[{"customer":{"resourceName":"customers/123","currencyCode":"EUR"}}]}`))
	}))
	defer srv.Close()
	client := keywordplanner.NewTestClient("dev-token", "123", "", srv.URL, srv.Client())
	if c := client.CurrencyCode(context.Background()); c != "EUR" {
		t.Errorf("currency = %q, want EUR", c)
	}
	_ = client.CurrencyCode(context.Background())
	if calls.Load() != 1 {
		t.Errorf("currency lookups = %d, want 1 (cached)", calls.Load())
	}

	failing := captureServer(t, http.StatusForbidden, `{"error":{"code":403,"message":"denied","status":"PERMISSION_DENIED"}}`, nil)
	if c := keywordplanner.NewTestClient("dev-token", "123", "", failing.URL, failing.Client()).CurrencyCode(context.Background()); c != "" {
		t.Errorf("currency on failure = %q, want empty", c)
	}
}
