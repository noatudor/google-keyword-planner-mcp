package keywordplanner

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Demand tiers and verdicts.
const (
	TierStrong = "strong"
	TierUsable = "usable"
	TierWeak   = "weak"

	VerdictLaunch     = "launch"
	VerdictBorderline = "borderline"
	VerdictSkip       = "skip"
)

// Thresholds define the demand tiers. Defaults follow the media-buyer spec:
// strong = high bid >= 2.00, competition HIGH or MEDIUM, 500+ searches;
// usable = high bid 1.00-2.00, or strong-level bids at 100-500 searches.
type Thresholds struct {
	StrongMinHighBid  float64 `json:"strongMinHighBid"`
	StrongMinSearches int64   `json:"strongMinSearches"`
	UsableMinHighBid  float64 `json:"usableMinHighBid"`
	UsableMinSearches int64   `json:"usableMinSearches"`
}

// DefaultThresholds returns the spec's tier thresholds.
func DefaultThresholds() Thresholds {
	return Thresholds{StrongMinHighBid: 2.00, StrongMinSearches: 500, UsableMinHighBid: 1.00, UsableMinSearches: 100}
}

// TierRank orders tiers: strong 3, usable 2, weak 1, unknown 0.
func TierRank(tier string) int {
	switch tier {
	case TierStrong:
		return 3
	case TierUsable:
		return 2
	case TierWeak:
		return 1
	}
	return 0
}

// BrandInfo says whether a keyword refers to a brand or trademark.
type BrandInfo struct {
	IsBrand bool     `json:"isBrand"`
	Brands  []string `json:"brands,omitempty"`
	// Checked is false when no brand annotations were available for this keyword.
	Checked bool `json:"checked"`
}

// Trend summarises the monthly search history.
type Trend struct {
	Direction        string   `json:"direction"`
	YoYPct           *float64 `json:"yoyPct,omitempty"`
	Momentum3mPct    *float64 `json:"momentum3mPct,omitempty"`
	PeakMonths       []int    `json:"peakMonths,omitempty"`
	SeasonalityIndex float64  `json:"seasonalityIndex"`
	NextPeakMonth    int      `json:"nextPeakMonth,omitempty"`
	DaysToNextPeak   *int     `json:"daysToNextPeak,omitempty"`
	MonthsOfHistory  int      `json:"monthsOfHistory"`

	monthAvg [13]float64
	mean     float64
}

// KeywordRow is one keyword scored for AdSense-for-Search arbitrage. Every tool returns this shape.
type KeywordRow struct {
	Text                   string                     `json:"text"`
	MatchedAs              string                     `json:"matchedAs,omitempty"`
	IsSeed                 bool                       `json:"isSeed,omitempty"`
	Source                 string                     `json:"source,omitempty"`
	Modifier               string                     `json:"modifier,omitempty"`
	AvgMonthlySearches     int64                      `json:"avgMonthlySearches"`
	Competition            string                     `json:"competition,omitempty"`
	CompetitionIndex       int64                      `json:"competitionIndex"`
	LowTopOfPageBid        float64                    `json:"lowTopOfPageBid"`
	HighTopOfPageBid       float64                    `json:"highTopOfPageBid"`
	AverageCpc             float64                    `json:"averageCpc"`
	LowTopOfPageBidMicros  int64                      `json:"lowTopOfPageBidMicros,omitempty"`
	HighTopOfPageBidMicros int64                      `json:"highTopOfPageBidMicros,omitempty"`
	AdvertiserValue        float64                    `json:"advertiserValue"`
	DemandTier             string                     `json:"demandTier"`
	Intent                 string                     `json:"commercialIntent"`
	IntentModifiers        []string                   `json:"intentModifiers,omitempty"`
	SensitiveCategoryHint  string                     `json:"sensitiveCategoryHint,omitempty"`
	Brand                  BrandInfo                  `json:"brand"`
	Trend                  *Trend                     `json:"trend,omitempty"`
	AfsScore               int                        `json:"afsScore"`
	Verdict                string                     `json:"verdict"`
	Reasons                []string                   `json:"reasons"`
	EstimatedRpc           *float64                   `json:"estimatedRpc,omitempty"`
	BreakEvenFbCpc         *float64                   `json:"breakEvenFbCpc,omitempty"`
	SeasonalLift           *float64                   `json:"seasonalLift,omitempty"`
	CloseVariants          []string                   `json:"closeVariants,omitempty"`
	MonthlySearchVolumes   []MonthlyVolume            `json:"monthlySearchVolumes,omitempty"`
	ByCountry              map[string]*CountryMetrics `json:"byCountry,omitempty"`
	BestCountry            string                     `json:"bestCountry,omitempty"`
}

