package lostfilm

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/cockroachdb/errors"
)

// defaultUserAgent mimics a recent desktop Chrome to reduce anti-bot friction.
// lostfilm sits behind Cloudflare; a realistic User-Agent is the cheapest
// mitigation and must match the one used to mint a cf_clearance cookie when one
// is supplied.
const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// Doer is the subset of *http.Client the scraper relies on. Hiding the
// transport behind an interface lets callers swap in a TLS-impersonating
// client (e.g. one built on utls) without touching the scraping code.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// resolve builds an absolute URL for a path under the active mirror, with the
// given already-encoded query attached.
func (s *Scraper) resolve(path, rawQuery string) string {
	ref := &url.URL{Path: path, RawQuery: rawQuery}

	return s.currentBase().ResolveReference(ref).String()
}

// newRequest creates a request with the configured User-Agent applied.
func (s *Scraper) newRequest(
	ctx context.Context,
	method, target string,
	body io.Reader,
) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, errors.Wrap(err, "build request")
	}

	req.Header.Set("User-Agent", s.userAgent)

	return req, nil
}

// doRequest performs a request, classifying transport failures and 5xx
// responses as ErrMirrorUnavailable so the caller can fail over to another
// mirror. The caller owns the returned body.
func (s *Scraper) doRequest(req *http.Request) (*http.Response, error) {
	resp, err := s.http.Do(req)
	if err != nil {
		//nolint:wrapcheck // Mark adds the mirror-failover sentinel on top of Wrap.
		return nil, errors.Mark(errors.Wrap(err, req.Method+" "+req.URL.Path), ErrMirrorUnavailable)
	}

	if resp.StatusCode >= http.StatusInternalServerError {
		_ = resp.Body.Close()

		return nil, errors.Wrapf(ErrMirrorUnavailable, "status %d from %s", resp.StatusCode, req.URL.Host)
	}

	return resp, nil
}

// getDoc fetches an HTML page under the active mirror and parses it. lostfilm
// serves UTF-8, so no charset transcoding is needed.
func (s *Scraper) getDoc(ctx context.Context, path string, query url.Values) (*goquery.Document, error) {
	resp, err := s.get(ctx, path, query)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "parse HTML")
	}

	return doc, nil
}

// getString fetches a resource under the active mirror and returns its body as
// a string. Used for the RSS feed and the v_search meta-refresh page.
func (s *Scraper) getString(ctx context.Context, path string, query url.Values) (string, error) {
	resp, err := s.get(ctx, path, query)
	if err != nil {
		return "", err
	}

	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "read response body")
	}

	return string(data), nil
}

// get issues a GET under the active mirror with the given query.
func (s *Scraper) get(ctx context.Context, path string, query url.Values) (*http.Response, error) {
	req, err := s.newRequest(ctx, http.MethodGet, s.resolve(path, query.Encode()), nil)
	if err != nil {
		return nil, err
	}

	return s.doRequest(req)
}

// postForm submits an application/x-www-form-urlencoded body to a path under the
// active mirror and returns the response body as a string. Used for ajaxik.php
// (login and search), which lostfilm serves as UTF-8 JSON.
func (s *Scraper) postForm(ctx context.Context, path string, values url.Values) (string, error) {
	req, err := s.newRequest(ctx, http.MethodPost, s.resolve(path, ""), strings.NewReader(values.Encode()))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := s.doRequest(req)
	if err != nil {
		return "", err
	}

	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "read response body")
	}

	return string(data), nil
}

// getAbsolute issues a GET to an absolute URL (e.g. the n.tracktor.site
// download host, which lives outside the mirror set). The caller owns the
// returned body and must close it.
func (s *Scraper) getAbsolute(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := s.newRequest(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.http.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "GET "+rawURL)
	}

	return resp, nil
}
