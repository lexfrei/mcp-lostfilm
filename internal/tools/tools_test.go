package tools_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/cockroachdb/errors"

	"github.com/lexfrei/mcp-lostfilm/internal/lostfilm"
	"github.com/lexfrei/mcp-lostfilm/internal/tools"
)

// minimalTorrent is a tiny but well-formed bencoded single-file .torrent, used
// to exercise the download tool's metadata enrichment.
const minimalTorrent = "d4:infod6:lengthi1024e4:name8:file.txt12:piece lengthi16384eee"

const (
	quality1080 = "1080p"
	quality720  = "720p"
	qualitySD   = "SD"
	torrentName = "ep.torrent"
)

// mockClient is a configurable lostfilm.Client for handler tests.
type mockClient struct {
	feed     []lostfilm.FeedItem
	search   []lostfilm.Series
	series   *lostfilm.SeriesInfo
	variants []lostfilm.TorrentVariant
	file     *lostfilm.TorrentFile
	err      error
}

func (m *mockClient) Feed(_ context.Context) ([]lostfilm.FeedItem, error) {
	return m.feed, m.err
}

func (m *mockClient) Search(_ context.Context, _ string) ([]lostfilm.Series, error) {
	return m.search, m.err
}

func (m *mockClient) SeriesInfo(_ context.Context, _ string) (*lostfilm.SeriesInfo, error) {
	return m.series, m.err
}

func (m *mockClient) Torrents(_ context.Context, _, _, _ int) ([]lostfilm.TorrentVariant, error) {
	return m.variants, m.err
}

func (m *mockClient) Download(_ context.Context, _ string) (*lostfilm.TorrentFile, error) {
	return m.file, m.err
}

func TestFeedHandler_Limit(t *testing.T) {
	t.Parallel()

	client := &mockClient{feed: []lostfilm.FeedItem{{Title: "a"}, {Title: "b"}, {Title: "c"}}}
	handler := tools.NewFeedHandler(client)

	_, result, err := handler(context.Background(), nil, tools.FeedParams{Limit: 2})
	if err != nil {
		t.Fatalf("feed: %v", err)
	}

	if result.Count != 2 || len(result.Items) != 2 {
		t.Errorf("count = %d, items = %d, want 2/2", result.Count, len(result.Items))
	}
}

