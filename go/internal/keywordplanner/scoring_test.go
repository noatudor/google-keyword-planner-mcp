package keywordplanner_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ncosentino/google-keyword-planner-mcp/go/internal/keywordplanner"
)

func TestDemandTier_Boundaries(t *testing.T) {
	t.Parallel()
	th := keywordplanner.DefaultThresholds()
	cases := []struct {
		name     string
		searches int64
		highBid  float64
		comp     string
		want     string
	}{
		{"strong at exact thresholds", 500, 2.00, "MEDIUM", keywordplanner.TierStrong},
		{"strong high competition", 1600, 12.81, "HIGH", keywordplanner.TierStrong},
		{"bid just under strong is usable", 5000, 1.99, "HIGH", keywordplanner.TierUsable},
		{"strong bids at 100-500 searches", 100, 5.00, "HIGH", keywordplanner.TierUsable},
		{"strong bids low competition", 5000, 3.00, "LOW", keywordplanner.TierUsable},
		{"usable at 1.00", 50, 1.00, "LOW", keywordplanner.TierUsable},
		{"strong bids at 99 searches", 99, 5.00, "HIGH", keywordplanner.TierWeak},
		{"under 1.00", 90000, 0.99, "HIGH", keywordplanner.TierWeak},
		{"no data", 0, 0, "", keywordplanner.TierWeak},
		{"bids but under 10 searches", 5, 3.00, "HIGH", keywordplanner.TierWeak},
	}
	for _, tc := range cases {
		if got := keywordplanner.DemandTier(tc.searches, tc.highBid, tc.comp, th); got != tc.want {
			t.Errorf("%s: DemandTier(%d, %.2f, %s) = %s, want %s", tc.name, tc.searches, tc.highBid, tc.comp, got, tc.want)
		}
	}
}

func TestClassifyIntent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		text, lang, want string
	}{
		{"stairlift rental", "en", keywordplanner.IntentTransactional},
		{"rent to own medical equipment", "en", keywordplanner.IntentTransactional},
		{"best hearing aids for seniors", "en", keywordplanner.IntentCommercial},
		{"what is a heat pump", "en", keywordplanner.IntentInformational},
		{"wärmepumpe finanzieren", "de", keywordplanner.IntentTransactional},
		{"treppenliftkosten", "de", keywordplanner.IntentTransactional},
		{"treppenlift kostenlos", "de", keywordplanner.IntentInformational},
		{"wärmepumpe förderung", "de", keywordplanner.IntentCommercial},
		{"location monte escalier", "fr", keywordplanner.IntentTransactional},
		{"salvaescaleras precio", "es", keywordplanner.IntentTransactional},
		{"heat pump", "en", keywordplanner.IntentUnclassified},
	}
	for _, tc := range cases {
		if got := keywordplanner.ClassifyIntent(tc.text, tc.lang, false).Intent; got != tc.want {
			t.Errorf("ClassifyIntent(%q, %s) = %s, want %s", tc.text, tc.lang, got, tc.want)
		}
	}
	if got := keywordplanner.ClassifyIntent("stannah stairlift price", "en", true).Intent; got != keywordplanner.IntentNavigational {
		t.Errorf("brand keyword intent = %s, want navigational", got)
	}
}

