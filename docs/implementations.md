---
description: Compare the Go and C# Native AOT implementations of the Google Keyword Planner MCP server -- binary size, startup time, and when to choose each.
---

# Go vs C#

Both implementations accept the same credentials and transports. The Go implementation exposes all eight tools, including the AdSense-for-Search research tools and the `countries` / scoring parameters; the C# implementation exposes `generate_keyword_ideas`, `get_historical_metrics` and `get_keyword_forecast` with the original parameters.

## Comparison

| | Go | C# Native AOT |
|---|---|---|
| Binary size | ~12 MB | ~20 MB |
| Startup time | < 5 ms | ~20 ms |
| Memory usage | Lower | Slightly higher |
| Platforms | All | All |
| Runtime required | None | None |
| Transports | stdio, HTTP | stdio, HTTP |
| HTTP mode | Stateless | Stateless |
| Default listener | `127.0.0.1:8080` | `127.0.0.1:8080` |
| HTTP endpoints | `/mcp`, `/health` | `/mcp`, `/health` |

Both are self-contained native binaries. Both implement the same shared HTTP
contract, including Host and Origin validation. The only cosmetic difference
is the disallowed-Host status code (`403` for Go and `400` for C#).

## When to Choose Go

- You prefer smaller binary sizes.
- You're deploying on resource-constrained machines where startup time matters.
- You want to build from source using only the Go toolchain.

## When to Choose C#

- You're already in a .NET ecosystem and prefer to build from source with `dotnet publish`.
- Your organization has tooling that works better with .NET binaries.

## Which Binary Is Right for Most Users?

Choose Go for the full tool set. For the three core tools, either works fine. The server starts once and stays running -- the ~15 ms startup difference is imperceptible. Pick whichever binary matches the platform you're on (see the [Getting Started](getting-started.md) download table).
