package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lexfrei/mcp-lostfilm/internal/lostfilm"
)

// FeedParams defines the parameters for the lostfilm_feed tool.
type FeedParams struct {
	Limit int `json:"limit,omitempty" jsonschema:"Maximum number of feed items to return (0 = all)"`
}

// FeedResult is the output of the lostfilm_feed tool.
type FeedResult struct {
	Count int                 `json:"count"`
	Items []lostfilm.FeedItem `json:"items"`
}

// FeedTool returns the MCP tool definition for lostfilm_feed.
func FeedTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "lostfilm_feed",
		Description: "List the latest lostfilm releases from the public RSS feed (the equivalent of subscribing to new episodes). No authentication required",
		Annotations: readOnly("Release Feed"),
	}
}

// NewFeedHandler creates a handler for the lostfilm_feed tool.
func NewFeedHandler(client lostfilm.Client) mcp.ToolHandlerFor[FeedParams, FeedResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params FeedParams,
	) (*mcp.CallToolResult, FeedResult, error) {
		items, err := client.Feed(ctx)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, FeedResult{}, lostfilmErr("feed failed", err)
		}

		if params.Limit > 0 && len(items) > params.Limit {
			items = items[:params.Limit]
		}

		return nil, FeedResult{Count: len(items), Items: items}, nil
	}
}
