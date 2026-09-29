---
description: find_seasonal_opportunities MCP tool -- find commercial keywords whose demand peaks in an upcoming launch window or is rising year over year.
---

# find_seasonal_opportunities

Find commercial, non-brand keywords whose historical demand peaks inside an upcoming launch window (default 14-42 days out, to cover the review lag) or is rising year over year. Ranked by advertiser value × seasonal lift. Each row gets `seasonalLift` (window-month volume ÷ yearly average) and a "why now" reason.

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `seed_keywords` | string[] | One of | Up to 20 seeds, e.g. `["heizung", "winterreifen"]` |
| `url` | string | One of | A page to seed from, e.g. an article about a new subsidy |
| `window_start_days` | int | No | Default 14 |
| `window_end_days` | int | No | Default 42 |
| `min_lift` | number | No | Default 1.2 |
| `include_rising` | bool | No | Also include YoY-rising keywords without a peak. Default `true` |
| `min_demand_tier` | string | No | Default `usable` |
| `exclude_terms` | string[] | No | Drop ideas containing these words |
| `limit` | int | No | Default 30 |
| `language` | string | No | ISO code (`de`), English name (`German`) or `languageConstants/1001`. Omitted: inferred from the first country. |
| `countries` | string[] | No | ISO alpha-2 codes (`["DE","AT"]`), presets (`DACH`, `BENELUX`, `NORDICS`, `IBERIA`, `EN_CORE`, `LATAM_ES`) or `geoTargetConstants/<id>`. Omitted: worldwide. |
| `per_country` | bool | No | With 2+ countries, adds a `byCountry` breakdown and `bestCountry` to every row (one extra API call per country, max 10). |
| `network` | string | No | `GOOGLE_SEARCH` (default) or `GOOGLE_SEARCH_AND_PARTNERS`. |
| `strong_min_high_bid`, `strong_min_searches`, `usable_min_high_bid`, `usable_min_searches` | number | No | Override the [demand tier](../afs-scoring.md#demand-tiers) thresholds. |
| `rpc_ratio` | number | No | Calibrated RPC ÷ high top-of-page bid from your own tracker data. Adds `estimatedRpc` and `breakEvenFbCpc`. Never guess it. |
| `target_roi` | number | No | ROI the break-even CPC must still leave (default `0.15`). |
