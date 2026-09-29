---
description: All MCP tools exposed by the Google Keyword Planner MCP server -- keyword ideas, exact lookups, forecasts, and AdSense-for-Search topic screening.
---

# MCP Tools

The Go server exposes eight tools built around one question: **what do Google advertisers pay for this keyword, in this market?** Every keyword row is scored the same way (see [AFS scoring](../afs-scoring.md)), so no post-processing is needed. All tools except `get_targeting_reference` require valid credentials -- see [Configuration](../configuration.md).

## Tool Overview

| Tool | Purpose |
|------|---------|
| [`generate_keyword_ideas`](generate-keyword-ideas/) | Related keywords from seeds, a URL or a site: filtered, brand-free, deduplicated, ranked |
| [`get_historical_metrics`](get-historical-metrics/) | Exact-keyword lookup with average CPC, 24-month trend and brand check |
| [`get_keyword_forecast`](get-keyword-forecast/) | Projected clicks, CTR, average CPC and cost per keyword at a max CPC |
| [`score_topics`](score-topics/) | Screen up to 25 candidate topics: best offerName, runner-ups, bidder evidence, verdict |
| [`find_offer_variants`](find-offer-variants/) | Expand one topic with commercial modifiers and recommend the exact offerName |
| [`compare_keyword_markets`](compare-keyword-markets/) | The same keywords across up to 8 markets, with a market ranking |
| [`find_seasonal_opportunities`](find-seasonal-opportunities/) | Commercial keywords peaking in an upcoming launch window |
| [`get_targeting_reference`](get-targeting-reference/) | Language and country codes, presets, thresholds and lexicons (no API quota) |

The C# build implements only the first three tools, with the original parameters.

## Common Notes

**Currency:** bids and CPCs are in the Google Ads account currency, returned as `currencyCode`. The raw `*Micros` fields are kept (1,000,000 = 1.00).

**Markets:** always pass `countries` for bid decisions -- bids differ a lot per country. The language is inferred from the first country when omitted.

**Quota:** the client paces requests (`KWP_MAX_QPS`, default 1/s), retries once on quota errors and caches identical requests (`KWP_CACHE_TTL`, default 12h). Fan-out tools (`score_topics`, `per_country`) cost more calls and take longer.

**Search volume precision:** without active ad spend on the account, Google returns search volumes as ranges rather than exact numbers.

**Errors** come back as `{"error": "..."}` with `isError: true`, including Google's error status and code.
