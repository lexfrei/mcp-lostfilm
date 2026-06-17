package tools

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lexfrei/mcp-lostfilm/internal/lostfilm"
	"github.com/lexfrei/mcp-lostfilm/internal/torrentmeta"
)

// File permissions for a saved .torrent and its directory.
const (
	downloadDirPerm  = 0o755
	downloadFilePerm = 0o644
)

// DownloadParams defines the parameters for the lostfilm_download tool.
type DownloadParams struct {
	SeriesID   int    `json:"seriesId"             jsonschema:"Series id (from a search result or the feed)"`
	Season     int    `json:"season"               jsonschema:"Season number"`
	Episode    int    `json:"episode"              jsonschema:"Episode number; use 999 for a whole-season pack"`
	Quality    string `json:"quality,omitempty"    jsonschema:"Preferred quality label (e.g. 1080p, 720p, SD, MP4); defaults to the largest available"`
	SaveToDisk *bool  `json:"saveToDisk,omitempty" jsonschema:"Also write the .torrent to the configured download directory"`
}

// DownloadResult is the output of the lostfilm_download tool. The base64
// content is directly compatible with the transmission_torrent_add metainfo
// parameter of a sibling Transmission MCP server.
type DownloadResult struct {
	Quality        string `json:"quality,omitempty"`
	Filename       string `json:"filename"`
	ContentBase64  string `json:"contentBase64"`
	SizeBytes      int    `json:"sizeBytes"`
	InfoHash       string `json:"infoHash,omitempty"`
	FileCount      int    `json:"fileCount,omitempty"`
	TotalSizeBytes int64  `json:"totalSizeBytes,omitempty"`
	SavedPath      string `json:"savedPath,omitempty"`
}

// DownloadTool returns the MCP tool definition for lostfilm_download.
func DownloadTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "lostfilm_download",
		Description: "Download an episode's .torrent as base64 (ready to hand to a BitTorrent client), choosing a quality variant and enriched with the info-hash and file list; optionally save it to disk. Requires authentication",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Download Torrent",
			DestructiveHint: ptrBool(false),
			OpenWorldHint:   ptrBool(true),
		},
	}
}

// NewDownloadHandler creates a handler for the lostfilm_download tool. Saved
// files are written under downloadDir when saveToDisk is requested.
func NewDownloadHandler(client lostfilm.Client, downloadDir string) mcp.ToolHandlerFor[DownloadParams, DownloadResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params DownloadParams,
	) (*mcp.CallToolResult, DownloadResult, error) {
		result, err := runDownload(ctx, client, downloadDir, &params)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, DownloadResult{}, err
		}

		return nil, result, nil
	}
}

// runDownload resolves the chosen variant, downloads it, and enriches/saves it.
func runDownload(
	ctx context.Context,
	client lostfilm.Client,
	downloadDir string,
	params *DownloadParams,
) (DownloadResult, error) {
	vErr := validateEpisode(params.SeriesID, params.Season, params.Episode)
	if vErr != nil {
		return DownloadResult{}, vErr
	}

	variants, err := client.Torrents(ctx, params.SeriesID, params.Season, params.Episode)
	if err != nil {
		return DownloadResult{}, lostfilmErr("torrent resolution failed", err)
	}

	variant := pickVariant(variants, params.Quality)
	if variant == nil {
		return DownloadResult{}, lostfilmErr("download failed", ErrNoVariants)
	}

	file, err := client.Download(ctx, variant.DownloadURL)
	if err != nil {
		return DownloadResult{}, lostfilmErr("download failed", err)
	}

	result := DownloadResult{
		Quality:       variant.Quality,
		Filename:      file.Filename,
		ContentBase64: base64.StdEncoding.EncodeToString(file.Content),
		SizeBytes:     file.SizeBytes,
	}
	enrichWithMeta(&result, file.Content)

	if deref(params.SaveToDisk) {
		saved, saveErr := saveTorrent(downloadDir, file)
		if saveErr != nil {
			return DownloadResult{}, saveErr
		}

		result.SavedPath = saved
	}

	return result, nil
}

// pickVariant selects the variant whose quality label matches the request,
// falling back to the largest available when no preference is given or matched.
// Matching is done against the quality label only (normalised so "720"/"720p"
// are equivalent); the free-text description is deliberately not matched, since
// a substring like "SD" or "720p" can appear inside an unrelated description.
func pickVariant(variants []lostfilm.TorrentVariant, quality string) *lostfilm.TorrentVariant {
	if len(variants) == 0 {
		return nil
	}

	if quality != "" {
		want := normalizeQuality(quality)
		for i := range variants {
			if normalizeQuality(variants[i].Quality) == want {
				return &variants[i]
			}
		}
	}

	best := &variants[0]
	for i := range variants {
		if variants[i].SizeBytes > best.SizeBytes {
			best = &variants[i]
		}
	}

	return best
}

// normalizeQuality lower-cases a quality label and drops a trailing "p" so that
// "1080p" and "1080" compare equal.
func normalizeQuality(quality string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(quality)), "p")
}

// enrichWithMeta decodes the torrent bytes and fills in the file count, total
// size, and info-hash. Parse failures are ignored: the raw download still works.
func enrichWithMeta(result *DownloadResult, content []byte) {
	meta, err := torrentmeta.Parse(content)
	if err != nil {
		return
	}

	result.InfoHash = meta.InfoHash
	result.FileCount = meta.FileCount
	result.TotalSizeBytes = meta.TotalSizeBytes
}

// saveTorrent writes the .torrent to downloadDir, sanitising the filename to a
// base name to prevent path traversal.
func saveTorrent(downloadDir string, file *lostfilm.TorrentFile) (string, error) {
	if downloadDir == "" {
		return "", validationErr(ErrNoDownloadDir)
	}

	// Reduce the server-controlled filename to a base name, then reject the
	// dot entries explicitly so a name like ".." cannot resolve to the parent
	// directory (relying on a later EISDIR would be fragile).
	name := filepath.Base(file.Filename)
	if name == "." || name == ".." {
		name = "download.torrent"
	}

	path := filepath.Join(downloadDir, name)

	mkErr := os.MkdirAll(downloadDir, downloadDirPerm)
	if mkErr != nil {
		return "", lostfilmErr("create download directory", mkErr)
	}

	writeErr := os.WriteFile(path, file.Content, downloadFilePerm)
	if writeErr != nil {
		return "", lostfilmErr("write torrent file", writeErr)
	}

	return path, nil
}
