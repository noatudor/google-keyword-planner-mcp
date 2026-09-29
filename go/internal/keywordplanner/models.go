// Package keywordplanner provides a client for the Google Ads Keyword Planner API
// and the AdSense-for-Search scoring built on top of it.
package keywordplanner

import (
	"strconv"
	"strings"
)

// Targeting selects the market a planner request is answered for.
type Targeting struct {
	// Language is a language resource name, e.g. "languageConstants/1001". Empty means all languages.
	Language string
	// GeoTargets are geo target resource names, e.g. "geoTargetConstants/2276". Empty means worldwide.
	GeoTargets []string
	// Network is GOOGLE_SEARCH or GOOGLE_SEARCH_AND_PARTNERS. Empty uses the API default.
	Network string
}

// Concept is a Google keyword concept annotation, e.g. {Name: "stannah", GroupType: "OTHER_BRANDS"}.
type Concept struct {
	Name      string `json:"name"`
	Group     string `json:"group,omitempty"`
	GroupType string `json:"groupType,omitempty"`
}

// MonthlyVolume is the search volume for a specific month.
type MonthlyVolume struct {
	Year            int32 `json:"year"`
	Month           int32 `json:"month"`
	MonthlySearches int64 `json:"monthlySearches"`
}

// KeywordData is one keyword's raw planner metrics, independent of which API call produced it.
type KeywordData struct {
	Text                   string
	AvgMonthlySearches     int64
	Competition            string
	CompetitionIndex       int64
	LowTopOfPageBidMicros  int64
	HighTopOfPageBidMicros int64
	AverageCpcMicros       int64
	MonthlySearchVolumes   []MonthlyVolume
	CloseVariants          []string
	Concepts               []Concept
}

// IdeasRequest describes a GenerateKeywordIdeas call. At least one of Seeds, URL or Site is required.
type IdeasRequest struct {
	Seeds []string
	URL   string
	Site  string
	Targeting
}

// IdeasResult is the parsed GenerateKeywordIdeas response.
type IdeasResult struct {
	Ideas          []KeywordData
	TotalSize      int64
	DeviceSearches map[string]int64
}

// HistoricalResult is the parsed GenerateKeywordHistoricalMetrics response.
type HistoricalResult struct {
	Keywords       []KeywordData
	DeviceSearches map[string]int64
}

// ForecastRequest describes a GenerateKeywordForecastMetrics call for one simulated campaign.
type ForecastRequest struct {
	Keywords          []string
	MatchType         string
	MaxCPCBidMicros   int64
	DailyBudgetMicros int64
	Days              int
	Targeting
}

// ForecastMetrics are the campaign-level forecast totals returned by the API.
type ForecastMetrics struct {
	Impressions      float64 `json:"impressions"`
	Clicks           float64 `json:"clicks"`
	CTR              float64 `json:"ctr"`
	AverageCpcMicros int64   `json:"averageCpcMicros"`
	CostMicros       int64   `json:"costMicros"`
	Conversions      float64 `json:"conversions,omitempty"`
}

// int64String decodes a proto3-JSON int64, which the API sends as a string ("123")
// but which some fixtures and proxies send as a bare number.
type int64String int64

// UnmarshalJSON accepts "123", 123, 123.0, "" and null.
func (v *int64String) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*v = 0
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		f, ferr := strconv.ParseFloat(s, 64)
		if ferr != nil {
			return err
		}
		n = int64(f)
	}
	*v = int64String(n)
	return nil
}

// --- Google Ads API raw request types ---

type generateKeywordIdeasRequest struct {
	Language                 string                    `json:"language,omitempty"`
	GeoTargetConstants       []string                  `json:"geoTargetConstants,omitempty"`
	IncludeAdultKeywords     bool                      `json:"includeAdultKeywords"`
	KeywordPlanNetwork       string                    `json:"keywordPlanNetwork,omitempty"`
	KeywordAnnotation        []string                  `json:"keywordAnnotation,omitempty"`
	AggregateMetrics         *aggregateMetrics         `json:"aggregateMetrics,omitempty"`
	HistoricalMetricsOptions *historicalMetricsOptions `json:"historicalMetricsOptions,omitempty"`
	KeywordSeed              *keywordSeed              `json:"keywordSeed,omitempty"`
	URLSeed                  *urlSeed                  `json:"urlSeed,omitempty"`
	KeywordAndURLSeed        *keywordAndURLSeed        `json:"keywordAndUrlSeed,omitempty"`
	SiteSeed                 *siteSeed                 `json:"siteSeed,omitempty"`
}

