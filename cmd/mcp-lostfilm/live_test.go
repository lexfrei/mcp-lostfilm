package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/lexfrei/mcp-lostfilm/internal/artifact"
	"github.com/lexfrei/mcp-lostfilm/internal/lostfilm"
	"github.com/lexfrei/mcp-lostfilm/internal/tools"
)

// TestLiveArtifactDownload drives the full artifact path against the real site:
// resolve a real episode, download it in artifact mode, then fetch the one-time
// URL twice (200 then 404). It is skipped unless LOSTFILM_LIVE is set and reads
// credentials from the environment:
//
//	LOSTFILM_LIVE=1 LOSTFILM_COOKIE='lf_session=...' \
//	  go test -run TestLiveArtifactDownload -count=1 ./cmd/mcp-lostfilm/
func TestLiveArtifactDownload(t *testing.T) {
	if os.Getenv("LOSTFILM_LIVE") == "" {
		t.Skip("set LOSTFILM_LIVE=1 to run the live artifact test")
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

	series, season, episode := liveEpisode(t, ctx, client)

	store := artifact.NewStore(time.Minute)
	srv := httptest.NewServer(artifactMux(store))
	defer srv.Close()

	handler := tools.NewDownloadHandler(client, store, srv.URL, true)

	_, result, err := handler(ctx, nil, tools.DownloadParams{
		SeriesID: series, Season: season, Episode: episode, Mode: "artifact",
	})
	if err != nil {
		t.Fatalf("download: %v", err)
	}

	if result.DownloadURL == "" || result.SHA256 == "" {
		t.Fatalf("artifact result incomplete: %+v", result)
	}

	t.Logf("artifact: %s (%d bytes) sha256=%s", result.Filename, result.SizeBytes, result.SHA256)

	fetchOnce(t, ctx, result.DownloadURL, result.SHA256)
	fetchGone(t, ctx, result.DownloadURL)
}

// artifactMux builds the same /artifacts/{id} route the real server serves.
func artifactMux(store *artifact.Store) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("GET /artifacts/{id}", artifactHandler(testLogger(), store))

	return mux
}

// liveEpisode resolves the first regular episode from the live feed.
func liveEpisode(t *testing.T, ctx context.Context, client lostfilm.Client) (int, int, int) {
	t.Helper()

	feed, err := client.Feed(ctx)
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}

	for i := range feed {
		item := feed[i]
		if item.SeriesID > 0 && item.Season > 0 && item.Episode > 0 && item.Episode != lostfilm.WholeSeasonEpisode {
			return item.SeriesID, item.Season, item.Episode
		}
	}

	t.Skip("no regular episode in the feed to resolve")

	return 0, 0, 0
}

// fetchOnce asserts the first GET serves the torrent with a matching digest.
func fetchOnce(t *testing.T, ctx context.Context, url, wantSHA string) {
	t.Helper()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("first GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first GET status = %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 || body[0] != 'd' {
		t.Fatalf("served body is not a bencoded torrent")
	}

	if got := resp.Header.Get("X-Content-Sha256"); got != wantSHA {
		t.Errorf("X-Content-Sha256 = %q, want %q", got, wantSHA)
	}
}

// fetchGone asserts the second GET of the same URL is 404 (one-time).
func fetchGone(t *testing.T, ctx context.Context, url string) {
	t.Helper()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("second GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("second GET status = %d, want 404 (one-time)", resp.StatusCode)
	}
}
