package lostfilm_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/lexfrei/mcp-lostfilm/internal/lostfilm"
)

// TestLive exercises the full flow against the real lostfilm site. It is
// skipped unless LOSTFILM_LIVE is set, and reads credentials from the
// environment so they never live in the repository:
//
//	LOSTFILM_LIVE=1 LOSTFILM_EMAIL=... LOSTFILM_PASSWORD=... \
//	  go test -run TestLive -count=1 ./internal/lostfilm/
func TestLive(t *testing.T) {
	if os.Getenv("LOSTFILM_LIVE") == "" {
		t.Skip("set LOSTFILM_LIVE=1 to run the live integration test")
	}

	client, err := lostfilm.New(&lostfilm.Options{
		BaseURL:  os.Getenv("LOSTFILM_BASE_URL"),
		Email:    os.Getenv("LOSTFILM_EMAIL"),
		Password: os.Getenv("LOSTFILM_PASSWORD"),
		Cookie:   os.Getenv("LOSTFILM_COOKIE"),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	feed := liveFeed(t, ctx, client)
	liveSearch(t, ctx, client)
	liveTorrents(t, ctx, client, feed)
}

// liveFeed checks the public RSS feed and returns it for downstream use.
func liveFeed(t *testing.T, ctx context.Context, client lostfilm.Client) []lostfilm.FeedItem {
	t.Helper()

	feed, err := client.Feed(ctx)
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}

	if len(feed) == 0 {
		t.Fatal("Feed returned no items")
	}

	t.Logf("feed: %d items; first=%q s%02de%02d id=%d",
		len(feed), feed[0].Title, feed[0].Season, feed[0].Episode, feed[0].SeriesID)

	return feed
}

// liveSearch checks the public search endpoint and the series page parse.
func liveSearch(t *testing.T, ctx context.Context, client lostfilm.Client) {
	t.Helper()

	results, err := client.Search(ctx, "Peaky Blinders")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Search returned no results")
	}

	top := results[0]
	t.Logf("search: %d results; top=%q (%s) id=%d link=%s", len(results), top.Title, top.Type, top.ID, top.Link)

	info, err := client.SeriesInfo(ctx, top.Link)
	if err != nil {
		t.Fatalf("SeriesInfo: %v", err)
	}

	t.Logf("series: %q (%q) seasons=%d episodes=%d", info.Title, info.TitleOrig, info.Seasons, len(info.Episodes))
}

// liveTorrents resolves and downloads a torrent for the first feed entry that is
// a regular episode, exercising the authenticated path end to end.
func liveTorrents(t *testing.T, ctx context.Context, client lostfilm.Client, feed []lostfilm.FeedItem) {
	t.Helper()

	target := pickEpisode(feed)
	if target == nil {
		t.Skip("no regular episode in the feed to resolve")
	}

	variants, err := client.Torrents(ctx, target.SeriesID, target.Season, target.Episode)
	if err != nil {
		t.Fatalf("Torrents(c=%d s=%d e=%d): %v", target.SeriesID, target.Season, target.Episode, err)
	}

	if len(variants) == 0 {
		t.Fatal("Torrents returned no variants")
	}

	for _, variant := range variants {
		t.Logf("variant: %s %s %s", variant.Quality, variant.SizeText, variant.DownloadURL)
	}

	file, err := client.Download(ctx, variants[0].DownloadURL)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}

	if len(file.Content) == 0 || file.Content[0] != 'd' {
		t.Fatalf("download is not a bencoded torrent: %q", file.Filename)
	}

	t.Logf("download: %q (%d bytes)", file.Filename, file.SizeBytes)
}

// pickEpisode returns the first feed item that maps to a concrete episode with a
// resolvable series id.
func pickEpisode(feed []lostfilm.FeedItem) *lostfilm.FeedItem {
	for i := range feed {
		item := &feed[i]
		if item.SeriesID > 0 && item.Season > 0 && item.Episode > 0 && item.Episode != lostfilm.WholeSeasonEpisode {
			return item
		}
	}

	return nil
}