// CountryMetrics is the per-market slice of a keyword row.
type CountryMetrics struct {
	Language           string   `json:"language,omitempty"`
	AvgMonthlySearches int64    `json:"avgMonthlySearches"`
	Competition        string   `json:"competition,omitempty"`
	HighTopOfPageBid   float64  `json:"highTopOfPageBid"`
	LowTopOfPageBid    float64  `json:"lowTopOfPageBid"`
	AverageCpc         float64  `json:"averageCpc"`
	AdvertiserValue    float64  `json:"advertiserValue"`
	DemandTier         string   `json:"demandTier"`
	AfsScore           int      `json:"afsScore"`
	Verdict            string   `json:"verdict"`
	TrendDirection     string   `json:"trendDirection,omitempty"`
	EstimatedRpc       *float64 `json:"estimatedRpc,omitempty"`
	Found              bool     `json:"found"`
}

// Scorer turns raw planner data into AFS keyword rows.
type Scorer struct {
	Thresholds Thresholds
	// Language is the ISO code used to pick intent lexicons ("" checks all).
	Language string
	Currency string
	// RPCRatio, when > 0, estimates RPC as RPCRatio × high top-of-page bid. It must come from
	// our own calibration (tracker RPC vs planner bid); the server never assumes one.
	RPCRatio float64
	// TargetROI is the ROI the break-even Facebook CPC must still leave (default 0.15).
	TargetROI float64
	Now       time.Time
	// Brands are normalized brand names known from concept annotations in the same result set.
	Brands map[string]bool
	// BrandChecked says whether brand annotations were available at all.
	BrandChecked bool
}

// BrandNames collects the brand concept names (BRAND and OTHER_BRANDS groups) from a result set.
func BrandNames(data []KeywordData) map[string]bool {
	out := map[string]bool{}
	for _, d := range data {
		for _, c := range d.Concepts {
			if isBrandGroup(c.GroupType) {
				if n := Normalize(c.Name); len(n) >= 3 {
					out[n] = true
				}
			}
		}
	}
	return out
}

func isBrandGroup(t string) bool { return t == "BRAND" || t == "OTHER_BRANDS" }

// Row scores one keyword. The afsScore is capped at 40 for weak and 75 for usable demand.
func (s Scorer) Row(d KeywordData) KeywordRow {
	th := s.Thresholds
	if th == (Thresholds{}) {
		th = DefaultThresholds()
	}
	now := s.Now
	if now.IsZero() {
		now = time.Now()
	}
	r := KeywordRow{
		Text:                   d.Text,
		AvgMonthlySearches:     d.AvgMonthlySearches,
		Competition:            d.Competition,
		CompetitionIndex:       d.CompetitionIndex,
		LowTopOfPageBid:        microsToUnits(d.LowTopOfPageBidMicros),
		HighTopOfPageBid:       microsToUnits(d.HighTopOfPageBidMicros),
		AverageCpc:             microsToUnits(d.AverageCpcMicros),
		LowTopOfPageBidMicros:  d.LowTopOfPageBidMicros,
		HighTopOfPageBidMicros: d.HighTopOfPageBidMicros,
		CloseVariants:          d.CloseVariants,
		MonthlySearchVolumes:   d.MonthlySearchVolumes,
	}
	r.AdvertiserValue = round2(float64(r.AvgMonthlySearches) * r.HighTopOfPageBid)
	r.DemandTier = DemandTier(r.AvgMonthlySearches, r.HighTopOfPageBid, r.Competition, th)
	r.Brand = s.brandInfo(d)
	intent := ClassifyIntent(d.Text, s.Language, r.Brand.IsBrand)
	r.Intent, r.IntentModifiers, r.SensitiveCategoryHint = intent.Intent, intent.Modifiers, intent.SensitiveCategory
	r.Trend = ComputeTrend(d.MonthlySearchVolumes, now)

	if s.RPCRatio > 0 && r.HighTopOfPageBid > 0 {
		roi := s.TargetROI
		if roi <= 0 {
			roi = 0.15
		}
		rpc := round2(s.RPCRatio * r.HighTopOfPageBid)
		cpc := round2(rpc / (1 + roi))
		r.EstimatedRpc, r.BreakEvenFbCpc = &rpc, &cpc
	}
	r.AfsScore, r.Verdict, r.Reasons = s.verdict(r)
	return r
}