type keywordSeed struct {
	Keywords []string `json:"keywords"`
}

type urlSeed struct {
	URL string `json:"url"`
}

type siteSeed struct {
	Site string `json:"site"`
}

type keywordAndURLSeed struct {
	URL      string   `json:"url"`
	Keywords []string `json:"keywords"`
}

type aggregateMetrics struct {
	AggregateMetricTypes []string `json:"aggregateMetricTypes"`
}

type historicalMetricsOptions struct {
	YearMonthRange    *yearMonthRange `json:"yearMonthRange,omitempty"`
	IncludeAverageCpc bool            `json:"includeAverageCpc"`
}

type yearMonthRange struct {
	Start yearMonth `json:"start"`
	End   yearMonth `json:"end"`
}

type yearMonth struct {
	Year  int    `json:"year"`
	Month string `json:"month"`
}

type generateHistoricalMetricsRequest struct {
	Keywords                 []string                  `json:"keywords"`
	Language                 string                    `json:"language,omitempty"`
	GeoTargetConstants       []string                  `json:"geoTargetConstants,omitempty"`
	IncludeAdultKeywords     bool                      `json:"includeAdultKeywords"`
	KeywordPlanNetwork       string                    `json:"keywordPlanNetwork,omitempty"`
	AggregateMetrics         *aggregateMetrics         `json:"aggregateMetrics,omitempty"`
	HistoricalMetricsOptions *historicalMetricsOptions `json:"historicalMetricsOptions,omitempty"`
}

type generateForecastMetricsRequest struct {
	ForecastPeriod dateRange          `json:"forecastPeriod"`
	Campaign       campaignToForecast `json:"campaign"`
}

