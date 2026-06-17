package tools

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lexfrei/mcp-lostfilm/internal/lostfilm"
)

// TorrentsParams defines the parameters for the lostfilm_torrents tool.
type TorrentsParams struct {
	SeriesID int `json:"seriesId" jsonschema:"Series id (from a search result or the feed)"`
	Season   int `json:"season"   jsonschema:"Season number"`
	Episode  int `json:"episode"  jsonschema:"Episode number; use 999 for a whole-season pack"`
}

// TorrentsResult is the output of the lostfilm_torrents tool.
type TorrentsResult struct {
	Count    int                       `json:"count"`
	Variants []lostfilm.TorrentVariant `json:"variants"`
}

// TorrentsTool returns the MCP tool definition for lostfilm_torrents.
func TorrentsTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "lostfilm_torrents",
		Description: "Resolve the available quality variants (SD/720p/1080p/...) and their .torrent download links for an episode. Requires authentication (LOSTFILM_EMAIL/LOSTFILM_PASSWORD or LOSTFILM_COOKIE)",
		Annotations: readOnly("Resolve Torrents"),
	}
}

// NewTorrentsHandler creates a handler for the lostfilm_torrents tool.
func NewTorrentsHandler(client lostfilm.Client) mcp.ToolHandlerFor[TorrentsParams, TorrentsResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params TorrentsParams,
	) (*mcp.CallToolResult, TorrentsResult, error) {
		vErr := validateEpisode(params.SeriesID, params.Season, params.Episode)
		if vErr != nil {
			return &mcp.CallToolResult{IsError: true}, TorrentsResult{}, vErr
		}

		variants, err := client.Torrents(ctx, params.SeriesID, params.Season, params.Episode)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, TorrentsResult{}, lostfilmErr("torrent resolution failed", err)
		}

		return nil, TorrentsResult{Count: len(variants), Variants: variants}, nil
	}
}

// validateEpisode checks the series/season/episode triple is well-formed.
func validateEpisode(seriesID, season, episode int) error {
	switch {
	case seriesID <= 0:
		return validationErr(ErrSeriesIDRequired)
	case season <= 0:
		return validationErr(ErrSeasonRequired)
	case episode <= 0:
		return validationErr(ErrEpisodeRequired)
	default:
		return nil
	}
}
