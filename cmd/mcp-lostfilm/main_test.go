package main

import (
	"log/slog"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lexfrei/mcp-lostfilm/internal/lostfilm"
)

// testLogger discards log output to keep test runs quiet.
func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func TestRegisterTools_ListsAllTools(t *testing.T) {
	t.Parallel()

	client, err := lostfilm.New(&lostfilm.Options{})
	if err != nil {
		t.Fatalf("lostfilm.New: %v", err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: serverName, Version: "test"}, newServerOptions(testLogger()))
	registerTools(server, client, "")

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer func() { _ = serverSession.Close() }()

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)

	clientSession, err := mcpClient.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer func() { _ = clientSession.Close() }()

	result, err := clientSession.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	got := make(map[string]bool, len(result.Tools))
	for _, tool := range result.Tools {
		got[tool.Name] = true
	}

	want := []string{
		"lostfilm_server_version",
		"lostfilm_feed",
		"lostfilm_search",
		"lostfilm_series",
		"lostfilm_torrents",
		"lostfilm_download",
	}

	if len(got) != len(want) {
		t.Errorf("tool count = %d, want %d (%v)", len(got), len(want), result.Tools)
	}

	for _, name := range want {
		if !got[name] {
			t.Errorf("missing tool %q", name)
		}
	}
}

func TestNewServerOptions_HasInstructions(t *testing.T) {
	t.Parallel()

	opts := newServerOptions(testLogger())
	if opts.Instructions == "" {
		t.Error("server instructions must not be empty")
	}

	if opts.Logger == nil {
		t.Error("server logger must be set")
	}
}

func TestRun_StartsWithoutCredentials(t *testing.T) {
	t.Parallel()

	// The server must build a client even with no credentials, since discovery
	// tools work without a session.
	client, err := lostfilm.New(&lostfilm.Options{})
	if err != nil {
		t.Fatalf("lostfilm.New without credentials: %v", err)
	}

	if client == nil {
		t.Fatal("expected a non-nil client")
	}
}
