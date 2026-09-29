using System.Net;
using System.Text.Json;
using KeywordPlannerMcp.KeywordPlanner;
using Xunit;

namespace KeywordPlannerMcp.Tests;

/// <summary>
/// Tests for KeywordPlannerClient focusing on header correctness and error visibility.
/// Uses a fake HttpMessageHandler to avoid real network calls.
/// </summary>
public sealed class KeywordPlannerClientTests
{
    /// <summary>
    /// Fake handler that intercepts all HTTP requests. Handles the OAuth2 token endpoint
    /// with a canned success response and all other URLs with a configurable response.
    /// </summary>
    private sealed class FakeHttpHandler : HttpMessageHandler
    {
        private readonly HttpStatusCode _apiStatusCode;
        private readonly string _apiResponseBody;
        public List<HttpRequestMessage> ApiRequests { get; } = [];
        public List<string> ApiBodies { get; } = [];

        internal FakeHttpHandler(
            HttpStatusCode apiStatusCode = HttpStatusCode.OK,
            string? apiResponseBody = null)
        {
            _apiStatusCode = apiStatusCode;
            _apiResponseBody = apiResponseBody ?? JsonSerializer.Serialize(
                new { results = Array.Empty<object>() });
        }

        protected override async Task<HttpResponseMessage> SendAsync(
            HttpRequestMessage request, CancellationToken cancellationToken)
        {
            // Satisfy the OAuth2 token refresh call.
            if (request.RequestUri?.Host == "oauth2.googleapis.com")
            {
                var tokenJson = JsonSerializer.Serialize(new
                {
                    access_token = "fake-token",
                    expires_in = 3600,
                    token_type = "Bearer"
                });
                return new HttpResponseMessage(HttpStatusCode.OK)
                {
                    Content = new StringContent(tokenJson, System.Text.Encoding.UTF8, "application/json")
                };
            }

            ApiRequests.Add(request);
            ApiBodies.Add(request.Content is null ? "" : await request.Content.ReadAsStringAsync(cancellationToken));
            return new HttpResponseMessage(_apiStatusCode)
            {
                Content = new StringContent(_apiResponseBody, System.Text.Encoding.UTF8, "application/json")
            };
        }
    }

    private static (KeywordPlannerClient client, FakeHttpHandler handler) CreateClient(
        string? loginCustomerId = null,
        HttpStatusCode apiStatus = HttpStatusCode.OK,
        string? apiBody = null)
    {
        var handler = new FakeHttpHandler(apiStatus, apiBody);
        var httpClient = new HttpClient(handler);
        var tokenProvider = new OAuth2TokenProvider("client-id", "client-secret", "refresh-token", httpClient);
        var apiHttpClient = new HttpClient(handler);
        var client = new KeywordPlannerClient("dev-token", "3778350596", loginCustomerId, tokenProvider, apiHttpClient);
        return (client, handler);
    }

    [Fact]
    public async Task GenerateKeywordIdeas_SendsLoginCustomerIdHeader_WhenSet()
    {
        var (client, handler) = CreateClient(loginCustomerId: "1381404200");

        await client.GenerateKeywordIdeasAsync(["go programming"], null, null);

        var apiRequest = Assert.Single(handler.ApiRequests);
        Assert.True(apiRequest.Headers.TryGetValues("login-customer-id", out var values));
        Assert.Equal("1381404200", values.Single());
    }

    [Fact]
    public async Task GenerateKeywordIdeas_OmitsLoginCustomerIdHeader_WhenNotSet()
    {
        var (client, handler) = CreateClient(loginCustomerId: null);

        await client.GenerateKeywordIdeasAsync(["go programming"], null, null);

        var apiRequest = Assert.Single(handler.ApiRequests);
        Assert.False(apiRequest.Headers.Contains("login-customer-id"));
    }

    [Fact]
    public async Task GetHistoricalMetrics_SendsLoginCustomerIdHeader_WhenSet()
    {
        var historicalBody = JsonSerializer.Serialize(new { results = Array.Empty<object>() });
        var (client, handler) = CreateClient(loginCustomerId: "1381404200", apiBody: historicalBody);

        await client.GetHistoricalMetricsAsync(["blazor"]);

        var apiRequest = Assert.Single(handler.ApiRequests);
        Assert.True(apiRequest.Headers.TryGetValues("login-customer-id", out var values));
        Assert.Equal("1381404200", values.Single());
    }

