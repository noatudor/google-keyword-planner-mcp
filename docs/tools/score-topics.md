---
description: score_topics MCP tool -- screen candidate topics for AdSense-for-Search arbitrage and get the best offerName with bidder evidence in one call.
---

# score_topics

The daily new-topic screener. Pass the head terms you found (web search, adjacency, season) and get, per topic:

- the best non-brand **offerName** variant (highest AFS score among ideas that contain the topic),
- runner-up variants for a keyword phrasing test,
- the head term's own numbers,
- a verdict (`launch`, `borderline`, `skip`),
- a one-line **bidderEvidence** answering "who is bidding behind this keyword?", e.g.
  `wärmepumpe finanzieren · DE · 1,600/mo · HIGH · bid 2.85–12.81 EUR · avg CPC 3.10 EUR · strong · transactional · trend rising`.

Topics are ranked best first. One ideas call per topic, plus one lookup for head terms Google didn't return.

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `topics` | string[] | Yes | 1-25 head terms, in the market's language |
| `alternatives` | int | No | Runner-ups per topic. Default 3, max 5 |
| `language` | string | No | ISO code (`de`), English name (`German`) or `languageConstants/1001`. Omitted: inferred from the first country. |
| `countries` | string[] | No | ISO alpha-2 codes (`["DE","AT"]`), presets (`DACH`, `BENELUX`, `NORDICS`, `IBERIA`, `EN_CORE`, `LATAM_ES`) or `geoTargetConstants/<id>`. Omitted: worldwide. |
| `per_country` | bool | No | With 2+ countries, adds a `byCountry` breakdown and `bestCountry` to every row (one extra API call per country, max 10). |
| `network` | string | No | `GOOGLE_SEARCH` (default) or `GOOGLE_SEARCH_AND_PARTNERS`. |
| `strong_min_high_bid`, `strong_min_searches`, `usable_min_high_bid`, `usable_min_searches` | number | No | Override the [demand tier](../afs-scoring.md#demand-tiers) thresholds. |
| `rpc_ratio` | number | No | Calibrated RPC ÷ high top-of-page bid from your own tracker data. Adds `estimatedRpc` and `breakEvenFbCpc`. Never guess it. |
| `target_roi` | number | No | ROI the break-even CPC must still leave (default `0.15`). |

## Response

```json
{
  "currencyCode": "EUR", "launchable": 6,
  "topics": [{"topic": "treppenlift", "verdict": "launch", "offerName": "treppenlift kosten",
              "bidderEvidence": "...", "bestVariant": {"...": "..."}, "alternatives": [...], "headTerm": {"...": "..."},
              "ideasConsidered": 84, "strongVariants": 19}]
}
```
