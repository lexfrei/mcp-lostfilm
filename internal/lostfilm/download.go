package lostfilm

import (
	"context"
	"io"
	"mime"
	"net/http"

	"github.com/cockroachdb/errors"
)

// maxTorrentSize caps how much of a download response is read into memory.
// .torrent files are small; this guards against an unexpected large body.
const maxTorrentSize = 32 << 20

// bencodeDictPrefix is the first byte of a valid bencoded .torrent (a dict).
const bencodeDictPrefix = 'd'

// Download fetches the .torrent file from a variant's DownloadURL (an absolute
// n.tracktor.site link). The signed token in the URL is self-authenticating, so
// this needs no session cookie.
func (s *Scraper) Download(ctx context.Context, downloadURL string) (*TorrentFile, error) {
	resp, err := s.getAbsolute(ctx, downloadURL)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Drain the body so the keep-alive connection to the download host can be
		// reused for the next request in the session.
		_, _ = io.Copy(io.Discard, resp.Body)

		return nil, errors.Wrapf(ErrDownloadFailed, "status %d", resp.StatusCode)
	}

	// Read one byte past the cap so an over-sized body is detected and rejected
	// rather than silently truncated into a wrong info-hash.
	data, err := io.ReadAll(io.LimitReader(resp.Body, s.maxTorrentBytes+1))
	if err != nil {
		return nil, errors.Wrap(err, "read torrent body")
	}

	if int64(len(data)) > s.maxTorrentBytes {
		return nil, errors.Wrapf(ErrTorrentTooLarge, "exceeds %d bytes", s.maxTorrentBytes)
	}

	// A stale or rejected request yields an HTML page; a valid torrent always
	// starts with a bencode dictionary.
	if len(data) == 0 || data[0] != bencodeDictPrefix {
		return nil, errors.Wrap(ErrDownloadFailed, "response is not a bencoded torrent")
	}

	return &TorrentFile{
		Filename:  filenameFromResponse(resp),
		Content:   data,
		SizeBytes: len(data),
	}, nil
}

// filenameFromResponse derives a download filename from the Content-Disposition
// header, falling back to "download.torrent". The returned name is
// server-controlled and untrusted: callers writing it to disk must sanitise it.
func filenameFromResponse(resp *http.Response) string {
	const fallback = "download.torrent"

	disposition := resp.Header.Get("Content-Disposition")
	if disposition == "" {
		return fallback
	}

	_, params, err := mime.ParseMediaType(disposition)
	if err != nil {
		return fallback
	}

	if filename := params["filename"]; filename != "" {
		return filename
	}

	return fallback
}
