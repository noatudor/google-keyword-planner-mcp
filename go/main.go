// Command google-keyword-planner-mcp is an MCP server that exposes Google Ads Keyword Planner
// as tools for AI assistants. It supports STDIO transport (default) and HTTP transport.
//
// Usage:
//
//	google-keyword-planner-mcp [--transport stdio|http]
//	    [--listen-address <address>] [--port <port>] [--allowed-hosts <list>]
//	    [--developer-token <token>] [--client-id <id>] [--client-secret <secret>]
//	    [--refresh-token <token>] [--customer-id <id>]
//
// Credential resolution order: CLI flags > environment variables > .env file.
// When --transport http, MCP_LISTEN_ADDRESS and PORT configure the listener.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ncosentino/google-keyword-planner-mcp/go/internal/config"
	"github.com/ncosentino/google-keyword-planner-mcp/go/internal/keywordplanner"
)

var version = "dev"

func main() {
	developerToken := flag.String("developer-token", "", "Google Ads developer token")
	clientID := flag.String("client-id", "", "OAuth2 client ID")
	clientSecret := flag.String("client-secret", "", "OAuth2 client secret")
	refreshToken := flag.String("refresh-token", "", "OAuth2 refresh token")
	customerID := flag.String("customer-id", "", "Google Ads customer ID")
	loginCustomerID := flag.String("login-customer-id", "", "Google Ads manager/MCC account ID (required when customer-id is a sub-account)")
	transport := flag.String("transport", "stdio", "Transport mode: stdio or http")
	listenAddress := flag.String(
		"listen-address",
		"",
		"HTTP listen address (default MCP_LISTEN_ADDRESS or 127.0.0.1)",
	)
	port := flag.Int("port", 0, "HTTP listen port (default PORT or 8080)")
	allowedHosts := flag.String("allowed-hosts", "localhost,127.0.0.1,[::1]",
		"Comma-separated Host header allow-list for --transport http (protects against DNS rebinding)")
	flag.Parse()
	explicitFlags := make(map[string]bool)
	flag.Visit(func(definedFlag *flag.Flag) {
		explicitFlags[definedFlag.Name] = true
	})

	// All diagnostic output must go to stderr to avoid corrupting the MCP STDIO stream.
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg := config.Resolve(config.Flags{
		DeveloperToken:  *developerToken,
		ClientID:        *clientID,
		ClientSecret:    *clientSecret,
		RefreshToken:    *refreshToken,
		CustomerID:      *customerID,
		LoginCustomerID: *loginCustomerID,
	})

	if !cfg.IsComplete() {
		slog.Error("incomplete Google Ads credentials",
			"hint", "set GOOGLE_ADS_DEVELOPER_TOKEN, GOOGLE_ADS_CLIENT_ID, "+
				"GOOGLE_ADS_CLIENT_SECRET, GOOGLE_ADS_REFRESH_TOKEN, GOOGLE_ADS_CUSTOMER_ID")
		os.Exit(1)
	}

	client := keywordplanner.NewClient(
		cfg.DeveloperToken, cfg.ClientID, cfg.ClientSecret, cfg.RefreshToken, cfg.CustomerID, cfg.LoginCustomerID,
		clientOptionsFromEnv()...,
	)

	srv := newServer(client)

	switch *transport {
	case "http":
		httpListenAddress, err := resolveHTTPListenAddress(
			*listenAddress,
			explicitFlags["listen-address"],
		)
		if err != nil {
			slog.Error("invalid HTTP listen address", "err", err)
			os.Exit(1)
		}
		httpPort, err := resolveHTTPPort(*port, explicitFlags["port"])
		if err != nil {
			slog.Error("invalid HTTP port", "err", err)
			os.Exit(1)
		}
		ctx, stop := signal.NotifyContext(
			context.Background(),
			os.Interrupt,
			syscall.SIGTERM,
		)
		defer stop()
		if err := runHTTP(ctx, srv, httpServerOptions{
			ListenAddress: httpListenAddress,
			Port:          httpPort,
			AllowedHosts:  splitAndTrim(*allowedHosts),
			ShutdownToken: strings.TrimSpace(os.Getenv("MCP_SHUTDOWN_TOKEN")),
		}); err != nil {
			slog.Error("server stopped with error", "err", err)
			os.Exit(1)
		}
	case "stdio":
		slog.Info("google-keyword-planner-mcp starting", "version", version, "transport", "stdio")
		if err := srv.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
			slog.Error("server stopped with error", "err", err)
			os.Exit(1)
		}
	default:
		slog.Error("invalid transport", "transport", *transport, "expected", "stdio or http")
		os.Exit(1)
	}
}

// newServer builds the MCP server with all tools and middleware registered. It is
// independent of which transport (stdio or http) will ultimately serve it.
func newServer(client *keywordplanner.Client) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "google-keyword-planner-mcp",
		Version: version,
	}, nil)

	// Repair a widespread MCP client bug where array-typed arguments arrive
	// JSON-encoded as a string instead of a genuine array (see stringified_args.go).
	srv.AddReceivingMiddleware(coerceStringifiedArrayArgs(toolArrayFields))

	registerTools(srv, client)
	return srv
}

// clientOptionsFromEnv reads KWP_MAX_QPS (requests per second, 0 disables limiting),
// KWP_CACHE_TTL (a Go duration such as "12h", 0 disables caching) and KWP_CACHE_MAX_MB
// (cache size budget in MB, 0 disables caching).
func clientOptionsFromEnv() []keywordplanner.Option {
	var opts []keywordplanner.Option
	if v := strings.TrimSpace(os.Getenv("KWP_MAX_QPS")); v != "" {
		qps, err := strconv.ParseFloat(v, 64)
		if err != nil {
			slog.Warn("ignoring invalid KWP_MAX_QPS", "value", v)
		} else {
			opts = append(opts, keywordplanner.WithMaxQPS(qps))
		}
	}
	if v := strings.TrimSpace(os.Getenv("KWP_CACHE_TTL")); v != "" {
		ttl, err := time.ParseDuration(v)
		if err != nil {
			slog.Warn("ignoring invalid KWP_CACHE_TTL", "value", v)
		} else {
			opts = append(opts, keywordplanner.WithCacheTTL(ttl))
		}
	}
	if v := strings.TrimSpace(os.Getenv("KWP_CACHE_MAX_MB")); v != "" {
		mb, err := strconv.Atoi(v)
		if err != nil {
			slog.Warn("ignoring invalid KWP_CACHE_MAX_MB", "value", v)
		} else {
			opts = append(opts, keywordplanner.WithCacheMaxBytes(mb<<20))
		}
	}
	return opts
}

// splitAndTrim splits a comma-separated flag value into a trimmed, non-empty slice.
func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