func TestSensitiveCategoryHint(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"wärmepumpe finanzieren": "CREDIT",
		"rent to own furniture":  "CREDIT",
		"truck driver jobs":      "EMPLOYMENT",
		"wohnung mieten berlin":  "HOUSING",
		"hospital bed rental":    "",
	}
	for text, want := range cases {
		if got := keywordplanner.SensitiveCategoryHint(text); got != want {
			t.Errorf("SensitiveCategoryHint(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestScorerRow_StrongCommercialKeywordLaunches(t *testing.T) {
	t.Parallel()
	s := keywordplanner.Scorer{Language: "de", Currency: "EUR", Now: fixedNow}
	r := s.Row(keywordplanner.KeywordData{
		Text: "treppenlift mieten", AvgMonthlySearches: 2400, Competition: "HIGH",
		LowTopOfPageBidMicros: 2_100_000, HighTopOfPageBidMicros: 9_400_000, AverageCpcMicros: 3_200_000,
	})
	if r.DemandTier != keywordplanner.TierStrong || r.Verdict != keywordplanner.VerdictLaunch || r.Intent != keywordplanner.IntentTransactional {
		t.Errorf("row = tier %s verdict %s intent %s, want strong/launch/transactional", r.DemandTier, r.Verdict, r.Intent)
	}
	if r.HighTopOfPageBid != 9.40 || r.AverageCpc != 3.20 || r.AdvertiserValue != 22560 {
		t.Errorf("unit conversion wrong: high %.2f cpc %.2f value %.2f", r.HighTopOfPageBid, r.AverageCpc, r.AdvertiserValue)
	}
	if r.AfsScore < 60 {
		t.Errorf("afsScore = %d, want a high score for a strong transactional keyword", r.AfsScore)
	}
	if !strings.Contains(strings.Join(r.Reasons, " | "), "EUR") {
		t.Errorf("reasons should name the currency: %v", r.Reasons)
	}
	if r.EstimatedRpc != nil || r.BreakEvenFbCpc != nil {
		t.Error("RPC must not be estimated without rpc_ratio")
	}
}

func TestScorerRow_BrandIsSkipped(t *testing.T) {
	t.Parallel()
	s := keywordplanner.Scorer{Language: "de", Now: fixedNow}
	r := s.Row(keywordplanner.KeywordData{
		Text: "stannah treppenlift", AvgMonthlySearches: 5000, Competition: "HIGH", HighTopOfPageBidMicros: 9_000_000,
		Concepts: []keywordplanner.Concept{{Name: "Stannah", GroupType: "OTHER_BRANDS"}},
	})
	if !r.Brand.IsBrand || r.Verdict != keywordplanner.VerdictSkip || r.AfsScore > 10 {
		t.Errorf("brand row = %+v, want isBrand, skip, score <= 10", r)
	}

	// A brand name learned from other ideas in the same result set also flags this keyword.
	s.Brands = map[string]bool{"stannah": true}
	r = s.Row(keywordplanner.KeywordData{Text: "Stannah Preise", AvgMonthlySearches: 500, HighTopOfPageBidMicros: 4_000_000})
	if !r.Brand.IsBrand || !slices.Contains(r.Brand.Brands, "stannah") {
		t.Errorf("brand from result set not detected: %+v", r.Brand)
	}
}

func TestScorerRow_ScoreIsCappedByTier(t *testing.T) {
	t.Parallel()
	s := keywordplanner.Scorer{Language: "en", Now: fixedNow}
	// Huge CPC but only 50 searches: weak demand must not outrank real demand.
	weak := s.Row(keywordplanner.KeywordData{Text: "local stairlift companies", AvgMonthlySearches: 50, Competition: "HIGH",
		HighTopOfPageBidMicros: 30_000_000, AverageCpcMicros: 29_000_000})
	usable := s.Row(keywordplanner.KeywordData{Text: "stairlift companies near me", AvgMonthlySearches: 480, Competition: "HIGH",
		HighTopOfPageBidMicros: 28_000_000, AverageCpcMicros: 25_000_000})
	if weak.AfsScore > 40 || usable.AfsScore > 75 || weak.Verdict != keywordplanner.VerdictSkip {
		t.Errorf("weak=%d (%s) usable=%d, want <= 40 skip and <= 75", weak.AfsScore, weak.Verdict, usable.AfsScore)
	}
}

func TestScorerRow_RPCOnlyWithRatio(t *testing.T) {
	t.Parallel()
	s := keywordplanner.Scorer{RPCRatio: 0.12, TargetROI: 0.15, Now: fixedNow}
	r := s.Row(keywordplanner.KeywordData{Text: "x", AvgMonthlySearches: 1000, HighTopOfPageBidMicros: 5_000_000})
	if r.EstimatedRpc == nil || *r.EstimatedRpc != 0.6 {
		t.Fatalf("estimatedRpc = %v, want 0.60", r.EstimatedRpc)
	}
	if *r.BreakEvenFbCpc != 0.52 {
		t.Errorf("breakEvenFbCpc = %v, want 0.52 (0.60 / 1.15)", *r.BreakEvenFbCpc)
	}
}

func TestComputeTrend_YoYAndSeasonality(t *testing.T) {
	t.Parallel()
	// 24 months Sep 2024..Aug 2026 with a winter peak (Dec, Jan) and 50% growth in the last year.
	var vols []keywordplanner.MonthlyVolume
	for i := range 24 {
		d := time.Date(2024, time.September, 1, 0, 0, 0, 0, time.UTC).AddDate(0, i, 0)
		v := int64(1000)
		if d.Month() == time.December || d.Month() == time.January {
			v = 3000
		}
		if i >= 12 {
			v = v * 3 / 2
		}
		vols = append(vols, keywordplanner.MonthlyVolume{Year: int32(d.Year()), Month: int32(d.Month()), MonthlySearches: v})
	}
	tr := keywordplanner.ComputeTrend(vols, fixedNow)
	if tr == nil {
		t.Fatal("trend is nil")
	}
	if tr.YoYPct == nil || *tr.YoYPct != 50 || tr.Direction != "rising" {
		t.Errorf("yoy = %v direction %s, want +50 rising", tr.YoYPct, tr.Direction)
	}
	if !slices.Contains(tr.PeakMonths, 12) || !slices.Contains(tr.PeakMonths, 1) {
		t.Errorf("peak months = %v, want Dec and Jan", tr.PeakMonths)
	}
	if tr.NextPeakMonth != 12 || tr.DaysToNextPeak == nil || *tr.DaysToNextPeak != 63 {
		t.Errorf("next peak = %d in %v days, want December in 63 days", tr.NextPeakMonth, tr.DaysToNextPeak)
	}
	if lift := tr.SeasonalLift(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 20, 0, 0, 0, 0, time.UTC)); lift < 2 {
		t.Errorf("Dec-Jan lift = %.2f, want >= 2", lift)
	}
	if lift := tr.SeasonalLift(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)); lift >= 1 {
		t.Errorf("June lift = %.2f, want < 1", lift)
	}
	if keywordplanner.ComputeTrend(vols[:5], fixedNow) != nil {
		t.Error("trend with under 6 months of history must be nil")
	}
}

