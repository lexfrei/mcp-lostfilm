package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lexfrei/mcp-lostfilm/internal/lostfilm"
)

// SearchParams defines the parameters for the lostfilm_search tool.
type SearchParams struct {
	Query string `json:"query" jsonschema:"Series or movie title to search for (matches both current and older titles)"`
}

// SearchResult is the output of the lostfilm_search tool.
type SearchResult struct {
	Count   int               `json:"count"`
	Results []lostfilm.Series `json:"results"`
}

// SearchTool returns the MCP tool definition for lostfilm_search.
func SearchTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "lostfilm_search",
		Description: "Search lostfilm for series and movies by title, including the full back catalogue of older shows. Returns the series id and link needed by lostfilm_series and lostfilm_torrents. No authentication required",
		Annotations: readOnly("Search Series"),
	}
}

// NewSearchHandler creates a handler for the lostfilm_search tool.
func NewSearchHandler(client lostfilm.Client) mcp.ToolHandlerFor[SearchParams, SearchResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params SearchParams,
	) (*mcp.CallToolResult, SearchResult, error) {
		if params.Query == "" {
			return &mcp.CallToolResult{IsError: true}, SearchResult{}, validationErr(ErrQueryRequired)
		}

		results, err := client.Search(ctx, params.Query)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, SearchResult{}, lostfilmErr("search failed", err)
		}

		return nil, SearchResult{Count: len(results), Results: results}, nil
	}
}
