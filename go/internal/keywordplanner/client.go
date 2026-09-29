package keywordplanner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/time/rate"
)

const (
	tokenURL    = "https://oauth2.googleapis.com/token"
	adsAPIBase  = "https://googleads.googleapis.com/v23"
	httpTimeout = 60 * time.Second

	// DefaultMaxQPS is the default client-side request rate. Keyword Planning
	// services have much tighter quotas than the rest of the Google Ads API.
	DefaultMaxQPS = 1.0
	// DefaultCacheTTL is how long identical planner requests are served from memory.
	// Planner data is refreshed monthly, so a long TTL is safe.
	DefaultCacheTTL = 12 * time.Hour

	// DefaultCacheMaxBytes bounds the response cache. Idea responses with 24 months of
	// history for thousands of ideas are several MB each.
	DefaultCacheMaxBytes = 64 << 20

	defaultRetryDelay = 2 * time.Second
	historyMonths     = 24
)

// Client calls the Google Ads Keyword Planner API.
type Client struct {
	httpClient      *http.Client
	developerToken  string
	customerID      string
	loginCustomerID string
	baseURL         string

	limiter    *rate.Limiter
	cache      *responseCache
	retryDelay time.Duration
	now        func() time.Time

	currencyMu   sync.Mutex
	currencyCode string
}

// Option configures a Client.
type Option func(*Client)

// WithMaxQPS limits outgoing API requests to qps per second. Zero or negative disables limiting.
func WithMaxQPS(qps float64) Option {
	return func(c *Client) {
		if qps <= 0 {
			c.limiter = nil
			return
		}
		c.limiter = rate.NewLimiter(rate.Limit(qps), 3)
	}
}

// WithCacheTTL caches identical planner requests for ttl. Zero or negative disables caching.
func WithCacheTTL(ttl time.Duration) Option {
	return func(c *Client) {
		if ttl <= 0 {
			c.cache = nil
			return
		}
		maxBytes := DefaultCacheMaxBytes
		if c.cache != nil {
			maxBytes = c.cache.maxBytes
		}
		c.cache = newResponseCache(ttl, maxBytes)
	}
}

// WithCacheMaxBytes bounds the response cache size. Zero or negative disables caching.
func WithCacheMaxBytes(n int) Option {
	return func(c *Client) {
		if n <= 0 {
			c.cache = nil
			return
		}
		ttl := DefaultCacheTTL
		if c.cache != nil {
			ttl = c.cache.ttl
		}
		c.cache = newResponseCache(ttl, n)
	}
}

// WithRetryDelay sets the wait before the single retry of a quota-limited request.
func WithRetryDelay(d time.Duration) Option {
	return func(c *Client) { c.retryDelay = d }
}

// WithClock overrides the clock used for forecast periods and history ranges. Intended for tests.
func WithClock(now func() time.Time) Option {
	return func(c *Client) { c.now = now }
}

// NewClient creates a Client with the provided OAuth2 credentials.
// loginCustomerID is the manager/MCC account ID; set it when customerID is a sub-account.
func NewClient(developerToken, clientID, clientSecret, refreshToken, customerID, loginCustomerID string, opts ...Option) *Client {
	return NewClientWithBaseURL(developerToken, clientID, clientSecret, refreshToken, customerID, loginCustomerID, adsAPIBase, opts...)
}

