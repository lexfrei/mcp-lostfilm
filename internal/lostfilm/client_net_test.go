package lostfilm

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/cockroachdb/errors"
)

// Endpoint paths and hosts reused across the transport-level tests.
const (
	pathAjaxik    = "/ajaxik.php"
	pathVSearch   = "/v_search.php"
	pathRSS       = "/rss.xml"
	pathV         = "/V/"
	hostPrimary   = "www.lostfilm.tv"
	hostSecondary = "www.lostfilmtv5.site"
	primaryBase   = "https://www.lostfilm.tv/"

	loginOK   = `{"name":"u","success":true,"result":"ok"}`
	setCookie = "lf_session=tok; Path=/"

	vSearchMeta = `<html><head><meta http-equiv="refresh" content="0; url=/V/?c=1&s=1&e=1&u=1&h=abc&ts=1"></head></html>`

	rssFixture = `<?xml version="1.0" encoding="utf-8"?><rss version="0.91"><channel><title>x</title>` +
		`<item><title>Show (Show Orig). Ep. (S01E02)</title>` +
		`<link>https://www.lostfilm.tv/series/X/season_1/episode_2/</link>` +
		`<description><![CDATA[<img src="/Static/Images/100/Posters/image.jpg" />]]></description>` +
		`<pubDate>Tue, 16 Jun 2026 20:07:59 +0000</pubDate></item></channel></rss>`
)

// fakeTransport is an http.RoundTripper that records request targets and
// delegates response construction to fn.
type fakeTransport struct {
	fn    func(req *http.Request) *http.Response
	mu    sync.Mutex
	calls []string
}

func (f *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	f.calls = append(f.calls, req.URL.Host+req.URL.Path)
	f.mu.Unlock()

	resp := f.fn(req)
	resp.Request = req

	if resp.Header == nil {
		resp.Header = make(http.Header)
	}

	return resp, nil
}

func (f *fakeTransport) contacted(target string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Contains(f.calls, target)
}

func textResp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func newFakeScraper(t *testing.T, opts *Options, fn func(req *http.Request) *http.Response) (*Scraper, *fakeTransport) {
	t.Helper()

	transport := &fakeTransport{fn: fn}
	opts.Transport = transport

	scraper, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return scraper, transport
}

func TestFeed_MirrorFailover(t *testing.T) {
	t.Parallel()

	scraper, transport := newFakeScraper(t, &Options{}, func(req *http.Request) *http.Response {
		if req.URL.Host == hostPrimary {
			return textResp(http.StatusServiceUnavailable, "")
		}

		if req.URL.Path == pathRSS {
			return textResp(http.StatusOK, rssFixture)
		}

		return textResp(http.StatusNotFound, "")
	})

	items, err := scraper.Feed(context.Background())
	if err != nil {
		t.Fatalf("Feed: %v", err)
	}

	if len(items) != 1 || items[0].SeriesID != 100 {
		t.Fatalf("items = %+v", items)
	}

	if !transport.contacted(hostPrimary + pathRSS) {
		t.Error("expected the primary mirror to be tried first")
	}

	if !transport.contacted(hostSecondary + pathRSS) {
		t.Error("expected failover to the second mirror")
	}
}

func TestTorrents_LoginAndResolve(t *testing.T) {
	t.Parallel()

	scraper, transport := newFakeScraper(t,
		&Options{BaseURL: primaryBase, Email: "u@example.com", Password: "p"},
		func(req *http.Request) *http.Response {
			switch req.URL.Path {
			case pathAjaxik:
				resp := textResp(http.StatusOK, loginOK)
				resp.Header.Set("Set-Cookie", setCookie)

				return resp
			case pathVSearch:
				return textResp(http.StatusOK, vSearchMeta)
			case pathV:
				return textResp(http.StatusOK, torrentPageFixture)
			default:
				return textResp(http.StatusNotFound, "")
			}
		})

	variants, err := scraper.Torrents(context.Background(), 1, 1, 1)
	if err != nil {
		t.Fatalf("Torrents: %v", err)
	}

	if len(variants) != 2 {
		t.Fatalf("variants = %d, want 2", len(variants))
	}

	if !transport.contacted(hostPrimary + pathAjaxik) {
		t.Error("expected a login call to ajaxik.php")
	}
}