// DemandTier applies the tier rules. Keywords with under 10 searches have no usable data and are weak.
func DemandTier(searches int64, highBid float64, competition string, th Thresholds) string {
	if searches < 10 {
		return TierWeak
	}
	compOK := competition == "HIGH" || competition == "MEDIUM"
	switch {
	case highBid >= th.StrongMinHighBid && compOK && searches >= th.StrongMinSearches:
		return TierStrong
	case highBid >= th.StrongMinHighBid && searches >= th.UsableMinSearches:
		return TierUsable
	case highBid >= th.UsableMinHighBid && highBid < th.StrongMinHighBid:
		return TierUsable
	}
	return TierWeak
}

func (s Scorer) brandInfo(d KeywordData) BrandInfo {
	info := BrandInfo{Checked: s.BrandChecked || len(d.Concepts) > 0}
	var brands []string
	for _, c := range d.Concepts {
		if isBrandGroup(c.GroupType) {
			brands = append(brands, strings.ToLower(c.Name))
		}
	}
	if len(s.Brands) > 0 {
		padded := " " + Normalize(d.Text) + " "
		for b := range s.Brands {
			if strings.Contains(padded, " "+b+" ") {
				brands = append(brands, b)
			}
		}
	}
	info.Brands = uniqueSorted(brands)
	info.IsBrand = len(info.Brands) > 0
	return info
}

// verdict computes the 0-100 AFS score, the verdict and the human-readable reasons.
func (s Scorer) verdict(r KeywordRow) (int, string, []string) {
	cur := s.Currency
	if cur != "" {
		cur = " " + cur
	}
	var reasons []string

	price := r.AverageCpc
	if price <= 0 {
		price = r.HighTopOfPageBid
	}
	bidPts := 40 * math.Min(1, math.Log1p(price)/math.Log1p(8))
	volPts := 0.0
	if r.AvgMonthlySearches > 0 {
		volPts = 20 * math.Min(1, math.Log10(float64(r.AvgMonthlySearches))/math.Log10(50000))
	}
	compPts := map[string]float64{"HIGH": 15, "MEDIUM": 10, "LOW": 4}[r.Competition]
	intentPts := map[string]float64{IntentTransactional: 15, IntentCommercial: 12, IntentUnclassified: 6, IntentInformational: 2}[r.Intent]
	trendPts := 5.0
	if r.Trend != nil {
		trendPts = map[string]float64{"rising": 10, "flat": 6, "falling": 2}[r.Trend.Direction]
	}
	score := bidPts + volPts + compPts + intentPts + trendPts

	bidText := fmt.Sprintf("top-of-page bid %.2f–%.2f%s", r.LowTopOfPageBid, r.HighTopOfPageBid, cur)
	if r.HighTopOfPageBid == 0 {
		bidText = "no bid data"
	}
	demand := fmt.Sprintf("%s, %s competition, %s searches/mo → %s",
		bidText, orDash(r.Competition), formatInt(r.AvgMonthlySearches), r.DemandTier)
	reasons = append(reasons, demand)
	if r.AverageCpc > 0 {
		reasons = append(reasons, fmt.Sprintf("advertisers paid avg CPC %.2f%s", r.AverageCpc, cur))
	}
	if len(r.IntentModifiers) > 0 {
		reasons = append(reasons, fmt.Sprintf("%s intent (%s)", r.Intent, strings.Join(r.IntentModifiers, ", ")))
	} else {
		reasons = append(reasons, r.Intent+" intent")
	}
	if t := r.Trend; t != nil {
		switch {
		case t.YoYPct != nil:
			reasons = append(reasons, fmt.Sprintf("trend %s (YoY %+.0f%%)", t.Direction, *t.YoYPct))
		case t.Momentum3mPct != nil:
			reasons = append(reasons, fmt.Sprintf("trend %s (3m %+.0f%%)", t.Direction, *t.Momentum3mPct))
		}
		if t.DaysToNextPeak != nil && t.SeasonalityIndex >= 1.3 {
			reasons = append(reasons, fmt.Sprintf("seasonal: peak in %s (in %d days)", time.Month(t.NextPeakMonth), *t.DaysToNextPeak))
		}
	}
	if r.SensitiveCategoryHint != "" {
		reasons = append(reasons, fmt.Sprintf("%s-type wording: Adspy may assign a special ad category (never set it yourself)", r.SensitiveCategoryHint))
	}
	if r.EstimatedRpc != nil {
		reasons = append(reasons, fmt.Sprintf("est. RPC %.2f%s → break-even FB CPC %.2f%s", *r.EstimatedRpc, cur, *r.BreakEvenFbCpc, cur))
	}

	var verdict string
	switch {
	case r.Brand.IsBrand:
		verdict = VerdictSkip
		score = math.Min(score, 10)
		reasons = append([]string{"brand keyword (" + strings.Join(r.Brand.Brands, ", ") + "): no-brand rule"}, reasons...)
	case r.Intent == IntentNavigational:
		verdict = VerdictSkip
	case r.DemandTier == TierStrong && r.Intent == IntentInformational:
		verdict = VerdictBorderline
		reasons = append(reasons, "strong bids but informational wording: prefer a commercial variant")
	case r.DemandTier == TierStrong:
		verdict = VerdictLaunch
		if r.Trend != nil && r.Trend.YoYPct != nil && *r.Trend.YoYPct <= -30 {
			verdict = VerdictBorderline
			reasons = append(reasons, "demand falling sharply year over year")
		}
	case r.DemandTier == TierUsable:
		verdict = VerdictBorderline
	default:
		verdict = VerdictSkip
		reasons = append(reasons, "weak advertiser demand: curiosity traffic, low RPC expected")
	}
	// Cap by tier so the score never ranks thin or unbid keywords above real demand.
	switch r.DemandTier {
	case TierWeak:
		score = math.Min(score, 40)
	case TierUsable:
		score = math.Min(score, 75)
	}
	return int(math.Round(math.Max(0, math.Min(100, score)))), verdict, reasons
}

