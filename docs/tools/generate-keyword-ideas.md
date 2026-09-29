---
description: generate_keyword_ideas MCP tool -- find keywords advertisers pay for from seeds, a URL or a site, filtered, brand-free and ranked for AdSense for Search.
---

# generate_keyword_ideas

Generate related keywords from seed keywords, a URL or a whole site. Ideas are scored, brand keywords are dropped, look-alike variants are collapsed, and the top 50 are returned. Seed keywords are always returned, so the tool also works as a lookup.

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `seed_keywords` | string[] | One of | Up to 20 seed keywords |
| `url` | string | One of | A page to seed from (advertiser landing page, news article) |
| `site` | string | One of | A whole domain, e.g. `example.com` |
| `language` | string | No | ISO code (`de`), English name (`German`) or `languageConstants/1001`. Omitted: inferred from the first country. |
| `countries` | string[] | No | ISO alpha-2 codes (`["DE","AT"]`), presets (`DACH`, `BENELUX`, `NORDICS`, `IBERIA`, `EN_CORE`, `LATAM_ES`) or `geoTargetConstants/<id>`. Omitted: worldwide. |
| `per_country` | bool | No | With 2+ countries, adds a `byCountry` breakdown and `bestCountry` to every row (one extra API call per country, max 10). |
| `network` | string | No | `GOOGLE_SEARCH` (default) or `GOOGLE_SEARCH_AND_PARTNERS`. |
| `min_monthly_searches` | int | No | Minimum average monthly searches |
| `min_high_bid` | number | No | Minimum high top-of-page bid, in account currency |
| `min_average_cpc` | number | No | Minimum average CPC |
| `competition` | string[] | No | `LOW`, `MEDIUM`, `HIGH` |
| `min_demand_tier` | string | No | `weak`, `usable` or `strong` |
| `intent` | string[] | No | `transactional`, `commercial`, `informational`, `unclassified`, `navigational` |
| `exclude_brands` | bool | No | Drop Google-annotated brand keywords. Default `true` |
| `exclude_terms` | string[] | No | Drop ideas containing these words |
| `dedupe_variants` | bool | No | Collapse plural/accent twins. Default `true` |
| `sort_by` | string | No | `afs_score` (default), `advertiser_value`, `high_bid`, `average_cpc`, `searches`, `trend`, `relevance` |
| `limit` | int | No | Default 50, max 2000 |
| `include_monthly_volumes` | bool | No | Include the 24-month series. Default `false` |
| `strong_min_high_bid`, `strong_min_searches`, `usable_min_high_bid`, `usable_min_searches` | number | No | Override the [demand tier](../afs-scoring.md#demand-tiers) thresholds. |
| `rpc_ratio` | number | No | Calibrated RPC ÷ high top-of-page bid from your own tracker data. Adds `estimatedRpc` and `breakEvenFbCpc`. Never guess it. |
| `target_roi` | number | No | ROI the break-even CPC must still leave (default `0.15`). |

## Response

```json
{
  "market": {"label": "DE", "language": {"code": "de", "constant": "languageConstants/1001"}, "languageInferred": true, "countries": [...], "network": "GOOGLE_SEARCH"},
  "currencyCode": "EUR",
  "totalIdeas": 1133, "matchedFilters": 87, "brandFiltered": 12, "count": 25,
  "tierCounts": {"strong": 31, "usable": 40, "weak": 16},
  "mobileShare": 0.71,
  "ideas": [{"text": "wärmepumpe finanzieren", "demandTier": "strong", "verdict": "launch", "afsScore": 78, "...": "..."}]
}
```

Every keyword row has the [AFS keyword row](../afs-scoring.md#the-keyword-row) shape.

## Example Prompts

- _"Find strong German keywords around 'wärmepumpe' for Germany, no brands, top 25."_
- _"Which commercial keywords sit around this advertiser page: https://example.com/stairlifts?"_
