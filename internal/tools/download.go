package tools

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lexfrei/mcp-lostfilm/internal/artifact"
	"github.com/lexfrei/mcp-lostfilm/internal/lostfilm"
	"github.com/lexfrei/mcp-lostfilm/internal/torrentmeta"
)

// Download delivery modes.
const (
	modeMetadata = "metadata"
	modeBase64   = "base64"
	modeArtifact = "artifact"
)

// DownloadParams defines the parameters for the lostfilm_download tool.
type DownloadParams struct {
	SeriesID int    `json:"seriesId"          jsonschema:"Series id (from a search result or the feed)"`
	Season   int    `json:"season"            jsonschema:"Season number"`
	Episode  int    `json:"episode"           jsonschema:"Episode number; use 999 for a whole-season pack"`
	Quality  string `json:"quality,omitempty" jsonschema:"Preferred quality label (e.g. 1080p, 720p, SD, MP4); defaults to the largest available"`
	Mode     string `json:"mode,omitempty"    jsonschema:"How to deliver the .torrent: 'metadata' (info only), 'base64' (inline content for piping to a torrent client), or 'artifact' (a one-time download URL; requires the HTTP transport). Default: artifact when HTTP is enabled, otherwise metadata."`
}

// DownloadResult is the output of the lostfilm_download tool. Metadata fields
// are always present; the content is delivered inline (base64) or via a
// one-time download URL (artifact) depending on the mode.
type DownloadResult struct {
	Quality        string    `json:"quality,omitempty"`
	Filename       string    `json:"filename"`
	SizeBytes      int       `json:"sizeBytes"`
	SHA256         string    `json:"sha256"`
	InfoHash       string    `json:"infoHash,omitempty"`
	FileCount      int       `json:"fileCount,omitempty"`
	TotalSizeBytes int64     `json:"totalSizeBytes,omitempty"`
	ContentBase64  string    `json:"contentBase64,omitempty"`
	ArtifactID     string    `json:"artifactId,omitempty"`
	DownloadURL    string    `json:"downloadUrl,omitempty"`
	ExpiresAt      time.Time `json:"expiresAt,omitzero"`
}

// DownloadTool returns the MCP tool definition for lostfilm_download.
func DownloadTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "lostfilm_download",
		Description: "Fetch an episode's .torrent (choosing a quality variant), enriched with its file list, info-hash, and sha256. Returns a one-time download URL (HTTP mode) or metadata (stdio) by default; set mode=base64 for inline content. Requires authentication",
		Annotations: &mcp.ToolAnnotations{
			Title:           "Download Torrent",
			DestructiveHint: ptrBool(false),
			OpenWorldHint:   ptrBool(true),
		},
	}
}

// NewDownloadHandler creates a handler for the lostfilm_download tool. In
// artifact mode the .torrent is stored and served once from artifactBaseURL.
func NewDownloadHandler(
	client lostfilm.Client,
	store *artifact.Store,
	artifactBaseURL string,
	httpEnabled bool,
) mcp.ToolHandlerFor[DownloadParams, DownloadResult] {
	return func(
		ctx context.Context,
		_ *mcp.CallToolRequest,
		params DownloadParams,
	) (*mcp.CallToolResult, DownloadResult, error) {
		result, err := runDownload(ctx, client, store, artifactBaseURL, httpEnabled, &params)
		if err != nil {
			return &mcp.CallToolResult{IsError: true}, DownloadResult{}, err
		}

		return nil, result, nil
	}
}

// runDownload resolves the chosen variant, downloads it, and delivers it in the
// requested mode.
func runDownload(
	ctx context.Context,
	client lostfilm.Client,
	store *artifact.Store,
	artifactBaseURL string,
	httpEnabled bool,
	params *DownloadParams,
) (DownloadResult, error) {
	vErr := validateEpisode(params.SeriesID, params.Season, params.Episode)
	if vErr != nil {
		return DownloadResult{}, vErr
	}

	mode, modeErr := resolveMode(params.Mode, httpEnabled)
	if modeErr != nil {
		return DownloadResult{}, validationErr(modeErr)
	}

	if mode == modeArtifact && !httpEnabled {
		return DownloadResult{}, validationErr(ErrArtifactUnavailable)
	}

	variant, file, err := resolveAndDownload(ctx, client, params)
	if err != nil {
		return DownloadResult{}, err
	}

	sum := sha256.Sum256(file.Content)
	result := DownloadResult{
		Quality:   variant.Quality,
		Filename:  file.Filename,
		SizeBytes: file.SizeBytes,
		SHA256:    hex.EncodeToString(sum[:]),
	}
	enrichWithMeta(&result, file.Content)

	deliverErr := deliver(&result, mode, store, artifactBaseURL, file)
	if deliverErr != nil {
		return DownloadResult{}, deliverErr
	}

	return result, nil
}

// resolveAndDownload resolves the episode to its quality variants, picks one,
// and downloads its .torrent.
func resolveAndDownload(
	ctx context.Context,
	client lostfilm.Client,
	params *DownloadParams,
) (*lostfilm.TorrentVariant, *lostfilm.TorrentFile, error) {
	variants, err := client.Torrents(ctx, params.SeriesID, params.Season, params.Episode)
	if err != nil {
		return nil, nil, lostfilmErr("torrent resolution failed", err)
	}

	variant := pickVariant(variants, params.Quality)
	if variant == nil {
		return nil, nil, lostfilmErr("download failed", ErrNoVariants)
	}

	file, err := client.Download(ctx, variant.DownloadURL)
	if err != nil {
		return nil, nil, lostfilmErr("download failed", err)
	}

	return variant, file, nil
}

// resolveMode validates the requested mode and applies the adaptive default:
// artifact when the HTTP transport is enabled, otherwise metadata.
func resolveMode(raw string, httpEnabled bool) (string, error) {
	switch raw {
	case "":
		if httpEnabled {
			return modeArtifact, nil
		}

		return modeMetadata, nil
	case modeMetadata, modeBase64, modeArtifact:
		return raw, nil
	default:
		return "", errors.Wrapf(ErrInvalidMode, "%q", raw)
	}
}

// deliver fills the mode-specific output fields (inline base64 or a one-time
// artifact URL); metadata mode adds nothing beyond the common fields.
func deliver(
	result *DownloadResult,
	mode string,
	store *artifact.Store,
	artifactBaseURL string,
	file *lostfilm.TorrentFile,
) error {
	switch mode {
	case modeBase64:
		result.ContentBase64 = base64.StdEncoding.EncodeToString(file.Content)
	case modeArtifact:
		art, putErr := store.Put(file.Filename, file.Content)
		if putErr != nil {
			return lostfilmErr("store artifact", putErr)
		}

		result.ArtifactID = art.Token
		result.DownloadURL = artifactBaseURL + "/artifacts/" + art.Token
		result.ExpiresAt = art.ExpiresAt
	case modeMetadata:
	default:
		// Unreachable: resolveMode validates the mode first. The arm makes the
		// invariant self-defending if a new mode is ever added.
		return lostfilmErr("deliver", errors.Wrapf(ErrInvalidMode, "%q", mode))
	}

	return nil
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
