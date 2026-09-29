---
description: get_keyword_forecast MCP tool -- forecast clicks, CTR, average CPC and cost per keyword for a Google Search campaign at a given max CPC, per market.
---

# get_keyword_forecast

Forecast a simulated manual-CPC Google Search campaign. The v23 API only returns campaign totals, so each keyword is forecast on its own, plus one combined total. The average CPC shows the price at which advertisers actually clear at that bid.

## Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `keywords` | string[] | Yes | Up to 20 keywords |
| `max_cpc` | number | No | Max CPC in account currency (e.g. `2.5`). Overrides `max_cpc_micros` |
| `max_cpc_micros` | int | No | Max CPC in micros. Default `1000000` |
| `daily_budget` | number | No | Optional daily budget in account currency |
| `forecast_days` | int | No | Days to forecast, starting tomorrow. Default 30, max 365 |
| `match_type` | string | No | `BROAD` (default), `PHRASE` or `EXACT` |
| `language`, `countries`, `network` | | No | Market, as for the other tools |

## Response

```json
{
  "currencyCode": "EUR", "forecastDays": 30, "maxCpc": 2.5, "maxCpcMicros": 2500000, "matchType": "BROAD",
  "total": {"impressions": 18000, "clicks": 610, "ctr": 0.034, "averageCpc": 1.92, "cost": 1171.2},
  "keywords": [{"text": "treppenlift mieten", "impressions": 9000, "clicks": 320, "ctr": 0.036, "averageCpc": 2.05, "cost": 656}]
}
```

A keyword whose own forecast fails gets an `error` field; the others are still returned.