// VerdictRank orders verdicts: launch 3, borderline 2, skip 1.
func VerdictRank(v string) int {
	switch v {
	case VerdictLaunch:
		return 3
	case VerdictBorderline:
		return 2
	case VerdictSkip:
		return 1
	}
	return 0
}

// ComputeTrend derives direction, YoY, momentum and seasonality from monthly volumes.
// It returns nil when there are fewer than 6 months of history.
func ComputeTrend(vols []MonthlyVolume, now time.Time) *Trend {
	series := make([]MonthlyVolume, 0, len(vols))
	for _, v := range vols {
		if v.Year > 0 && v.Month >= 1 && v.Month <= 12 {
			series = append(series, v)
		}
	}
	if len(series) < 6 {
		return nil
	}
	sort.Slice(series, func(i, j int) bool {
		return series[i].Year*12+series[i].Month < series[j].Year*12+series[j].Month
	})
	n := len(series)
	sum := func(from, to int) float64 {
		total := 0.0
		for _, v := range series[from:to] {
			total += float64(v.MonthlySearches)
		}
		return total
	}
	t := &Trend{Direction: "flat", MonthsOfHistory: n}
	if prev := sum(n-6, n-3); prev > 0 {
		m := round1((sum(n-3, n)/prev - 1) * 100)
		t.Momentum3mPct = &m
	}
	if n >= 15 {
		if prev := sum(n-15, n-12); prev > 0 {
			y := round1((sum(n-3, n)/prev - 1) * 100)
			t.YoYPct = &y
		}
	}
	signal := t.YoYPct
	if signal == nil {
		signal = t.Momentum3mPct
	}
	if signal != nil {
		switch {
		case *signal >= 15:
			t.Direction = "rising"
		case *signal <= -15:
			t.Direction = "falling"
		}
	}

	var counts [13]float64
	for _, v := range series {
		t.monthAvg[v.Month] += float64(v.MonthlySearches)
		counts[v.Month]++
	}
	total, months := 0.0, 0.0
	for m := 1; m <= 12; m++ {
		if counts[m] > 0 {
			t.monthAvg[m] /= counts[m]
			total += t.monthAvg[m]
			months++
		}
	}
	if months == 0 || total == 0 {
		return t
	}
	t.mean = total / months
	maxAvg := 0.0
	type mv struct {
		m int
		v float64
	}
	var peaks []mv
	for m := 1; m <= 12; m++ {
		maxAvg = math.Max(maxAvg, t.monthAvg[m])
		if counts[m] > 0 && t.monthAvg[m] >= 1.25*t.mean {
			peaks = append(peaks, mv{m, t.monthAvg[m]})
		}
	}
	t.SeasonalityIndex = round2(maxAvg / t.mean)
	sort.Slice(peaks, func(i, j int) bool { return peaks[i].v > peaks[j].v })
	for i, p := range peaks {
		if i == 3 {
			break
		}
		t.PeakMonths = append(t.PeakMonths, p.m)
	}
	if len(t.PeakMonths) > 0 {
		best := -1
		for _, pm := range t.PeakMonths {
			d := daysUntilMonth(now, pm)
			if best < 0 || d < best {
				best, t.NextPeakMonth = d, pm
			}
		}
		t.DaysToNextPeak = &best
	}
	return t
}