func TestTorrents_NotAuthenticated(t *testing.T) {
	t.Parallel()

	// Cookie-only client with no password cannot recover from an expired session.
	scraper, _ := newFakeScraper(t,
		&Options{BaseURL: primaryBase, Cookie: "lf_session=stale"},
		func(req *http.Request) *http.Response {
			if req.URL.Path == pathVSearch {
				return textResp(http.StatusOK, loginPromptMarker)
			}

			return textResp(http.StatusNotFound, "")
		})

	_, err := scraper.Torrents(context.Background(), 1, 1, 1)
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("expected ErrNotAuthenticated, got %v", err)
	}
}

func TestTorrents_ReauthRetry(t *testing.T) {
	t.Parallel()

	var vSearchCalls int

	scraper, transport := newFakeScraper(t,
		&Options{BaseURL: primaryBase, Email: "u@example.com", Password: "p", Cookie: "lf_session=stale"},
		func(req *http.Request) *http.Response {
			switch req.URL.Path {
			case pathAjaxik:
				resp := textResp(http.StatusOK, loginOK)
				resp.Header.Set("Set-Cookie", setCookie)

				return resp
			case pathVSearch:
				vSearchCalls++
				if vSearchCalls == 1 {
					return textResp(http.StatusOK, loginPromptMarker)
				}

				return textResp(http.StatusOK, vSearchMeta)
			case pathV:
				return textResp(http.StatusOK, torrentPageFixture)
			default:
				return textResp(http.StatusNotFound, "")
			}
		})

	variants, err := scraper.Torrents(context.Background(), 1, 1, 1)
	if err != nil {
		t.Fatalf("Torrents after reauth: %v", err)
	}

	if len(variants) != 2 {
		t.Fatalf("variants = %d, want 2", len(variants))
	}

	if !transport.contacted(hostPrimary + pathAjaxik) {
		t.Error("expected a re-login call after the expired session")
	}
}

func TestDownload_Success(t *testing.T) {
	t.Parallel()

	scraper, _ := newFakeScraper(t, &Options{BaseURL: primaryBase}, func(_ *http.Request) *http.Response {
		resp := textResp(http.StatusOK, "d4:infod6:lengthi1e4:name1:a12:piece lengthi1eee")
		resp.Header.Set("Content-Disposition", `attachment; filename="ep.torrent"`)

		return resp
	})

	file, err := scraper.Download(context.Background(), "https://n.tracktor.site/td.php?s=x")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}

	if file.Filename != "ep.torrent" || file.Content[0] != bencodeDictPrefix {
		t.Errorf("file = %+v", file)
	}
}

func TestDownload_RejectsHTML(t *testing.T) {
	t.Parallel()

	scraper, _ := newFakeScraper(t, &Options{BaseURL: primaryBase}, func(_ *http.Request) *http.Response {
		return textResp(http.StatusOK, "<html>log in first</html>")
	})

	_, err := scraper.Download(context.Background(), "https://n.tracktor.site/td.php?s=x")
	if !errors.Is(err, ErrDownloadFailed) {
		t.Fatalf("expected ErrDownloadFailed, got %v", err)
	}
}

func TestDownload_NonOK(t *testing.T) {
	t.Parallel()

	scraper, _ := newFakeScraper(t, &Options{BaseURL: primaryBase}, func(_ *http.Request) *http.Response {
		return textResp(http.StatusNotFound, "nope")
	})

	_, err := scraper.Download(context.Background(), "https://n.tracktor.site/td.php?s=x")
	if !errors.Is(err, ErrDownloadFailed) {
		t.Fatalf("expected ErrDownloadFailed, got %v", err)
	}
}

func TestDownload_TooLarge(t *testing.T) {
	t.Parallel()

	scraper, _ := newFakeScraper(t, &Options{BaseURL: primaryBase}, func(_ *http.Request) *http.Response {
		return textResp(http.StatusOK, "ddddddddddddddddd")
	})
	scraper.maxTorrentBytes = 4

	_, err := scraper.Download(context.Background(), "https://n.tracktor.site/td.php?s=x")
	if !errors.Is(err, ErrTorrentTooLarge) {
		t.Fatalf("expected ErrTorrentTooLarge, got %v", err)
	}
}
