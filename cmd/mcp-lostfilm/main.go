// Command mcp-lostfilm is an MCP server exposing lostfilm.tv release-feed,
// search, and torrent tools over stdio and, optionally, HTTP.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/sync/errgroup"

	"github.com/lexfrei/mcp-lostfilm/internal/config"
	"github.com/lexfrei/mcp-lostfilm/internal/lostfilm"
	"github.com/lexfrei/mcp-lostfilm/internal/tools"
)

const (
	serverName        = "mcp-lostfilm"
	readHeaderTimeout = 10 * time.Second
	shutdownTimeout   = 5 * time.Second
)

// version and revision are set via ldflags at build time.
var (
	version  = "dev"
	revision = "unknown"
)

func main() {
	logger := newLogger()

	err := run(logger)
	if err != nil {
		logger.Error("server failed", slog.Any("error", err))
		os.Exit(1)
	}
}

// newLogger builds the structured JSON logger. Logs go to stderr because stdout
// carries the JSON-RPC stream.
func newLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

func run(logger *slog.Logger) error {
	cfg, cfgErr := config.Load()
	if cfgErr != nil {
		return errors.Wrap(cfgErr, "invalid configuration")
	}

	// Discovery (feed, search, series) needs no credentials; only torrent
	// resolution does. Warn rather than fail so the server still serves the
	// public tools when no credentials are configured.
	if !cfg.HasCredentials() {
		logger.Warn("no credentials configured: feed and search work, but " +
			"torrent resolution requires LOSTFILM_EMAIL/LOSTFILM_PASSWORD or LOSTFILM_COOKIE")
	}

	transport, transportErr := cfg.ProxyTransport()
	if transportErr != nil {
		return errors.Wrap(transportErr, "invalid proxy configuration")
	}

	client, clientErr := lostfilm.New(&lostfilm.Options{
		BaseURL:    cfg.BaseURL,
		Email:      cfg.Email,
		Password:   cfg.Password,
		Cookie:     cfg.Cookie,
		CookiePath: cfg.CookieFile,
		UserAgent:  cfg.UserAgent,
		Transport:  transport,
	})
	if clientErr != nil {
		return errors.Wrap(clientErr, "failed to create lostfilm client")
	}

	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    serverName,
			Version: version + "+" + revision,
		},
		newServerOptions(logger),
	)

	registerTools(server, client, cfg.DownloadDir)

	return serve(logger, server, cfg)
}

// newServerOptions wires the shared logger into the MCP server so its internal
// logs share the structured JSON format used by the rest of the binary.
func newServerOptions(logger *slog.Logger) *mcp.ServerOptions {
	return &mcp.ServerOptions{
		Instructions: "MCP server for lostfilm.tv. Provides tools to browse the " +
			"release feed (the RSS equivalent), search current and older series, " +
			"inspect a series' episodes, resolve an episode's torrent quality " +
			"variants, and download .torrent files (returned as base64, ready to " +
			"hand to a BitTorrent client). Feed and search are public; torrent " +
			"resolution and download require authentication via " +
			"LOSTFILM_EMAIL/LOSTFILM_PASSWORD or a LOSTFILM_COOKIE session " +
			"override. With no LOSTFILM_BASE_URL set, the client round-robins " +
			"across known lostfilm mirrors.",
		Logger: logger,
	}
}

func registerTools(server *mcp.Server, client lostfilm.Client, downloadDir string) {
	mcp.AddTool(server, tools.ServerVersionTool(),
		tools.NewServerVersionHandler(version, revision, runtime.Version()))
	mcp.AddTool(server, tools.FeedTool(), tools.NewFeedHandler(client))
	mcp.AddTool(server, tools.SearchTool(), tools.NewSearchHandler(client))
	mcp.AddTool(server, tools.SeriesTool(), tools.NewSeriesHandler(client))
	mcp.AddTool(server, tools.TorrentsTool(), tools.NewTorrentsHandler(client))
	mcp.AddTool(server, tools.DownloadTool(), tools.NewDownloadHandler(client, downloadDir))
}

// serve runs the stdio transport and, when configured, an HTTP transport.
func serve(logger *slog.Logger, server *mcp.Server, cfg *config.Config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-sigChan:
			cancel()
		case <-ctx.Done():
		}

		signal.Stop(sigChan)
	}()

	group, groupCtx := errgroup.WithContext(ctx)
	httpEnabled := cfg.HTTPEnabled()

	group.Go(func() error {
		runErr := server.Run(groupCtx, &mcp.StdioTransport{})
		if runErr != nil && groupCtx.Err() == nil {
			return errors.Wrap(runErr, "stdio server failed")
		}

		if !httpEnabled {
			cancel()
		}

		return nil
	})

	if httpEnabled {
		group.Go(func() error {
			return runHTTPServer(groupCtx, logger, server, cfg.HTTPAddr())
		})
	}

	//nolint:wrapcheck // errors are already wrapped inside the group goroutines.
	return group.Wait()
}

// runHTTPServer starts an HTTP/SSE transport for the MCP server. Sharing a
// single *mcp.Server across transports is safe: the SDK guards internal state
// with a mutex.
func runHTTPServer(ctx context.Context, logger *slog.Logger, server *mcp.Server, addr string) error {
	handler := mcp.NewStreamableHTTPHandler(
		func(_ *http.Request) *mcp.Server { return server },
		nil,
	)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	//nolint:gosec // G118: shutdown uses a fresh context because ctx is already cancelled.
	go func() {
		<-ctx.Done()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer shutdownCancel()

		shutdownErr := httpServer.Shutdown(shutdownCtx) //nolint:contextcheck // fresh context for graceful shutdown.
		if shutdownErr != nil {
			logger.Error("http server shutdown failed", slog.Any("error", shutdownErr))
		}
	}()

	logger.Info("http server listening", slog.String("addr", addr))

	listenErr := httpServer.ListenAndServe()
	if errors.Is(listenErr, http.ErrServerClosed) {
		return nil
	}

	return errors.Wrap(listenErr, "HTTP listen failed")
}
