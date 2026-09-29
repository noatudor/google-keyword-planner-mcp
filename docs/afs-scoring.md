---
description: How the Keyword Planner MCP scores keywords for AdSense-for-Search arbitrage -- demand tiers, intent, brand flags, trend, AFS score and RPC calibration.
---

# AFS Scoring

The tools are built for AdSense-for-Search (AFS) arbitrage, not SEO. The revenue per click on a search feed page is set by what Google's advertisers bid on the landed keyword, so the planner's bid data is the RPC signal. Every tool returns keyword rows scored the same way.

## The keyword row

| Field | Meaning |
|-------|---------|
| `avgMonthlySearches`, `competition`, `competitionIndex` | Planner volume and competition for the market |
| `lowTopOfPageBid`, `highTopOfPageBid` | 20th / 80th percentile top-of-page bid, in `currencyCode` |
| `averageCpc` | What advertisers actually paid per click |
| `advertiserValue` | `avgMonthlySearches × highTopOfPageBid`: the monthly money behind the keyword |
| `demandTier` | `strong`, `usable` or `weak` (below) |
| `commercialIntent` | `transactional`, `commercial`, `informational`, `navigational` (brand) or `unclassified` |
| `intentModifiers` | The modifier words found, e.g. `["finanzieren"]` |
| `sensitiveCategoryHint` | `CREDIT`, `EMPLOYMENT` or `HOUSING` wording. Informational only -- Adspy assigns the special ad category; never set it yourself |
| `brand` | `isBrand`, `brands` and `checked`, from Google's BRAND / OTHER_BRANDS concept annotations |
| `trend` | `direction` (rising/flat/falling), `yoyPct`, `momentum3mPct`, `peakMonths`, `seasonalityIndex`, `nextPeakMonth`, `daysToNextPeak` |
| `afsScore`, `verdict`, `reasons` | 0-100 score, `launch` / `borderline` / `skip`, and the human-readable evidence |
| `estimatedRpc`, `breakEvenFbCpc` | Only with `rpc_ratio` (below) |
| `byCountry`, `bestCountry` | Only with `per_country: true` |

## Demand tiers

| Tier | Rule (defaults, overridable per call) |
|------|------|
| strong | high bid ≥ 2.00, competition HIGH or MEDIUM, ≥ 500 searches |
| usable | high bid 1.00-2.00, or strong-level bids at 100-500 searches (or with LOW competition) |
| weak | everything else, and anything under 10 searches |

## AFS score and verdict

The score weighs price 40 (average CPC, falling back to the high bid, log-scaled up to 8.00), volume 20 (log-scaled up to 50,000), competition 15, commercial intent 15 and trend 10. It is capped at 40 for weak and 75 for usable demand, so a thin keyword with a huge CPC never outranks real demand.

- **skip:** brand keywords (score capped at 10), navigational wording, weak demand.
- **launch:** strong demand with commercial or transactional wording, unless demand fell 30%+ year over year.
- **borderline:** everything in between, e.g. usable tiers or strong bids with informational wording.

## Look-alike variants

Rows are collapsed when their text normalizes to the same key (case, accents, punctuation, plural "s"), or when Google reports an identical metric fingerprint -- Google gives exactly the same numbers to phrasings it treats as one search ("kosten treppenlift", "kosten für einen treppenlift"). The best-scoring row is kept, ties keep Google's first (canonical) phrasing, and the others are listed in `closeVariants`.

## Intent

Intent comes from modifier lexicons per language (English is always checked too). In German, Dutch and the Nordic languages the market language's modifiers also match inside compounds (`treppenliftkosten`), after removing informational words so `kostenlos` does not count as `kosten`. `get_targeting_reference` lists every lexicon.

## RPC calibration

The server never assumes an RPC. Once your tracker shows real RPC next to the planner's high bid for enough topics, pass the ratio as `rpc_ratio` (e.g. `0.12` = RPC is 12% of the high bid). Rows then get:

- `estimatedRpc = rpc_ratio × highTopOfPageBid`
- `breakEvenFbCpc = estimatedRpc / (1 + target_roi)`, with `target_roi` defaulting to 0.15.