type dateRange struct {
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

type campaignToForecast struct {
	LanguageConstants  []string                `json:"languageConstants,omitempty"`
	GeoModifiers       []criterionBidModifier  `json:"geoModifiers,omitempty"`
	KeywordPlanNetwork string                  `json:"keywordPlanNetwork"`
	BiddingStrategy    campaignBiddingStrategy `json:"biddingStrategy"`
	AdGroups           []forecastAdGroup       `json:"adGroups"`
}

type criterionBidModifier struct {
	GeoTargetConstant string `json:"geoTargetConstant"`
}

type campaignBiddingStrategy struct {
	ManualCpcBiddingStrategy manualCpcBiddingStrategy `json:"manualCpcBiddingStrategy"`
}

type manualCpcBiddingStrategy struct {
	DailyBudgetMicros string `json:"dailyBudgetMicros,omitempty"`
	MaxCpcBidMicros   string `json:"maxCpcBidMicros"`
}

type forecastAdGroup struct {
	BiddableKeywords []biddableKeyword `json:"biddableKeywords"`
}

type biddableKeyword struct {
	Keyword keywordInfo `json:"keyword"`
}

type keywordInfo struct {
	Text      string `json:"text"`
	MatchType string `json:"matchType"`
}

type searchRequest struct {
	Query string `json:"query"`
}

// --- Google Ads API raw response types ---

type keywordPlanHistoricalMetrics struct {
	AvgMonthlySearches     int64String           `json:"avgMonthlySearches"`
	MonthlySearchVolumes   []monthlySearchVolume `json:"monthlySearchVolumes"`
	Competition            string                `json:"competition"`
	CompetitionIndex       int64String           `json:"competitionIndex"`
	LowTopOfPageBidMicros  int64String           `json:"lowTopOfPageBidMicros"`
	HighTopOfPageBidMicros int64String           `json:"highTopOfPageBidMicros"`
	AverageCpcMicros       int64String           `json:"averageCpcMicros"`
}

type monthlySearchVolume struct {
	Year            int64String `json:"year"`
	Month           string      `json:"month"`
	MonthlySearches int64String `json:"monthlySearches"`
}

type keywordAnnotations struct {
	Concepts []keywordConcept `json:"concepts"`
}

type keywordConcept struct {
	Name         string `json:"name"`
	ConceptGroup struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"conceptGroup"`
}

type aggregateMetricResults struct {
	DeviceSearches []struct {
		Device      string      `json:"device"`
		SearchCount int64String `json:"searchCount"`
	} `json:"deviceSearches"`
}

type generateKeywordIdeasResponse struct {
	Results                []keywordIdeaResult     `json:"results"`
	AggregateMetricResults *aggregateMetricResults `json:"aggregateMetricResults"`
	TotalSize              int64String             `json:"totalSize"`
}

type keywordIdeaResult struct {
	Text               string                       `json:"text"`
	KeywordIdeaMetrics keywordPlanHistoricalMetrics `json:"keywordIdeaMetrics"`
	KeywordAnnotations *keywordAnnotations          `json:"keywordAnnotations"`
	CloseVariants      []string                     `json:"closeVariants"`
}

type generateHistoricalMetricsResponse struct {
	Results                []historicalMetricsResult `json:"results"`
	AggregateMetricResults *aggregateMetricResults   `json:"aggregateMetricResults"`
}

type historicalMetricsResult struct {
	Text           string                       `json:"text"`
	CloseVariants  []string                     `json:"closeVariants"`
	KeywordMetrics keywordPlanHistoricalMetrics `json:"keywordMetrics"`
}

type generateForecastMetricsResponse struct {
	CampaignForecastMetrics *keywordForecastMetrics `json:"campaignForecastMetrics"`
}

type keywordForecastMetrics struct {
	Impressions      float64     `json:"impressions"`
	ClickThroughRate float64     `json:"clickThroughRate"`
	AverageCpcMicros int64String `json:"averageCpcMicros"`
	Clicks           float64     `json:"clicks"`
	CostMicros       int64String `json:"costMicros"`
	Conversions      float64     `json:"conversions"`
}

type searchResponse struct {
	Results []struct {
		Customer struct {
			CurrencyCode string `json:"currencyCode"`
		} `json:"customer"`
	} `json:"results"`
}

type apiErrorBody struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
		Details []struct {
			Errors []struct {
				ErrorCode map[string]string `json:"errorCode"`
				Message   string            `json:"message"`
			} `json:"errors"`
			RequestID string `json:"requestId"`
		} `json:"details"`
	} `json:"error"`
}

func (m keywordPlanHistoricalMetrics) toData(text string, closeVariants []string, ann *keywordAnnotations) KeywordData {
	monthly := make([]MonthlyVolume, 0, len(m.MonthlySearchVolumes))
	for _, v := range m.MonthlySearchVolumes {
		monthly = append(monthly, MonthlyVolume{
			Year:            int32(v.Year),
			Month:           parseMonthEnum(v.Month),
			MonthlySearches: int64(v.MonthlySearches),
		})
	}
	var concepts []Concept
	if ann != nil {
		for _, c := range ann.Concepts {
			concepts = append(concepts, Concept{Name: c.Name, Group: c.ConceptGroup.Name, GroupType: c.ConceptGroup.Type})
		}
	}
	return KeywordData{
		Text:                   text,
		AvgMonthlySearches:     int64(m.AvgMonthlySearches),
		Competition:            m.Competition,
		CompetitionIndex:       int64(m.CompetitionIndex),
		LowTopOfPageBidMicros:  int64(m.LowTopOfPageBidMicros),
		HighTopOfPageBidMicros: int64(m.HighTopOfPageBidMicros),
		AverageCpcMicros:       int64(m.AverageCpcMicros),
		MonthlySearchVolumes:   monthly,
		CloseVariants:          closeVariants,
		Concepts:               concepts,
	}
}

func (a *aggregateMetricResults) deviceMap() map[string]int64 {
	if a == nil || len(a.DeviceSearches) == 0 {
		return nil
	}
	out := make(map[string]int64, len(a.DeviceSearches))
	for _, d := range a.DeviceSearches {
		out[d.Device] = int64(d.SearchCount)
	}
	return out
}

var monthNames = [...]string{"JANUARY", "FEBRUARY", "MARCH", "APRIL", "MAY", "JUNE",
	"JULY", "AUGUST", "SEPTEMBER", "OCTOBER", "NOVEMBER", "DECEMBER"}

// parseMonthEnum converts "JANUARY" → 1, etc.
func parseMonthEnum(month string) int32 {
	for i, name := range monthNames {
		if strings.EqualFold(name, month) {
			return int32(i + 1)
		}
	}
	return 0
}
