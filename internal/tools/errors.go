// Package tools provides MCP tool definitions and handlers for lostfilm.
package tools

import "github.com/cockroachdb/errors"

// ErrValidation indicates invalid parameters provided by the caller.
var ErrValidation = errors.New("validation error")

// ErrQueryRequired is returned when a search query is empty.
var ErrQueryRequired = errors.New("query is required")

// ErrLinkRequired is returned when a series link is empty.
var ErrLinkRequired = errors.New("link is required")

// ErrSeriesIDRequired is returned when a series ID is missing or non-positive.
var ErrSeriesIDRequired = errors.New("seriesId must be a positive integer")

// ErrSeasonRequired is returned when a season number is missing or non-positive.
var ErrSeasonRequired = errors.New("season must be a positive integer")

// ErrEpisodeRequired is returned when an episode number is missing or
// non-positive.
var ErrEpisodeRequired = errors.New("episode must be a positive integer")

// ErrLostfilm indicates a failure talking to lostfilm.
var ErrLostfilm = errors.New("lostfilm request error")

// ErrNoVariants indicates the requested episode resolved to no torrent variants.
var ErrNoVariants = errors.New("no torrents found for the requested episode")

// ErrNoDownloadDir is returned when saveToDisk is requested but no download
// directory is configured.
var ErrNoDownloadDir = errors.New("saveToDisk requires LOSTFILM_DOWNLOAD_DIR to be set")

// validationErr marks an error as a validation error.
func validationErr(err error) error {
	//nolint:wrapcheck // Mark adds a sentinel category; the caller supplies the message.
	return errors.Mark(err, ErrValidation)
}

// lostfilmErr wraps a message and underlying error as a lostfilm error.
func lostfilmErr(msg string, err error) error {
	//nolint:wrapcheck // Mark adds a sentinel category on top of Wrap which adds context.
	return errors.Mark(errors.Wrap(err, msg), ErrLostfilm)
}