// SeasonalLift is the average volume of the calendar months overlapping [from, to]
// relative to the yearly mean (1.0 = no lift). It returns 0 without history.
func (t *Trend) SeasonalLift(from, to time.Time) float64 {
	if t == nil || t.mean == 0 {
		return 0
	}
	seen := map[int]bool{}
	total, n := 0.0, 0.0
	for d := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC); !d.After(to); d = d.AddDate(0, 1, 0) {
		m := int(d.Month())
		if seen[m] {
			continue
		}
		seen[m] = true
		total += t.monthAvg[m]
		n++
	}
	if n == 0 {
		return 0
	}
	return round2(total / n / t.mean)
}

// daysUntilMonth returns days from now to the 1st of the next occurrence of month (0 if now is in it).
func daysUntilMonth(now time.Time, month int) int {
	if int(now.Month()) == month {
		return 0
	}
	y := now.Year()
	if month < int(now.Month()) {
		y++
	}
	target := time.Date(y, time.Month(month), 1, 0, 0, 0, 0, now.Location())
	return int(math.Ceil(target.Sub(now).Hours() / 24))
}

// CountrySlice builds the per-market view of a scored row.
func CountrySlice(r KeywordRow, language string) *CountryMetrics {
	c := &CountryMetrics{
		Language:           language,
		AvgMonthlySearches: r.AvgMonthlySearches,
		Competition:        r.Competition,
		HighTopOfPageBid:   r.HighTopOfPageBid,
		LowTopOfPageBid:    r.LowTopOfPageBid,
		AverageCpc:         r.AverageCpc,
		AdvertiserValue:    r.AdvertiserValue,
		DemandTier:         r.DemandTier,
		AfsScore:           r.AfsScore,
		Verdict:            r.Verdict,
		EstimatedRpc:       r.EstimatedRpc,
		Found:              true,
	}
	if r.Trend != nil {
		c.TrendDirection = r.Trend.Direction
	}
	return c
}

// BestCountry picks the market with the highest AFS score, then advertiser value.
func BestCountry(by map[string]*CountryMetrics) string {
	best := ""
	for code, m := range by {
		if !m.Found {
			continue
		}
		if best == "" {
			best = code
			continue
		}
		b := by[best]
		if m.AfsScore > b.AfsScore || (m.AfsScore == b.AfsScore && m.AdvertiserValue > b.AdvertiserValue) ||
			(m.AfsScore == b.AfsScore && m.AdvertiserValue == b.AdvertiserValue && code < best) {
			best = code
		}
	}
	return best
}

// BidderEvidence is a one-line "who is bidding" summary for a topic profile.
func BidderEvidence(r KeywordRow, market, currency string) string {
	cur := ""
	if currency != "" {
		cur = " " + currency
	}
	parts := []string{r.Text}
	if market != "" {
		parts = append(parts, market)
	}
	parts = append(parts,
		formatInt(r.AvgMonthlySearches)+"/mo",
		orDash(r.Competition),
		fmt.Sprintf("bid %.2f–%.2f%s", r.LowTopOfPageBid, r.HighTopOfPageBid, cur))
	if r.AverageCpc > 0 {
		parts = append(parts, fmt.Sprintf("avg CPC %.2f%s", r.AverageCpc, cur))
	}
	parts = append(parts, r.DemandTier, r.Intent)
	if r.Trend != nil {
		parts = append(parts, "trend "+r.Trend.Direction)
	}
	return strings.Join(parts, " · ")
}

func microsToUnits(m int64) float64 { return round2(float64(m) / 1_000_000) }

func round1(f float64) float64 { return math.Round(f*10) / 10 }

func round2(f float64) float64 { return math.Round(f*100) / 100 }

func orDash(s string) string {
	if s == "" {
		return "UNKNOWN"
	}
	return s
}

func formatInt(n int64) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		return s
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}
