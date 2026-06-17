package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lexfrei/mcp-lostfilm/internal/lostfilm"
)

// SeriesParams defines the parameters for the lostfilm_series tool.
type SeriesParams struct {
	Link string `json:"link" jsonschema:"Site-relative series link from a search result, e.g. /series/Peaky_Blinders"`
}

// SeriesTool returns the MCP tool definition for lostfilm_series.
func SeriesTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "lostfilm_series",
		Description: "Get a series' metadata and full episode list (season/episode numbers, titles, dates) from its link. Use the season/episode numbers with lostfilm_torrents. No authentication required",
		Annotations: readOnly("Series Info"),
	}
}

// NewSeriesHandler creates a handler for the lostfilm_series tool.
func NewSeriesHandler(client lostfilm.Client) mcp.ToolHandlerFor[SeriesParams, lostfilm.SeriesInfo] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params SeriesParams,
	) (*mcp.CallToolResult, lostfilm.SeriesInfo, error) {
		if params.Link == "" {
			return &mcp.CallToolResult{IsError: true}, lostfilm.SeriesInfo{}, validationErr(ErrLinkRequired)
		}

		info, err := client.SeriesInfo(ctx, params.Link)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, lostfilm.SeriesInfo{}, lostfilmErr("series lookup failed", err)
		}

		return nil, *info, nil
	}
}
