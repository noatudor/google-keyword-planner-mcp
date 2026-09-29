---
description: get_historical_metrics MCP tool -- exact keyword lookup with bids, average CPC, 24-month trend, seasonality and brand check, per market.
---

# get_historical_metrics

Look up an exact list of keywords -- typically candidate offerNames. No new ideas are generated. Google groups close variants, so a requested keyword may be answered by a variant (`matchedAs`).

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `keywords` | string[] | Yes | Up to 500 exact keywords |
| `language` | string | No | ISO code (`de`), English name (`German`) or `languageConstants/1001`. Omitted: inferred from the first country. |
| `countries` | string[] | No | ISO alpha-2 codes (`["DE","AT"]`), presets (`DACH`, `BENELUX`, `NORDICS`, `IBERIA`, `EN_CORE`, `LATAM_ES`) or `geoTargetConstants/<id>`. Omitted: worldwide. |
| `per_country` | bool | No | With 2+ countries, adds a `byCountry` breakdown and `bestCountry` to every row (one extra API call per country, max 10). |
| `network` | string | No | `GOOGLE_SEARCH` (default) or `GOOGLE_SEARCH_AND_PARTNERS`. |
| `detect_brands` | bool | No | Check against Google's brand annotations (one extra call per 20 keywords). Default `true` |
| `include_monthly_volumes` | bool | No | Include the 24-month series. Default `true` |
| `strong_min_high_bid`, `strong_min_searches`, `usable_min_high_bid`, `usable_min_searches` | number | No | Override the [demand tier](../afs-scoring.md#demand-tiers) thresholds. |
| `rpc_ratio` | number | No | Calibrated RPC ÷ high top-of-page bid from your own tracker data. Adds `estimatedRpc` and `breakEvenFbCpc`. Never guess it. |
| `target_roi` | number | No | ROI the break-even CPC must still leave (default `0.15`). |

## Response

```json
{
  "market": {"label": "DE", "...": "..."},
  "currencyCode": "EUR",
  "keywords": [{"text": "wärmepumpe finanzieren", "avgMonthlySearches": 1600, "competition": "HIGH",
                 "lowTopOfPageBid": 2.85, "highTopOfPageBid": 12.81, "averageCpc": 3.1,
                 "demandTier": "strong", "trend": {"direction": "rising", "yoyPct": 24}, "...": "..."}],
  "count": 1,
  "notFound": [],
  "mobileShare": 0.68
}
```

Every keyword row has the [AFS keyword row](../afs-scoring.md#the-keyword-row) shape.