// NewClientWithBaseURL creates a Client with a custom API base URL. Intended for testing.
func NewClientWithBaseURL(developerToken, clientID, clientSecret, refreshToken, customerID, loginCustomerID, baseURL string, opts ...Option) *Client {
	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     oauth2.Endpoint{TokenURL: tokenURL},
	}
	ts := conf.TokenSource(context.Background(), &oauth2.Token{RefreshToken: refreshToken})
	httpClient := oauth2.NewClient(context.Background(), ts)
	httpClient.Timeout = httpTimeout

	c := &Client{
		httpClient:      httpClient,
		developerToken:  developerToken,
		customerID:      customerID,
		loginCustomerID: loginCustomerID,
		baseURL:         baseURL,
		retryDelay:      defaultRetryDelay,
		now:             time.Now,
	}
	WithMaxQPS(DefaultMaxQPS)(c)
	WithCacheTTL(DefaultCacheTTL)(c)
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// NewTestClient creates a Client that uses a plain http.Client (no OAuth2), with no rate
// limiting, no cache and no retry delay unless options say otherwise.
// Do not use in production code.
func NewTestClient(developerToken, customerID, loginCustomerID, baseURL string, httpClient *http.Client, opts ...Option) *Client {
	c := &Client{
		httpClient:      httpClient,
		developerToken:  developerToken,
		customerID:      customerID,
		loginCustomerID: loginCustomerID,
		baseURL:         baseURL,
		now:             time.Now,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Now returns the client's clock, so callers score trends against the same date the
// history range was requested for.
func (c *Client) Now() time.Time { return c.now() }

// KeywordIdeas returns keyword ideas with 24 months of history, average CPC and
// brand/non-brand concept annotations.
func (c *Client) KeywordIdeas(ctx context.Context, req IdeasRequest) (*IdeasResult, error) {
	body := generateKeywordIdeasRequest{
		Language:                 req.Language,
		GeoTargetConstants:       req.GeoTargets,
		KeywordPlanNetwork:       req.Network,
		KeywordAnnotation:        []string{"KEYWORD_CONCEPT"},
		AggregateMetrics:         &aggregateMetrics{AggregateMetricTypes: []string{"DEVICE"}},
		HistoricalMetricsOptions: c.historyOptions(),
	}
	switch {
	case req.Site != "":
		body.SiteSeed = &siteSeed{Site: req.Site}
	case len(req.Seeds) > 0 && req.URL != "":
		body.KeywordAndURLSeed = &keywordAndURLSeed{URL: req.URL, Keywords: req.Seeds}
	case req.URL != "":
		body.URLSeed = &urlSeed{URL: req.URL}
	default:
		body.KeywordSeed = &keywordSeed{Keywords: req.Seeds}
	}

	var raw generateKeywordIdeasResponse
	if err := c.post(ctx, c.customerEndpoint("generateKeywordIdeas"), body, &raw, true); err != nil {
		return nil, err
	}
	ideas := make([]KeywordData, 0, len(raw.Results))
	for _, r := range raw.Results {
		ideas = append(ideas, r.KeywordIdeaMetrics.toData(r.Text, r.CloseVariants, r.KeywordAnnotations))
	}
	return &IdeasResult{Ideas: ideas, TotalSize: int64(raw.TotalSize), DeviceSearches: raw.AggregateMetricResults.deviceMap()}, nil
}

// HistoricalMetrics returns exact-keyword metrics with 24 months of history and average CPC.
// Close variants are grouped by the API: one result can cover several requested keywords.
func (c *Client) HistoricalMetrics(ctx context.Context, keywords []string, t Targeting) (*HistoricalResult, error) {
	body := generateHistoricalMetricsRequest{
		Keywords:                 keywords,
		Language:                 t.Language,
		GeoTargetConstants:       t.GeoTargets,
		KeywordPlanNetwork:       t.Network,
		AggregateMetrics:         &aggregateMetrics{AggregateMetricTypes: []string{"DEVICE"}},
		HistoricalMetricsOptions: c.historyOptions(),
	}
	var raw generateHistoricalMetricsResponse
	if err := c.post(ctx, c.customerEndpoint("generateKeywordHistoricalMetrics"), body, &raw, true); err != nil {
		return nil, err
	}
	out := make([]KeywordData, 0, len(raw.Results))
	for _, r := range raw.Results {
		out = append(out, r.KeywordMetrics.toData(r.Text, r.CloseVariants, nil))
	}
	return &HistoricalResult{Keywords: out, DeviceSearches: raw.AggregateMetricResults.deviceMap()}, nil
}

// Forecast returns campaign-level forecast totals for one simulated manual-CPC campaign.
func (c *Client) Forecast(ctx context.Context, req ForecastRequest) (*ForecastMetrics, error) {
	if req.Days <= 0 {
		req.Days = 30
	}
	if req.MaxCPCBidMicros <= 0 {
		req.MaxCPCBidMicros = 1_000_000
	}
	if req.MatchType == "" {
		req.MatchType = "BROAD"
	}
	network := req.Network
	if network == "" {
		network = "GOOGLE_SEARCH"
	}
	// The forecast period must start in the future.
	start := c.now().UTC().AddDate(0, 0, 1)
	end := start.AddDate(0, 0, req.Days-1)

	keywords := make([]biddableKeyword, 0, len(req.Keywords))
	for _, kw := range req.Keywords {
		keywords = append(keywords, biddableKeyword{Keyword: keywordInfo{Text: kw, MatchType: req.MatchType}})
	}
	campaign := campaignToForecast{
		KeywordPlanNetwork: network,
		BiddingStrategy: campaignBiddingStrategy{ManualCpcBiddingStrategy: manualCpcBiddingStrategy{
			MaxCpcBidMicros: strconv.FormatInt(req.MaxCPCBidMicros, 10),
		}},
		AdGroups: []forecastAdGroup{{BiddableKeywords: keywords}},
	}
	if req.DailyBudgetMicros > 0 {
		campaign.BiddingStrategy.ManualCpcBiddingStrategy.DailyBudgetMicros = strconv.FormatInt(req.DailyBudgetMicros, 10)
	}
	if req.Language != "" {
		campaign.LanguageConstants = []string{req.Language}
	}
	for _, g := range req.GeoTargets {
		campaign.GeoModifiers = append(campaign.GeoModifiers, criterionBidModifier{GeoTargetConstant: g})
	}
	body := generateForecastMetricsRequest{
		ForecastPeriod: dateRange{StartDate: start.Format("2006-01-02"), EndDate: end.Format("2006-01-02")},
		Campaign:       campaign,
	}

	var raw generateForecastMetricsResponse
	if err := c.post(ctx, c.customerEndpoint("generateKeywordForecastMetrics"), body, &raw, true); err != nil {
		return nil, err
	}
	m := raw.CampaignForecastMetrics
	if m == nil {
		return &ForecastMetrics{}, nil
	}
	return &ForecastMetrics{
		Impressions:      m.Impressions,
		Clicks:           m.Clicks,
		CTR:              m.ClickThroughRate,
		AverageCpcMicros: int64(m.AverageCpcMicros),
		CostMicros:       int64(m.CostMicros),
		Conversions:      m.Conversions,
	}, nil
}

// CurrencyCode returns the account currency (e.g. "EUR"), which every bid and CPC is in.
// Successful lookups are cached for the client's lifetime; failures return "" and are retried
// on the next call, so a missing permission never fails a tool.
func (c *Client) CurrencyCode(ctx context.Context) string {
	c.currencyMu.Lock()
	defer c.currencyMu.Unlock()
	if c.currencyCode != "" {
		return c.currencyCode
	}
	var raw searchResponse
	endpoint := fmt.Sprintf("%s/customers/%s/googleAds:search", c.baseURL, c.customerID)
	if err := c.post(ctx, endpoint, searchRequest{Query: "SELECT customer.currency_code FROM customer LIMIT 1"}, &raw, false); err != nil {
		return ""
	}
	if len(raw.Results) > 0 {
		c.currencyCode = raw.Results[0].Customer.CurrencyCode
	}
	return c.currencyCode
}

func (c *Client) customerEndpoint(method string) string {
	return fmt.Sprintf("%s/customers/%s:%s", c.baseURL, c.customerID, method)
}

// historyOptions requests the last 24 complete months plus average CPC. If the API has less
// history available it returns the subset it has.
func (c *Client) historyOptions() *historicalMetricsOptions {
	now := c.now().UTC()
	end := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
	start := end.AddDate(0, -(historyMonths - 1), 0)
	return &historicalMetricsOptions{
		YearMonthRange: &yearMonthRange{
			Start: yearMonth{Year: start.Year(), Month: monthNames[start.Month()-1]},
			End:   yearMonth{Year: end.Year(), Month: monthNames[end.Month()-1]},
		},
		IncludeAverageCpc: true,
	}
}

// APIError is a non-200 response from the Google Ads API.
type APIError struct {
	HTTPStatus int
	Status     string
	Message    string
	Details    []string
	RequestID  string
	Body       string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("Google Ads API returned HTTP %d: %s", e.HTTPStatus, e.Body)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Google Ads API returned HTTP %d %s: %s", e.HTTPStatus, e.Status, e.Message)
	if len(e.Details) > 0 {
		fmt.Fprintf(&b, " [%s]", strings.Join(e.Details, "; "))
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, " (request id %s)", e.RequestID)
	}
	return b.String()
}

// Retryable reports whether the error is a quota or transient failure worth one retry.
func (e *APIError) Retryable() bool {
	if e.HTTPStatus == http.StatusTooManyRequests || e.HTTPStatus == http.StatusServiceUnavailable {
		return true
	}
	if e.Status == "RESOURCE_EXHAUSTED" || e.Status == "UNAVAILABLE" {
		return true
	}
	for _, d := range e.Details {
		if strings.Contains(d, "RESOURCE_EXHAUSTED") || strings.Contains(d, "RESOURCE_TEMPORARILY_EXHAUSTED") {
			return true
		}
	}
	return false
}

func parseAPIError(status int, body []byte) *APIError {
	apiErr := &APIError{HTTPStatus: status, Body: string(body)}
	var parsed apiErrorBody
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.Error.Message == "" {
		return apiErr
	}
	apiErr.Status = parsed.Error.Status
	apiErr.Message = parsed.Error.Message
	for _, d := range parsed.Error.Details {
		if d.RequestID != "" {
			apiErr.RequestID = d.RequestID
		}
		for _, e := range d.Errors {
			codes := make([]string, 0, len(e.ErrorCode))
			for k, v := range e.ErrorCode {
				codes = append(codes, k+"="+v)
			}
			sort.Strings(codes)
			detail := strings.Join(codes, ",")
			if e.Message != "" {
				detail += ": " + e.Message
			}
			apiErr.Details = append(apiErr.Details, detail)
		}
	}
	return apiErr
}

func (c *Client) post(ctx context.Context, endpoint string, body, out any, cacheable bool) error {
	reqBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshalling request: %w", err)
	}
	cacheKey := endpoint + "\n" + string(reqBytes)
	if cacheable && c.cache != nil {
		if cached, ok := c.cache.get(cacheKey); ok {
			return decodeResponse(cached, out)
		}
	}

	respBody, err := c.send(ctx, endpoint, reqBytes)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.Retryable() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.retryDelay):
		}
		respBody, err = c.send(ctx, endpoint, reqBytes)
	}
	if err != nil {
		return err
	}
	if err := decodeResponse(respBody, out); err != nil {
		return err
	}
	if cacheable && c.cache != nil {
		c.cache.set(cacheKey, respBody)
	}
	return nil
}