func TestSearchHandler_EmptyQuery(t *testing.T) {
	t.Parallel()

	handler := tools.NewSearchHandler(&mockClient{})

	_, _, err := handler(context.Background(), nil, tools.SearchParams{})
	if !errors.Is(err, tools.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestSearchHandler_Results(t *testing.T) {
	t.Parallel()

	client := &mockClient{search: []lostfilm.Series{{ID: 197, Title: "x"}}}
	handler := tools.NewSearchHandler(client)

	_, result, err := handler(context.Background(), nil, tools.SearchParams{Query: "x"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}

	if result.Count != 1 || result.Results[0].ID != 197 {
		t.Errorf("result = %+v", result)
	}
}

func TestSeriesHandler_EmptyLink(t *testing.T) {
	t.Parallel()

	handler := tools.NewSeriesHandler(&mockClient{})

	_, _, err := handler(context.Background(), nil, tools.SeriesParams{})
	if !errors.Is(err, tools.ErrValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestTorrentsHandler_Validation(t *testing.T) {
	t.Parallel()

	handler := tools.NewTorrentsHandler(&mockClient{})

	tests := []tools.TorrentsParams{
		{SeriesID: 0, Season: 1, Episode: 1},
		{SeriesID: 1, Season: 0, Episode: 1},
		{SeriesID: 1, Season: 1, Episode: 0},
	}

	for _, params := range tests {
		_, _, err := handler(context.Background(), nil, params)
		if !errors.Is(err, tools.ErrValidation) {
			t.Errorf("params %+v: expected validation error, got %v", params, err)
		}
	}
}

func TestTorrentsHandler_Results(t *testing.T) {
	t.Parallel()

	client := &mockClient{variants: []lostfilm.TorrentVariant{{Quality: qualitySD}, {Quality: quality1080}}}
	handler := tools.NewTorrentsHandler(client)

	_, result, err := handler(context.Background(), nil, tools.TorrentsParams{SeriesID: 1, Season: 1, Episode: 1})
	if err != nil {
		t.Fatalf("torrents: %v", err)
	}

	if result.Count != 2 {
		t.Errorf("count = %d, want 2", result.Count)
	}
}

func TestDownloadHandler_PicksQualityAndEnriches(t *testing.T) {
	t.Parallel()

	client := &mockClient{
		variants: []lostfilm.TorrentVariant{
			{Quality: qualitySD, SizeBytes: 100, DownloadURL: "https://n.tracktor.site/td.php?s=SD"},
			{Quality: quality1080, SizeBytes: 900, DownloadURL: "https://n.tracktor.site/td.php?s=FHD"},
		},
		file: &lostfilm.TorrentFile{
			Filename:  torrentName,
			Content:   []byte(minimalTorrent),
			SizeBytes: len(minimalTorrent),
		},
	}
	handler := tools.NewDownloadHandler(client, "")

	_, result, err := handler(context.Background(), nil, tools.DownloadParams{
		SeriesID: 1, Season: 1, Episode: 1, Quality: quality1080,
	})
	if err != nil {
		t.Fatalf("download: %v", err)
	}

	if result.Quality != quality1080 {
		t.Errorf("quality = %q, want %q", result.Quality, quality1080)
	}

	if result.ContentBase64 == "" {
		t.Error("expected base64 content")
	}

	if result.InfoHash == "" || result.FileCount != 1 {
		t.Errorf("enrichment failed: hash=%q files=%d", result.InfoHash, result.FileCount)
	}
}

func TestDownloadHandler_SaveRequiresDir(t *testing.T) {
	t.Parallel()

	client := &mockClient{
		variants: []lostfilm.TorrentVariant{{Quality: qualitySD, DownloadURL: "https://n.tracktor.site/td.php?s=SD"}},
		file:     &lostfilm.TorrentFile{Filename: torrentName, Content: []byte(minimalTorrent)},
	}
	handler := tools.NewDownloadHandler(client, "")
	save := true

	_, _, err := handler(context.Background(), nil, tools.DownloadParams{
		SeriesID: 1, Season: 1, Episode: 1, SaveToDisk: &save,
	})
	if !errors.Is(err, tools.ErrValidation) {
		t.Fatalf("expected validation error for missing download dir, got %v", err)
	}
}

func TestDownloadHandler_QualityIgnoresDescription(t *testing.T) {
	t.Parallel()

	// A 1080p variant whose description mentions "720p" must not be chosen when
	// the caller asks for 720p; matching is on the quality label only.
	client := &mockClient{
		variants: []lostfilm.TorrentVariant{
			{Quality: quality1080, Description: "Видео: 1080p (rip from 720p source)", SizeBytes: 900, DownloadURL: "u1"},
			{Quality: quality720, SizeBytes: 700, DownloadURL: "u2"},
		},
		file: &lostfilm.TorrentFile{Filename: torrentName, Content: []byte(minimalTorrent)},
	}
	handler := tools.NewDownloadHandler(client, "")

	_, result, err := handler(context.Background(), nil, tools.DownloadParams{
		SeriesID: 1, Season: 1, Episode: 1, Quality: quality720,
	})
	if err != nil {
		t.Fatalf("download: %v", err)
	}

	if result.Quality != quality720 {
		t.Errorf("quality = %q, want %q", result.Quality, quality720)
	}
}

func TestDownloadHandler_SaveSanitizesFilename(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	client := &mockClient{
		variants: []lostfilm.TorrentVariant{{Quality: qualitySD, DownloadURL: "u"}},
		file:     &lostfilm.TorrentFile{Filename: "../../evil.torrent", Content: []byte(minimalTorrent)},
	}
	handler := tools.NewDownloadHandler(client, dir)
	save := true

	_, result, err := handler(context.Background(), nil, tools.DownloadParams{
		SeriesID: 1, Season: 1, Episode: 1, SaveToDisk: &save,
	})
	if err != nil {
		t.Fatalf("download: %v", err)
	}

	want := filepath.Join(dir, "evil.torrent")
	if result.SavedPath != want {
		t.Errorf("savedPath = %q, want %q (traversal must be stripped)", result.SavedPath, want)
	}
}

func TestDownloadHandler_SaveRejectsDotName(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	client := &mockClient{
		variants: []lostfilm.TorrentVariant{{Quality: qualitySD, DownloadURL: "u"}},
		file:     &lostfilm.TorrentFile{Filename: "..", Content: []byte(minimalTorrent)},
	}
	handler := tools.NewDownloadHandler(client, dir)
	save := true

	_, result, err := handler(context.Background(), nil, tools.DownloadParams{
		SeriesID: 1, Season: 1, Episode: 1, SaveToDisk: &save,
	})
	if err != nil {
		t.Fatalf("download: %v", err)
	}

	want := filepath.Join(dir, "download.torrent")
	if result.SavedPath != want {
		t.Errorf("savedPath = %q, want %q (a \"..\" name must fall back, not escape)", result.SavedPath, want)
	}
}
