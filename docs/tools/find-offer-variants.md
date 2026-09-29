---
description: find_offer_variants MCP tool -- expand a topic with commercial modifiers in the market language and recommend the exact offerName to launch.
---

# find_offer_variants

Pick the exact offerName for one topic. The head term is expanded with the market language's commercial modifiers -- cost, price, financing, rental, rent to own, near me, for seniors, grant, used, installation, ... (en, de, fr, es, it, nl, pt, pl, sv, da, no, fi) -- plus Google's own related ideas that contain the topic. All candidates are looked up exactly, scored, deduplicated and ranked.

The `recommendation` names the `offerName` and a `runnerUp` with a different modifier for a keyword phrasing test.

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `topic` | string | Yes | Head term, e.g. `treppenlift` |
| `extra_modifiers` | string[] | No | More variants: templates with `{t}` (`{t} für zuhause`) or words appended to the topic |
| `include_ideas` | bool | No | Add Google's related ideas. Default `true` |
| `limit` | int | No | Default 20 |
| `language` | string | No | ISO code (`de`), English name (`German`) or `languageConstants/1001`. Omitted: inferred from the first country. |
| `countries` | string[] | No | ISO alpha-2 codes (`["DE","AT"]`), presets (`DACH`, `BENELUX`, `NORDICS`, `IBERIA`, `EN_CORE`, `LATAM_ES`) or `geoTargetConstants/<id>`. Omitted: worldwide. |
| `per_country` | bool | No | With 2+ countries, adds a `byCountry` breakdown and `bestCountry` to every row (one extra API call per country, max 10). |
| `network` | string | No | `GOOGLE_SEARCH` (default) or `GOOGLE_SEARCH_AND_PARTNERS`. |
| `strong_min_high_bid`, `strong_min_searches`, `usable_min_high_bid`, `usable_min_searches` | number | No | Override the [demand tier](../afs-scoring.md#demand-tiers) thresholds. |
| `rpc_ratio` | number | No | Calibrated RPC ÷ high top-of-page bid from your own tracker data. Adds `estimatedRpc` and `breakEvenFbCpc`. Never guess it. |
| `target_roi` | number | No | ROI the break-even CPC must still leave (default `0.15`). |

Each variant row carries `source` (`template`, `extra`, `idea`) and `modifier`.