func decodeResponse(body []byte, out any) error {
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}
	return nil
}

func (c *Client) send(ctx context.Context, endpoint string, reqBytes []byte) ([]byte, error) {
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("developer-token", c.developerToken)
	if c.loginCustomerID != "" {
		req.Header.Set("login-customer-id", c.loginCustomerID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseAPIError(resp.StatusCode, respBody)
	}
	return respBody, nil
}

// responseCache is a TTL cache of raw API response bodies, bounded by total size.
type responseCache struct {
	mu       sync.Mutex
	ttl      time.Duration
	maxBytes int
	size     int
	entries  map[string]cacheEntry
}

type cacheEntry struct {
	body    []byte
	expires time.Time
}

func newResponseCache(ttl time.Duration, maxBytes int) *responseCache {
	return &responseCache{ttl: ttl, maxBytes: maxBytes, entries: make(map[string]cacheEntry)}
}

func (rc *responseCache) get(key string) ([]byte, bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	e, ok := rc.entries[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(e.expires) {
		rc.remove(key)
		return nil, false
	}
	return e.body, true
}

func (rc *responseCache) set(key string, body []byte) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	// One response may use at most a quarter of the budget, so the cache keeps variety.
	if len(body) > rc.maxBytes/4 {
		return
	}
	rc.remove(key)
	now := time.Now()
	for k, e := range rc.entries {
		if now.After(e.expires) {
			rc.remove(k)
		}
	}
	// Still over budget: evict the entries closest to expiry (the oldest).
	for rc.size+len(body) > rc.maxBytes && len(rc.entries) > 0 {
		var oldestKey string
		var oldest time.Time
		for k, e := range rc.entries {
			if oldestKey == "" || e.expires.Before(oldest) {
				oldestKey, oldest = k, e.expires
			}
		}
		rc.remove(oldestKey)
	}
	rc.entries[key] = cacheEntry{body: body, expires: now.Add(rc.ttl)}
	rc.size += len(body)
}

func (rc *responseCache) remove(key string) {
	if e, ok := rc.entries[key]; ok {
		rc.size -= len(e.body)
		delete(rc.entries, key)
	}
}