    [Fact]
    public async Task GetKeywordForecast_SendsLoginCustomerIdHeader_WhenSet()
    {
        var forecastBody = JsonSerializer.Serialize(new { campaignForecastMetrics = new { } });
        var (client, handler) = CreateClient(loginCustomerId: "1381404200", apiBody: forecastBody);

        await client.GetKeywordForecastAsync(["blazor"], maxCpcMicros: 1_000_000, forecastDays: 30);

        var apiRequest = Assert.Single(handler.ApiRequests);
        Assert.True(apiRequest.Headers.TryGetValues("login-customer-id", out var values));
        Assert.Equal("1381404200", values.Single());
    }

    [Fact]
    public async Task GetHistoricalMetrics_ParsesV23ResultsWithStringInt64s()
    {
        var body = """
            {"results": [{"text": "blazor", "keywordMetrics": {"avgMonthlySearches": "1600", "competition": "HIGH",
              "competitionIndex": "87", "highTopOfPageBidMicros": "12810000",
              "monthlySearchVolumes": [{"year": "2026", "month": "AUGUST", "monthlySearches": "1900"}]}}]}
            """;
        var (client, _) = CreateClient(apiBody: body);

        var result = await client.GetHistoricalMetricsAsync(["blazor"]);

        var metrics = Assert.Single(result.Keywords);
        Assert.Equal(1600, metrics.AvgMonthlySearches);
        Assert.Equal(87, metrics.CompetitionIndex);
        Assert.Equal(12_810_000, metrics.HighTopOfPageBidMicros);
        Assert.Equal(new MonthlyVolume(2026, 8, 1900), Assert.Single(metrics.MonthlySearchVolumes));
    }

    [Fact]
    public async Task GetKeywordForecast_UsesV23CampaignShape()
    {
        var (client, handler) = CreateClient(apiBody: """{"campaignForecastMetrics": {"clicks": 12.5, "costMicros": "3000000"}}""");

        var result = await client.GetKeywordForecastAsync(["blazor", "maui"], maxCpcMicros: 2_000_000, forecastDays: 30);

        using var request = JsonDocument.Parse(Assert.Single(handler.ApiBodies));
        Assert.False(request.RootElement.TryGetProperty("campaignForecastSpec", out _));
        var campaign = request.RootElement.GetProperty("campaign");
        Assert.Equal("GOOGLE_SEARCH", campaign.GetProperty("keywordPlanNetwork").GetString());
        Assert.Equal("2000000", campaign.GetProperty("biddingStrategy").GetProperty("manualCpcBiddingStrategy").GetProperty("maxCpcBidMicros").GetString());
        Assert.Equal(2, campaign.GetProperty("adGroups")[0].GetProperty("biddableKeywords").GetArrayLength());
        var start = DateTime.Parse(request.RootElement.GetProperty("forecastPeriod").GetProperty("startDate").GetString()!);
        Assert.True(start > DateTime.UtcNow.Date, "forecast must start in the future");

        var total = Assert.Single(result.Keywords);
        Assert.Equal("blazor, maui", total.Text);
        Assert.Equal(12.5, total.Clicks);
        Assert.Equal(3_000_000, total.CostMicros);
    }

    [Fact]
    public async Task GenerateKeywordIdeas_AlwaysSendsDeveloperTokenHeader()
    {
        var (client, handler) = CreateClient();

        await client.GenerateKeywordIdeasAsync(["go programming"], null, null);

        var apiRequest = Assert.Single(handler.ApiRequests);
        Assert.True(apiRequest.Headers.TryGetValues("developer-token", out var values));
        Assert.Equal("dev-token", values.Single());
    }

    [Fact]
    public async Task GenerateKeywordIdeas_ThrowsWithFullErrorBody_OnApiError()
    {
        var longBody = new string('x', 500);
        var (client, _) = CreateClient(apiStatus: HttpStatusCode.BadRequest, apiBody: longBody);

        var ex = await Assert.ThrowsAsync<InvalidOperationException>(
            () => client.GenerateKeywordIdeasAsync(["test"], null, null));

        // Error message must contain the full body — no truncation.
        Assert.Contains(longBody, ex.Message);
    }
}
