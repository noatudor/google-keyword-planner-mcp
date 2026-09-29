---
description: compare_keyword_markets MCP tool -- compare keywords across countries and languages to take a winning topic into a new market.
---

# compare_keyword_markets

Compare the same keywords across up to 8 markets. Returns, per keyword, each market's searches, bids, average CPC, tier, score and verdict plus the `bestMarket`, and a `marketRanking` by strong keywords and total advertiser value.

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `keywords` | string[] | Yes | 1-20 keywords |
| `markets` | object[] | Yes | 1-8 markets: `{"label": "UK", "language": "en", "countries": ["GB"]}`. Language is inferred from the first country when omitted |
| `network` | string | No | `GOOGLE_SEARCH` (default) or `GOOGLE_SEARCH_AND_PARTNERS` |
| `strong_min_high_bid`, `strong_min_searches`, `usable_min_high_bid`, `usable_min_searches` | number | No | Override the [demand tier](../afs-scoring.md#demand-tiers) thresholds. |
| `rpc_ratio` | number | No | Calibrated RPC ÷ high top-of-page bid from your own tracker data. Adds `estimatedRpc` and `breakEvenFbCpc`. Never guess it. |
| `target_roi` | number | No | ROI the break-even CPC must still leave (default `0.15`). |

## Example

```json
{"keywords": ["stairlift rental", "hospital bed rental"],
  "markets": [{"countries": ["US"]}, {"countries": ["GB"]}, {"countries": ["CA"]}, {"countries": ["AU"]}]}
```

For a different language, pass the keyword in that language too -- the planner does not translate.