func TestResolveLanguage(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"de", "DE", "German", "de_DE", "de-AT", "languageConstants/1001", "1001"} {
		got, err := keywordplanner.ResolveLanguage(in)
		if err != nil || got.Constant != "languageConstants/1001" {
			t.Errorf("ResolveLanguage(%q) = %+v, %v", in, got, err)
		}
	}
	if _, err := keywordplanner.ResolveLanguage("klingon"); err == nil || !strings.Contains(err.Error(), "de,") {
		t.Errorf("unknown language error should list codes, got %v", err)
	}
}

func TestResolveCountries(t *testing.T) {
	t.Parallel()
	got, err := keywordplanner.ResolveCountries([]string{"de", "DACH", "UK", "geoTargetConstants/1004234"})
	if err != nil {
		t.Fatal(err)
	}
	var constants []string
	for _, c := range got {
		constants = append(constants, c.Constant)
	}
	want := []string{"geoTargetConstants/2276", "geoTargetConstants/2040", "geoTargetConstants/2756", "geoTargetConstants/2826", "geoTargetConstants/1004234"}
	if !slices.Equal(constants, want) {
		t.Errorf("constants = %v, want %v (deduped, order kept)", constants, want)
	}
	if _, err := keywordplanner.ResolveCountries([]string{"XX"}); err == nil || !strings.Contains(err.Error(), "DACH") {
		t.Errorf("unknown country error should list presets, got %v", err)
	}
}

func TestResolveMarket_InfersLanguageAndSplitsPerCountry(t *testing.T) {
	t.Parallel()
	m, notes, err := keywordplanner.ResolveMarket("", "", []string{"CH", "FR"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if m.Language.Code != "de" || !m.LanguageInferred || m.Network != "GOOGLE_SEARCH" || m.Label != "CH+FR" {
		t.Errorf("market = %+v", m)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "different languages") {
		t.Errorf("notes = %v, want a mixed-language warning", notes)
	}
	per := m.PerCountry(false)
	if len(per) != 2 || per[0].Language.Code != "de" || per[1].Language.Code != "fr" || per[1].Label != "FR" {
		t.Errorf("per-country markets = %+v", per)
	}
	if kept := m.PerCountry(true); kept[1].Language.Code != "de" {
		t.Errorf("explicit language must be kept per country, got %s", kept[1].Language.Code)
	}
}

func TestDedupeAndSharesTopic(t *testing.T) {
	t.Parallel()
	rows := keywordplanner.Dedupe([]keywordplanner.KeywordRow{
		{Text: "stairlift rentals", AfsScore: 40},
		{Text: "Stairlift Rental", AfsScore: 55},
		{Text: "stairlift cost", AfsScore: 50},
	})
	if len(rows) != 2 || rows[0].Text != "Stairlift Rental" || !slices.Contains(rows[0].CloseVariants, "stairlift rentals") {
		t.Errorf("dedupe = %+v", rows)
	}
	// Identical metrics mean Google treats the texts as one search: keep the first (canonical) phrasing.
	same := func(text string) keywordplanner.KeywordRow {
		return keywordplanner.KeywordRow{Text: text, AfsScore: 97, AvgMonthlySearches: 9900, LowTopOfPageBidMicros: 3_870_000,
			HighTopOfPageBidMicros: 14_910_000, CompetitionIndex: 100, AverageCpc: 16.52}
	}
	grouped := keywordplanner.Dedupe([]keywordplanner.KeywordRow{same("treppenlift kosten"), same("kosten für einen treppenlift"), same("kosten treppenlift")})
	if len(grouped) != 1 || grouped[0].Text != "treppenlift kosten" || len(grouped[0].CloseVariants) != 2 {
		t.Errorf("fingerprint dedupe = %+v", grouped)
	}
	if !keywordplanner.SharesTopic("treppenliftkosten", "treppenlift") || !keywordplanner.SharesTopic("stair lift rental", "stairlift") ||
		keywordplanner.SharesTopic("air conditioner price", "heat pump") {
		t.Error("SharesTopic misclassified a candidate")
	}
}
